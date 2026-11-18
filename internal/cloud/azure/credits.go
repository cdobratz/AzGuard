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

const (
	billingAPIVersion = "2024-04-01"
	lotsAPIVersion    = "2024-08-01"
)

// CreditBalance is the sum of active Microsoft.Consumption credit lots.
type CreditBalance struct {
	Remaining float64
	Currency  string
	Expires   time.Time
	Soonest   bool
	Lots      int
}

// CreditClient reads Azure credit lots. The lots API is documented for
// Microsoft Customer Agreement billing accounts. It is read-only.
type CreditClient struct {
	Token   func() (string, error)
	HTTP    *http.Client
	BaseURL string
}

// NewCreditClient uses the Azure management endpoint.
func NewCreditClient(token func() (string, error)) *CreditClient {
	return &CreditClient{
		Token:   token,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		BaseURL: AzureManagementURL,
	}
}

// Balance lists billing accounts and sums active credit lots.
// An empty billing-account list is an error so the caller can fall back to config.
func (c *CreditClient) Balance(ctx context.Context) (CreditBalance, error) {
	accounts, err := c.listNames(ctx, "/providers/Microsoft.Billing/billingAccounts?api-version="+billingAPIVersion)
	if err != nil {
		return CreditBalance{}, err
	}
	if len(accounts) == 0 {
		return CreditBalance{}, fmt.Errorf("no Microsoft.Billing billing account was returned")
	}
	var out CreditBalance
	var expires []time.Time
	for _, account := range accounts {
		profiles, err := c.listNames(ctx, "/providers/Microsoft.Billing/billingAccounts/"+url.PathEscape(account)+"/billingProfiles?api-version="+billingAPIVersion)
		if err != nil {
			return CreditBalance{}, err
		}
		for _, profile := range profiles {
			path := "/providers/Microsoft.Billing/billingAccounts/" + url.PathEscape(account) +
				"/billingProfiles/" + url.PathEscape(profile) +
				"/providers/Microsoft.Consumption/lots?api-version=" + lotsAPIVersion
			lots, err := c.lots(ctx, path)
			if err != nil {
				return CreditBalance{}, err
			}
			for _, lot := range lots {
				if !lot.active() {
					continue
				}
				out.Remaining += lot.balance()
				out.Lots++
				if out.Currency == "" {
					out.Currency = lot.currency()
				}
				if t, ok := lot.expires(); ok {
					expires = append(expires, t)
				}
			}
		}
	}
	if len(expires) > 0 {
		soonest := expires[0]
		for _, t := range expires[1:] {
			if t.Before(soonest) {
				soonest = t
			}
			if !sameDay(t, expires[0]) {
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

// ExpiresText is the expiration calendar day, or empty when no lot had a date.
func (b CreditBalance) ExpiresText() string {
	if b.Expires.IsZero() {
		return ""
	}
	return b.Expires.UTC().Format("2006-01-02")
}

func sameDay(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

type namedResource struct {
	Name string `json:"name"`
}

func (c *CreditClient) listNames(ctx context.Context, path string) ([]string, error) {
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var page struct {
		Value []namedResource `json:"value"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var names []string
	for _, item := range page.Value {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

type creditLot struct {
	Properties struct {
		Status         string      `json:"status"`
		ExpirationDate string      `json:"expirationDate"`
		CreditCurrency string      `json:"creditCurrency"`
		ClosedBalance  moneyAmount `json:"closedBalance"`
	} `json:"properties"`
}

func (l creditLot) active() bool {
	switch strings.ToLower(strings.TrimSpace(l.Properties.Status)) {
	case "", "active":
		return l.balance() > 0
	default:
		return false
	}
}

func (l creditLot) balance() float64 { return l.Properties.ClosedBalance.Value }

func (l creditLot) currency() string {
	if l.Properties.ClosedBalance.Currency != "" {
		return l.Properties.ClosedBalance.Currency
	}
	return l.Properties.CreditCurrency
}

func (l creditLot) expires() (time.Time, bool) {
	raw := strings.TrimSpace(l.Properties.ExpirationDate)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

type moneyAmount struct {
	Value    float64
	Currency string
}

func (m *moneyAmount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '{' {
		var obj struct {
			Value    float64 `json:"value"`
			Currency string  `json:"currency"`
		}
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		m.Value = obj.Value
		m.Currency = obj.Currency
		return nil
	}
	return json.Unmarshal(data, &m.Value)
}

func (c *CreditClient) lots(ctx context.Context, path string) ([]creditLot, error) {
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var page struct {
		Value []creditLot `json:"value"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse credit lots: %w", err)
	}
	return page.Value, nil
}

func (c *CreditClient) get(ctx context.Context, path string) ([]byte, error) {
	if c.Token == nil {
		return nil, fmt.Errorf("azure token provider is not configured")
	}
	token, err := c.Token()
	if err != nil {
		return nil, fmt.Errorf("azure token: %w", err)
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = AzureManagementURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "azguard")
	client := c.HTTP
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
		return nil, fmt.Errorf("azure credit status %d: %s", resp.StatusCode, msg)
	}
	return body, nil
}
