package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBillingEndpoint = "https://billing.us-east-1.amazonaws.com/"
	defaultSTSEndpoint     = "https://sts.amazonaws.com/"
)

// CreditBalance is the sum of remaining AWS credits from Billing GetCredits.
type CreditBalance struct {
	Remaining float64
	Currency  string
	Expires   time.Time
	Count     int
	Soonest   bool
}

// BillingClient calls the read-only STS GetCallerIdentity and Billing GetCredits APIs.
type BillingClient struct {
	AccessKey       string
	SecretKey       string
	Token           string
	HTTP            *http.Client
	Now             func() time.Time
	BillingEndpoint string
	STSEndpoint     string
}

// BillingFromCost reuses credentials already resolved for the cost client.
func BillingFromCost(c *CostClient) BillingClient {
	b := BillingClient{Now: time.Now}
	if c == nil {
		b.HTTP = &http.Client{Timeout: 60 * time.Second}
		return b
	}
	b.AccessKey = c.AccessKey
	b.SecretKey = c.SecretKey
	b.Token = c.Token
	if c.HTTP != nil {
		b.HTTP = c.HTTP
	} else {
		b.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	return b
}

func (b BillingClient) enabled() bool {
	return b.AccessKey != "" && b.SecretKey != ""
}

func (b BillingClient) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

func (b BillingClient) billingEndpoint() string {
	if b.BillingEndpoint != "" {
		return b.BillingEndpoint
	}
	return defaultBillingEndpoint
}

func (b BillingClient) stsEndpoint() string {
	if b.STSEndpoint != "" {
		return b.STSEndpoint
	}
	return defaultSTSEndpoint
}

// AccountID calls STS GetCallerIdentity and returns the 12-digit account id.
func (b BillingClient) AccountID(ctx context.Context) (string, error) {
	if !b.enabled() {
		return "", fmt.Errorf("AWS credentials not configured")
	}
	form := url.Values{}
	form.Set("Action", "GetCallerIdentity")
	form.Set("Version", "2011-06-15")
	payload := form.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.stsEndpoint(), strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	signV4(req, []byte(payload), b.AccessKey, b.SecretKey, b.Token, "us-east-1", "sts", b.now())
	body, err := b.do(req)
	if err != nil {
		return "", err
	}
	return ParseAccountID(body)
}

// GetCredits calls AWS Billing GetCredits for accountID.
// startDate is 364 days before now and endDate is one minute before now,
// which matches the API rule that endDate is not in the future and startDate
// is not more than one year ago.
func (b BillingClient) GetCredits(ctx context.Context, accountID string) (CreditBalance, error) {
	if !b.enabled() {
		return CreditBalance{}, fmt.Errorf("AWS credentials not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if len(accountID) != 12 {
		return CreditBalance{}, fmt.Errorf("AWS account id must be 12 digits")
	}
	now := b.now()
	payload, err := json.Marshal(map[string]any{
		"accountId": accountID,
		"startDate": now.Add(-364 * 24 * time.Hour).Unix(),
		"endDate":   now.Add(-time.Minute).Unix(),
	})
	if err != nil {
		return CreditBalance{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.billingEndpoint(), bytes.NewReader(payload))
	if err != nil {
		return CreditBalance{}, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AWSBilling.GetCredits")
	signV4(req, payload, b.AccessKey, b.SecretKey, b.Token, "us-east-1", "billing", b.now())
	body, err := b.do(req)
	if err != nil {
		return CreditBalance{}, err
	}
	return ParseCredits(body)
}

// RemainingCredit reads the caller account and its remaining credits.
func (b BillingClient) RemainingCredit(ctx context.Context) (CreditBalance, error) {
	id, err := b.AccountID(ctx)
	if err != nil {
		return CreditBalance{}, err
	}
	return b.GetCredits(ctx, id)
}

func (b BillingClient) do(req *http.Request) ([]byte, error) {
	client := b.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return nil, fmt.Errorf("AWS status %d: %s", resp.StatusCode, msg)
	}
	return body, nil
}

// ParseAccountID reads the Account element from GetCallerIdentity XML.
func ParseAccountID(body []byte) (string, error) {
	var resp struct {
		Result struct {
			Account string `xml:"Account"`
		} `xml:"GetCallerIdentityResult"`
	}
	if err := xml.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse GetCallerIdentity: %w", err)
	}
	id := strings.TrimSpace(resp.Result.Account)
	if len(id) != 12 {
		return "", fmt.Errorf("parse GetCallerIdentity: account id %q", id)
	}
	return id, nil
}

// ParseCredits sums remainingAmount and keeps the soonest endDate.
func ParseCredits(body []byte) (CreditBalance, error) {
	var resp struct {
		Credits []struct {
			Remaining struct {
				Amount   string `json:"currencyAmount"`
				Currency string `json:"currencyCode"`
			} `json:"remainingAmount"`
			EndDate json.RawMessage `json:"endDate"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return CreditBalance{}, fmt.Errorf("parse GetCredits: %w", err)
	}
	var out CreditBalance
	var expires []time.Time
	for _, credit := range resp.Credits {
		if strings.TrimSpace(credit.Remaining.Amount) == "" {
			continue
		}
		amount, err := strconv.ParseFloat(credit.Remaining.Amount, 64)
		if err != nil {
			return CreditBalance{}, fmt.Errorf("parse GetCredits amount: %w", err)
		}
		if amount <= 0 {
			continue
		}
		out.Remaining += amount
		out.Count++
		if out.Currency == "" {
			out.Currency = credit.Remaining.Currency
		}
		if t, ok := parseEpoch(credit.EndDate); ok {
			expires = append(expires, t)
		}
	}
	if len(expires) > 0 {
		soonest := expires[0]
		for _, t := range expires[1:] {
			if t.Before(soonest) {
				soonest = t
			}
			if !t.Equal(expires[0]) {
				out.Soonest = true
			}
		}
		out.Expires = soonest.UTC()
	}
	if out.Currency == "" {
		out.Currency = "USD"
	}
	return out, nil
}

// ExpiresText is the expiration calendar day, or empty when the API did not send one.
func (b CreditBalance) ExpiresText() string {
	if b.Expires.IsZero() {
		return ""
	}
	return b.Expires.UTC().Format("2006-01-02")
}

func parseEpoch(raw json.RawMessage) (time.Time, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return time.Unix(int64(n), 0).UTC(), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, err := strconv.ParseFloat(s, 64); err == nil && n > 0 {
			return time.Unix(int64(n), 0).UTC(), true
		}
	}
	return time.Time{}, false
}
