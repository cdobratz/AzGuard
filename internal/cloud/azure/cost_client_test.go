package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQueryCostsAndForecast(t *testing.T) {
	var sawService, sawGroup, sawForecast bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		switch {
		case strings.Contains(body, `"Forecast"`):
			sawForecast = true
			if !strings.Contains(body, "Monthly") {
				t.Errorf("forecast body %s", body)
			}
			_, _ = io.WriteString(w, `{"value":[{"properties":{"cost":3.25,"currency":"USD"}},{"properties":{"cost":1,"currency":""}}]}`)
		case strings.Contains(body, "ResourceGroup"):
			sawGroup = true
			_, _ = io.WriteString(w, `{"value":[{"name":{"value":"rg-a"},"properties":{"cost":2,"currency":"EUR","usageDate":{"value":"2026-11-01"}}}]}`)
		default:
			sawService = true
			if !strings.Contains(body, "ServiceName") || !strings.Contains(body, "2026-11-01") {
				t.Errorf("query body %s", body)
			}
			_, _ = io.WriteString(w, `{"value":[{"name":{"value":"Virtual Machines"},"properties":{"cost":4,"currency":"USD","usageDate":{"value":"2026-11-02"}}},{"properties":{"cost":1,"currency":"USD"}}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	client := fixtureCostClient(t, srv)

	byService, err := client.QueryCostsByService(context.Background(), "2026-11-01", "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	if !sawService || byService.TotalCost != 5 || byService.Currency != "USD" || byService.Records[0].ServiceName != "Virtual Machines" {
		t.Fatalf("service = %+v", byService)
	}
	byGroup, err := client.QueryCostsByResourceGroup(context.Background(), "2026-11-01", "2026-12-01")
	if err != nil || !sawGroup || byGroup.Records[0].ServiceName != "rg-a" || byGroup.Currency != "EUR" {
		t.Fatalf("group = %+v err=%v", byGroup, err)
	}
	forecast, err := client.GetForecast(context.Background(), "Monthly")
	if err != nil || !sawForecast || forecast.TotalCost != 4.25 || forecast.Currency != "USD" {
		t.Fatalf("forecast = %+v err=%v", forecast, err)
	}
}

func TestCostClientFailures(t *testing.T) {
	client := NewCostClient("not-a-guid", func() (string, error) { return "t", nil })
	if _, err := client.QueryCosts(context.Background(), CostQueryRequest{}); err == nil {
		t.Fatal("expected invalid subscription")
	}
	if _, err := client.GetForecast(context.Background(), "Daily"); err == nil {
		t.Fatal("expected invalid subscription")
	}
	client = NewCostClient("11111111-1111-1111-1111-111111111111", func() (string, error) {
		return "", io.EOF
	})
	if _, err := client.QueryCostsByService(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected token error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "Forecast") {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(srv.Close)
	client = fixtureCostClient(t, srv)
	if _, err := client.QueryCostsByService(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected bad json")
	}
	if _, err := client.GetForecast(context.Background(), "Daily"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("forecast error = %v", err)
	}

	preset := NewCostClient("11111111-1111-1111-1111-111111111111", nil)
	preset.Token = "direct"
	if tok, err := preset.getToken(); err != nil || tok != "direct" {
		t.Fatalf("token %s err=%v", tok, err)
	}
}

func fixtureCostClient(t *testing.T, srv *httptest.Server) *CostClient {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewCostClient("11111111-1111-1111-1111-111111111111", nil)
	client.Token = "token"
	client.HTTPClient = &http.Client{Transport: rewriteHost{target: target}}
	return client
}

type rewriteHost struct{ target *url.URL }

func (h rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = h.target.Scheme
	clone.URL.Host = h.target.Host
	clone.Host = h.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
