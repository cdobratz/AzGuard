package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type CostClient struct {
	Region    string
	AccessKey string
	SecretKey string
	SessionToken string
	HTTPClient *http.Client
}

func NewCostClient(accessKey, secretKey, sessionToken, region string) *CostClient {
	return &CostClient{
		AccessKey: accessKey,
		SecretKey: secretKey,
		SessionToken: sessionToken,
		Region:    region,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type CostQueryRequest struct {
	TimePeriod CostTimePeriod `json:"TimePeriod"`
	Granularity string `json:"Granularity"`
	Metrics    []string `json:"Metrics"`
	GroupBy    []Group `json:"GroupBy"`
}

type CostTimePeriod struct {
	Start string `json:"Start"`
	End   string `json:"End"`
}

type Group struct {
	Type string `json:"Type"`
	Key  string `json:"Key"`
}

type CostQueryResponse struct {
	ResultsByTime []ResultByTime `json:"ResultsByTime"`
}

type ResultByTime struct {
	TimePeriod TimePeriod `json:"TimePeriod"`
	Groups     []GroupCost `json:"Groups"`
	Total      CostMetric `json:"Total"`
}

type TimePeriod struct {
	Start string `json:"Start"`
	End   string `json:"End"`
}

type GroupCost struct {
	Keys   []string `json:"Keys"`
	Metrics map[string]CostMetric `json:"Metrics"`
}

type CostMetric struct {
	Amount string `json:"Amount"`
	Unit   string `json:"Unit"`
}

type CostResult struct {
	Records    []CostRecord
	TotalCost float64
	Currency  string
}

type CostRecord struct {
	ServiceName string
	Cost       float64
	Currency   string
	Date       string
}

func (c *CostClient) GetCredentials() (string, string, string) {
	if c.AccessKey == "" {
		c.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if c.SecretKey == "" {
		c.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if c.SessionToken == "" {
		c.SessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}
	return c.AccessKey, c.SecretKey, c.SessionToken
}

func (c *CostClient) QueryCosts(ctx context.Context, startDate, endDate string) (*CostResult, error) {
	accessKey, secretKey, sessionToken := c.GetCredentials()
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials not configured. Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY")
	}

	url := fmt.Sprintf("https://ce.%s.amazonaws.com/", c.Region)
	if c.Region == "" {
		c.Region = "us-east-1"
	}

	req := CostQueryRequest{
		TimePeriod: CostTimePeriod{
			Start: startDate,
			End:   endDate,
		},
		Granularity: "DAILY",
		Metrics:     []string{"UnblendedCost"},
		GroupBy: []Group{
			{Type: "DIMENSION", Key: "SERVICE"},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Amz-Target", "AWSInsightsIndexService.GetCostAndUsage")
	httpReq.Header.Set("X-Amz-Date", time.Now().UTC().Format("20060102T150405Z"))

	signer := NewSigner(accessKey, secretKey, sessionToken)
	signer.Sign(httpReq, body)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AWS Cost Explorer request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CostQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return c.parseResponse(result), nil
}

func (c *CostClient) parseResponse(resp CostQueryResponse) *CostResult {
	var records []CostRecord
	var totalCost float64
	currency := "USD"

	for _, result := range resp.ResultsByTime {
		for _, group := range result.Groups {
			serviceName := "Unknown"
			if len(group.Keys) > 0 {
				serviceName = group.Keys[0]
			}

			cost := 0.0
			if costMetric, ok := group.Metrics["UnblendedCost"]; ok {
				fmt.Sscanf(costMetric.Amount, "%f", &cost)
			}

			records = append(records, CostRecord{
				ServiceName: serviceName,
				Cost:       cost,
				Currency:   currency,
				Date:       result.TimePeriod.Start,
			})

			totalCost += cost
		}
	}

	return &CostResult{
		Records:    records,
		TotalCost: totalCost,
		Currency:  currency,
	}
}

func (c *CostClient) GetForecast(ctx context.Context) (*CostResult, error) {
	accessKey, secretKey, sessionToken := c.GetCredentials()
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials not configured")
	}

	url := fmt.Sprintf("https://ce.%s.amazonaws.com/", c.Region)

	req := map[string]interface{}{
		"Type":          "FORECAST",
		"Metric":        "UNBLENDED_COST",
		"Granularity":   "MONTHLY",
		"ForecastPeriod": map[string]string{"Value": "1", "Unit": "MONTHS"},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Amz-Target", "AWSInsightsIndexService.GetCostForecast")

	signer := NewSigner(accessKey, secretKey, sessionToken)
	signer.Sign(httpReq, body)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AWS forecast request failed with status: %d", resp.StatusCode)
	}

	var result struct {
		ForecastResults []struct {
			MeanValue string `json:"MeanValue"`
		} `json:"ForecastResults"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var forecastCost float64
	if len(result.ForecastResults) > 0 {
		fmt.Sscanf(result.ForecastResults[0].MeanValue, "%f", &forecastCost)
	}

	return &CostResult{
		TotalCost: forecastCost,
		Currency:  "USD",
	}, nil
}
