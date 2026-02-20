package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	AzureManagementURL = "https://management.azure.com"
	CostManagementAPI  = "2023-03-01"
)

type CostClient struct {
	SubscriptionID string
	Token          string
	TokenProvider  func() (string, error)
	HTTPClient     *http.Client
}

func NewCostClient(subscriptionID string, tokenProvider func() (string, error)) *CostClient {
	return &CostClient{
		SubscriptionID: subscriptionID,
		TokenProvider:  tokenProvider,
		HTTPClient:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *CostClient) getToken() (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	return c.TokenProvider()
}

type CostQueryRequest struct {
	Type       string   `json:"type"`
	Timeframe  string   `json:"timeframe"`
	TimePeriod *TimePeriod `json:"timePeriod,omitempty"`
	Dataset    Dataset  `json:"dataset"`
}

type TimePeriod struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Dataset struct {
	Granularity string     `json:"granularity"`
	Aggregation map[string]Aggregation `json:"aggregation"`
	Grouping    []Grouping `json:"grouping"`
}

type Aggregation struct {
	Name     string `json:"name"`
	Function string `json:"function"`
}

type Grouping struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type CostQueryResponse struct {
	Value []CostItem `json:"value"`
}

type CostItem struct {
	ID         string         `json:"id"`
	Name       NameProperty   `json:"name"`
	Properties CostProperties `json:"properties"`
}

type NameProperty struct {
	Value string `json:"value"`
}

type CostProperties struct {
	Cost     float64            `json:"cost"`
	Currency string             `json:"currency"`
	UsageDate UsageDateProperty `json:"usageDate"`
}

type UsageDateProperty struct {
	Value string `json:"value"`
}

type CostQueryResult struct {
	Records []CostRecord
	TotalCost float64
	Currency string
}

type CostRecord struct {
	ServiceName   string
	ResourceGroup string
	Cost          float64
	Currency      string
	Date          string
}

func (c *CostClient) QueryCosts(ctx context.Context, req CostQueryRequest) (*CostQueryResult, error) {
	if c.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription ID is not configured; run 'azguard config set subscription YOUR_SUBSCRIPTION_ID'")
	}

	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.CostManagement/query?api-version=%s",
		AzureManagementURL, c.SubscriptionID, CostManagementAPI)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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
		return nil, fmt.Errorf("cost query failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CostQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return c.parseResponse(result), nil
}

func (c *CostClient) parseResponse(resp CostQueryResponse) *CostQueryResult {
	var records []CostRecord
	var totalCost float64
	currency := "USD"

	for _, item := range resp.Value {
		properties := item.Properties
		record := CostRecord{
			Cost:     properties.Cost,
			Currency: properties.Currency,
			Date:     properties.UsageDate.Value,
		}

		if item.Name.Value != "" {
			record.ServiceName = item.Name.Value
		}

		totalCost += properties.Cost
		if properties.Currency != "" {
			currency = properties.Currency
		}

		records = append(records, record)
	}

	return &CostQueryResult{
		Records:    records,
		TotalCost: totalCost,
		Currency:  currency,
	}
}

func (c *CostClient) QueryCostsByService(ctx context.Context, startDate, endDate string) (*CostQueryResult, error) {
	req := CostQueryRequest{
		Type:      "ActualCost",
		Timeframe: "Custom",
		TimePeriod: &TimePeriod{
			From: startDate,
			To:   endDate,
		},
		Dataset: Dataset{
			Granularity: "Daily",
			Aggregation: map[string]Aggregation{
				"costTotal": {
					Name:     "Cost",
					Function: "Sum",
				},
			},
			Grouping: []Grouping{
				{Type: "Dimension", Name: "ServiceName"},
			},
		},
	}

	return c.QueryCosts(ctx, req)
}

func (c *CostClient) QueryCostsByResourceGroup(ctx context.Context, startDate, endDate string) (*CostQueryResult, error) {
	req := CostQueryRequest{
		Type:      "ActualCost",
		Timeframe: "Custom",
		TimePeriod: &TimePeriod{
			From: startDate,
			To:   endDate,
		},
		Dataset: Dataset{
			Granularity: "Daily",
			Aggregation: map[string]Aggregation{
				"costTotal": {
					Name:     "Cost",
					Function: "Sum",
				},
			},
			Grouping: []Grouping{
				{Type: "Dimension", Name: "ResourceGroup"},
			},
		},
	}

	return c.QueryCosts(ctx, req)
}

func (c *CostClient) GetForecast(ctx context.Context, granularity string) (*CostQueryResult, error) {
	if c.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription ID is not configured; run 'azguard config set subscription YOUR_SUBSCRIPTION_ID'")
	}

	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	url := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.CostManagement/query?api-version=%s",
		AzureManagementURL, c.SubscriptionID, CostManagementAPI)

	forecastReq := CostQueryRequest{
		Type:      "Forecast",
		Timeframe: "BillingMonthToDate",
		Dataset: Dataset{
			Granularity: granularity,
			Aggregation: map[string]Aggregation{
				"costTotal": {
					Name:     "Cost",
					Function: "Sum",
				},
			},
		},
	}

	body, err := json.Marshal(forecastReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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
		return nil, fmt.Errorf("forecast request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CostQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var total float64
	var currency string
	for _, item := range result.Value {
		total += item.Properties.Cost
		if item.Properties.Currency != "" {
			currency = item.Properties.Currency
		}
	}

	if currency == "" {
		currency = "USD"
	}

	return &CostQueryResult{
		TotalCost: total,
		Currency:  currency,
	}, nil
}
