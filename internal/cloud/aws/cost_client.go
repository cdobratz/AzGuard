package aws

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	CostExplorerEndpoint = "https://ce.us-east-1.amazonaws.com"
	FreeTierEndpoint     = "https://freetier.us-east-1.amazonaws.com"
)

// CostClient communicates with the AWS Cost Explorer API.
type CostClient struct {
	Region    string
	AccessKey string
	SecretKey string
	Token     string
	HTTP      *http.Client
}

// NewCostClient creates a new AWS cost client.
// It resolves credentials from explicit config, env vars, or AWS CLI.
func NewCostClient(accessKey, secretKey, sessionToken, region string) *CostClient {
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if sessionToken == "" {
		sessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	// Fallback: try AWS CLI credentials
	if accessKey == "" || secretKey == "" {
		if ak, sk, tok, err := getCredentialsFromCLI(); err == nil {
			if accessKey == "" {
				accessKey = ak
			}
			if secretKey == "" {
				secretKey = sk
			}
			if sessionToken == "" && tok != "" {
				sessionToken = tok
			}
		}
	}

	return &CostClient{
		Region:    region,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Token:     sessionToken,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

// IsConfigured returns true if the client has valid credentials.
func (c *CostClient) IsConfigured() bool {
	return c.AccessKey != "" && c.SecretKey != ""
}

// CostRecord represents a single cost entry from AWS.
type CostRecord struct {
	ServiceName string
	Cost        float64
	Currency    string
	Date        string
	Unit        string
}

// CostQueryResult holds aggregated cost query results.
type CostQueryResult struct {
	Records   []CostRecord
	TotalCost float64
	Currency  string
}

// FreeTierUsage represents a single free tier usage entry.
type FreeTierUsage struct {
	ServiceName    string  `json:"service"`
	UsageType      string  `json:"usageType"`
	Description    string  `json:"description"`
	ActualUsage    float64 `json:"actualUsageAmount"`
	ForecastUsage  float64 `json:"forecastedUsageAmount"`
	FreeTierLimit  float64 `json:"limit"`
	Unit           string  `json:"unit"`
	PercentUsed    float64 `json:"percentUsed"`
}

// GetFreeTierUsage queries the AWS Free Tier Usage API.
func (c *CostClient) GetFreeTierUsage(ctx context.Context) ([]FreeTierUsage, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("AWS credentials not configured. Run 'aws configure' or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY")
	}

	// Build GetFreeTierUsage request
	payload := `{}`
	body, err := c.callAPI(ctx, FreeTierEndpoint, "AWSFreeTierService.GetFreeTierUsage", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get free tier usage: %w", err)
	}

	var resp struct {
		FreeTierUsages []struct {
			Service           string  `json:"service"`
			UsageType         string  `json:"usageType"`
			Description       string  `json:"description"`
			ActualUsageAmount float64 `json:"actualUsageAmount"`
			ForecastedUsage   float64 `json:"forecastedUsageAmount"`
			Limit             struct {
				Amount float64 `json:"amount"`
				Unit   string  `json:"unit"`
			} `json:"limit"`
		} `json:"freeTierUsages"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse free tier response: %w", err)
	}

	var usages []FreeTierUsage
	for _, u := range resp.FreeTierUsages {
		pct := 0.0
		if u.Limit.Amount > 0 {
			pct = (u.ActualUsageAmount / u.Limit.Amount) * 100
		}
		usages = append(usages, FreeTierUsage{
			ServiceName:   u.Service,
			UsageType:     u.UsageType,
			Description:   u.Description,
			ActualUsage:   u.ActualUsageAmount,
			ForecastUsage: u.ForecastedUsage,
			FreeTierLimit: u.Limit.Amount,
			Unit:          u.Limit.Unit,
			PercentUsed:   pct,
		})
	}

	return usages, nil
}

// QueryCostsByService queries AWS Cost Explorer for costs grouped by service.
func (c *CostClient) QueryCostsByService(ctx context.Context, startDate, endDate string) (*CostQueryResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("AWS credentials not configured. Run 'aws configure' or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY")
	}

	payload := fmt.Sprintf(`{
		"TimePeriod": {"Start": "%s", "End": "%s"},
		"Granularity": "MONTHLY",
		"Metrics": ["UnblendedCost"],
		"GroupBy": [{"Type": "DIMENSION", "Key": "SERVICE"}]
	}`, startDate, endDate)

	body, err := c.callAPI(ctx, CostExplorerEndpoint, "AWSInsightsIndexService.GetCostAndUsage", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to query costs: %w", err)
	}

	var resp struct {
		ResultsByTime []struct {
			TimePeriod struct {
				Start string `json:"Start"`
				End   string `json:"End"`
			} `json:"TimePeriod"`
			Groups []struct {
				Keys    []string `json:"Keys"`
				Metrics map[string]struct {
					Amount string `json:"Amount"`
					Unit   string `json:"Unit"`
				} `json:"Metrics"`
			} `json:"Groups"`
		} `json:"ResultsByTime"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse cost response: %w", err)
	}

	result := &CostQueryResult{Currency: "USD"}
	for _, period := range resp.ResultsByTime {
		for _, group := range period.Groups {
			serviceName := ""
			if len(group.Keys) > 0 {
				serviceName = group.Keys[0]
			}
			if metric, ok := group.Metrics["UnblendedCost"]; ok {
				var costVal float64
				fmt.Sscanf(metric.Amount, "%f", &costVal)
				result.Records = append(result.Records, CostRecord{
					ServiceName: serviceName,
					Cost:        costVal,
					Currency:    metric.Unit,
					Date:        period.TimePeriod.Start,
				})
				result.TotalCost += costVal
			}
		}
	}

	return result, nil
}

// callAPI makes a signed request to an AWS JSON API.
func (c *CostClient) callAPI(ctx context.Context, endpoint, target, payload string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader([]byte(payload)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	if c.Token != "" {
		req.Header.Set("X-Amz-Security-Token", c.Token)
	}

	// Sign the request with AWS Signature V4
	c.signRequest(req, []byte(payload))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AWS API error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// signRequest applies AWS Signature V4 to an HTTP request.
func (c *CostClient) signRequest(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	// Determine service from endpoint
	service := "ce"
	if strings.Contains(req.URL.Host, "freetier") {
		service = "freetier"
	}

	req.Header.Set("X-Amz-Date", amzdate)
	req.Header.Set("Host", req.URL.Host)

	// Create canonical request
	payloadHash := sha256Hex(payload)
	signedHeaders := "content-type;host;x-amz-date;x-amz-target"
	if c.Token != "" {
		signedHeaders = "content-type;host;x-amz-date;x-amz-security-token;x-amz-target"
	}

	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-date:%s\n",
		req.Header.Get("Content-Type"), req.URL.Host, amzdate)
	if c.Token != "" {
		canonicalHeaders = fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-date:%s\nx-amz-security-token:%s\n",
			req.Header.Get("Content-Type"), req.URL.Host, amzdate, c.Token)
	}
	canonicalHeaders += fmt.Sprintf("x-amz-target:%s\n", req.Header.Get("X-Amz-Target"))

	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s",
		canonicalHeaders, signedHeaders, payloadHash)

	// Create string to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, c.Region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzdate, credentialScope, sha256Hex([]byte(canonicalRequest)))

	// Calculate signature
	signingKey := getSignatureKey(c.SecretKey, datestamp, c.Region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Set Authorization header
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.AccessKey, credentialScope, signedHeaders, signature))
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

// getCredentialsFromCLI attempts to get AWS credentials from the AWS CLI.
func getCredentialsFromCLI() (accessKey, secretKey, sessionToken string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aws", "configure", "export-credentials", "--format", "env")
	output, err := cmd.Output()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get AWS CLI credentials: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "AWS_ACCESS_KEY_ID":
			accessKey = val
		case "AWS_SECRET_ACCESS_KEY":
			secretKey = val
		case "AWS_SESSION_TOKEN":
			sessionToken = val
		}
	}

	if accessKey == "" || secretKey == "" {
		return "", "", "", fmt.Errorf("no credentials found in AWS CLI output")
	}

	return accessKey, secretKey, sessionToken, nil
}
