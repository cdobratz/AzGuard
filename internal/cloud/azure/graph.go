package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const graphAPIVersion = "2021-03-01"

// Reader calls read-only Azure Resource Manager APIs (Resource Graph, usages, retail is separate).
type Reader struct {
	SubscriptionID string
	Token          func() (string, error)
	HTTP           *http.Client
	BaseURL        string
}

// NewReader builds a reader. token may be nil when only parsing fixtures.
func NewReader(subscriptionID string, token func() (string, error)) *Reader {
	return &Reader{
		SubscriptionID: subscriptionID,
		Token:          token,
		HTTP:           &http.Client{Timeout: 60 * time.Second},
		BaseURL:        AzureManagementURL,
	}
}

type graphRequest struct {
	Subscriptions []string          `json:"subscriptions"`
	Query         string            `json:"query"`
	Options       map[string]string `json:"options,omitempty"`
}

type graphPage struct {
	Data      []map[string]any `json:"data"`
	SkipToken string           `json:"$skipToken"`
	SkipAlt   string           `json:"skipToken"`
}

// ParseGraphPage parses one Resource Graph response page.
func ParseGraphPage(body []byte) ([]map[string]any, string, error) {
	var page graphPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, "", fmt.Errorf("parse resource graph: %w", err)
	}
	token := page.SkipToken
	if token == "" {
		token = page.SkipAlt
	}
	return page.Data, token, nil
}

// Query runs a Resource Graph KQL query and follows skip tokens.
func (r *Reader) Query(ctx context.Context, kql string) ([]map[string]any, error) {
	if err := ValidateSubscriptionID(r.SubscriptionID); err != nil {
		return nil, err
	}
	var all []map[string]any
	skip := ""
	for page := 0; page < 20; page++ {
		payload := graphRequest{
			Subscriptions: []string{r.SubscriptionID},
			Query:         kql,
			Options:       map[string]string{"resultFormat": "objectArray"},
		}
		if skip != "" {
			payload.Options["$skipToken"] = skip
		}
		body, err := r.post(ctx, "/providers/Microsoft.ResourceGraph/resources?api-version="+graphAPIVersion, payload)
		if err != nil {
			return nil, err
		}
		rows, next, err := ParseGraphPage(body)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		if next == "" {
			return all, nil
		}
		skip = next
	}
	return all, fmt.Errorf("resource graph: stopped after 20 pages")
}

// WorkspaceIngestionBytes reads the Log Analytics usage API and returns ingested bytes.
func (r *Reader) WorkspaceIngestionBytes(ctx context.Context, resourceID string) (float64, error) {
	if !strings.HasPrefix(resourceID, "/") {
		return 0, fmt.Errorf("workspace id must be an ARM resource id")
	}
	path := resourceID + "/usages?api-version=2022-10-01"
	body, err := r.get(ctx, path)
	if err != nil {
		return 0, err
	}
	return ParseWorkspaceIngestion(body)
}

type usageResponse struct {
	Value []struct {
		Name struct {
			Value string `json:"value"`
		} `json:"name"`
		Unit         string  `json:"unit"`
		CurrentValue float64 `json:"currentValue"`
	} `json:"value"`
}

// ParseWorkspaceIngestion returns billable ingestion bytes from a workspace usages payload.
func ParseWorkspaceIngestion(body []byte) (float64, error) {
	var resp usageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse workspace usages: %w", err)
	}
	var bytes float64
	for _, row := range resp.Value {
		name := strings.ToLower(row.Name.Value)
		if !strings.Contains(name, "ingestion") && !strings.Contains(name, "dataanalyzed") {
			continue
		}
		switch strings.ToLower(row.Unit) {
		case "bytes", "":
			bytes += row.CurrentValue
		case "gb":
			bytes += row.CurrentValue * 1024 * 1024 * 1024
		default:
			bytes += row.CurrentValue
		}
	}
	return bytes, nil
}

func (r *Reader) post(ctx context.Context, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return r.do(req)
}

func (r *Reader) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base()+path, nil)
	if err != nil {
		return nil, err
	}
	return r.do(req)
}

func (r *Reader) base() string {
	if r.BaseURL != "" {
		return strings.TrimRight(r.BaseURL, "/")
	}
	return AzureManagementURL
}

func (r *Reader) do(req *http.Request) ([]byte, error) {
	if r.Token == nil {
		return nil, fmt.Errorf("azure token provider is not configured")
	}
	token, err := r.Token()
	if err != nil {
		return nil, fmt.Errorf("azure token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "azguard")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("azure %s status %d (needs the Reader role): %s", req.URL.Path, resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("azure %s status %d: %s", req.URL.Path, resp.StatusCode, msg)
	}
	return body, nil
}

// ActivityFilter builds an Activity Log OData filter for [since, until].
func ActivityFilter(since, until time.Time) string {
	return fmt.Sprintf("eventTimestamp ge '%s' and eventTimestamp le '%s'",
		since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
}

// ActivityPath is the read-only Activity Log URL path for the subscription.
func (r *Reader) ActivityPath(since, until time.Time) string {
	q := url.Values{}
	q.Set("api-version", "2015-04-01")
	q.Set("$filter", ActivityFilter(since, until))
	return fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Insights/eventtypes/management/values?%s",
		r.SubscriptionID, q.Encode())
}
