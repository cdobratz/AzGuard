package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CostClient struct {
	ProjectID string
	HTTPClient *http.Client
}

func NewCostClient(projectID string) *CostClient {
	return &CostClient{
		ProjectID: projectID,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *CostClient) getToken() (string, error) {
	token := os.Getenv("GOOGLE_AUTH_TOKEN")
	if token != "" {
		return token, nil
	}

	cmd := exec.Command("gcloud", "auth", "print-access-token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get GCP token: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

type CostQueryRequest struct {
	ReportConfig ReportConfig `json:"reportConfig"`
}

type ReportConfig struct {
	TimePeriod     TimePeriod   `json:"timePeriod"`
	Metrics        []string     `json:"metrics"`
	DimensionFilter *[]Dimension `json:"dimensionFilter,omitempty"`
}

type TimePeriod struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type Dimension struct {
	DimensionName string `json:"dimensionName"`
	LabelFilterExpression *LabelFilter `json:"labelFilterExpression,omitempty"`
}

type LabelFilter struct {
	LabelName   string `json:"labelName"`
	StringValue string `json:"stringValue"`
	Comparator  string `json:"comparator"`
}

type CostQueryResponse struct {
	Rows   []Row   `json:"rows"`
	Schema Schema  `json:"schema"`
}

type Row struct {
	DimensionValues []DimensionValue `json:"dimensionValues"`
	MetricValues    []MetricValue   `json:"metricValues"`
}

type DimensionValue struct {
	Value string `json:"value"`
}

type MetricValue struct {
	Value string `json:"value"`
}

type Schema struct {
	Dimensions []DimensionDef `json:"dimensions"`
	Metrics    []MetricDef   `json:"metrics"`
}

type DimensionDef struct {
	Name string `json:"name"`
}

type MetricDef struct {
	Name string `json:"name"`
}

type CostResult struct {
	Records    []CostRecord
	TotalCost float64
	Currency  string
}

type CostRecord struct {
	ServiceName string
	Cost        float64
	Currency    string
	Date        string
}

func (c *CostClient) QueryCosts(ctx context.Context, startDate, endDate string) (*CostResult, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	if c.ProjectID == "" {
		c.ProjectID = os.Getenv("GCP_PROJECT_ID")
	}
	if c.ProjectID == "" {
		return nil, fmt.Errorf("GCP project ID not configured. Set GCP_PROJECT_ID")
	}

	url := fmt.Sprintf("https://cloudbilling.googleapis.com/v1/services/08EF-4734-5792/skus?alt=json")
	_ = url

	costAPIURL := fmt.Sprintf("https://cloudbilling.googleapis.com/v1/projects/%s:getCostInfo", c.ProjectID)

	req := CostQueryRequest{
		ReportConfig: ReportConfig{
			TimePeriod: TimePeriod{
				StartTime: startDate + "T00:00:00Z",
				EndTime:   endDate + "T23:59:59Z",
			},
			Metrics: []string{"cost"},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", costAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GCP Cost API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		CostAmountSum   string `json:"costAmountSum"`
		CurrencyCode    string `json:"currencyCode"`
		UsageStartTime  string `json:"usageStartTime"`
		UsageEndTime    string `json:"usageEndTime"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var totalCost float64
	fmt.Sscanf(result.CostAmountSum, "%f", &totalCost)

	return &CostResult{
		TotalCost: totalCost,
		Currency:  result.CurrencyCode,
		Records: []CostRecord{
			{
				ServiceName: "All Services",
				Cost:        totalCost,
				Currency:    result.CurrencyCode,
				Date:        result.UsageStartTime,
			},
		},
	}, nil
}

func (c *CostClient) GetForecast(ctx context.Context) (*CostResult, error) {
	if c.ProjectID == "" {
		c.ProjectID = os.Getenv("GCP_PROJECT_ID")
	}

	return &CostResult{
		TotalCost: 0,
		Currency:  "USD",
	}, nil
}
