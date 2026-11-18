package cost

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/cdobratz/azguard/internal/storage"
)

func TestServiceSummariesTrendsAndForecast(t *testing.T) {
	bodyOf := func(w http.ResponseWriter, r *http.Request) string {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return ""
		}
		return string(raw)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bodyOf(w, r)
		if raw == "" {
			return
		}
		day := time.Now().UTC().Format("2006-01-02")
		if strings.Contains(raw, `"Forecast"`) {
			_, _ = io.WriteString(w, `{"value":[{"properties":{"cost":12.5,"currency":"USD"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"value":[{"name":{"value":"Virtual Machines"},"properties":{"cost":4.5,"currency":"USD","usageDate":{"value":"`+day+`"}}},{"properties":{"cost":1.25,"currency":"","usageDate":{"value":"`+day+`"}}}]}`)
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	db := newTestDB(t)
	client := azure.NewCostClient("11111111-1111-1111-1111-111111111111", func() (string, error) {
		return "token", nil
	})
	client.HTTPClient = &http.Client{Transport: rewriteHost{target: target}}
	svc := NewService(db, client)
	ctx := context.Background()

	summary, err := svc.GetCurrentCosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalCost != 5.75 || summary.Currency != "USD" {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.ByService["Virtual Machines"] != 4.5 {
		t.Fatalf("by service = %+v", summary.ByService)
	}
	if summary.Forecast == nil || summary.Forecast.NextMonth != 12.5 || summary.Forecast.Confidence != "medium" {
		t.Fatalf("forecast = %+v", summary.Forecast)
	}

	history, err := svc.GetCostHistory(6)
	if err != nil {
		t.Fatal(err)
	}
	if history.TotalCost != 5.75 || len(history.MonthlyBreakdown) == 0 {
		t.Fatalf("history = %+v", history)
	}

	report, err := svc.GenerateReport()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCost != 5.75 || report.Forecast < 0 || len(report.TopServices) != 2 || report.GeneratedAt == "" {
		t.Fatalf("report = %+v", report)
	}

	if err := svc.FetchAndStoreCosts(ctx, "2026-01-01", "2026-02-01"); err != nil {
		t.Fatal(err)
	}
}

func TestForecastFallsBackToLocal(t *testing.T) {
	db := newTestDB(t)
	seedMonths(t, db, []float64{10, 20, 30, 40, 50, 60})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := azure.NewCostClient("11111111-1111-1111-1111-111111111111", nil)
	client.Token = "token"
	client.HTTPClient = &http.Client{Transport: rewriteHost{target: target}}
	svc := NewService(db, client)

	local, err := svc.GetLocalForecast()
	if err != nil {
		t.Fatal(err)
	}
	if local.Confidence != "high" || local.NextMonth < 0 {
		t.Fatalf("local = %+v", local)
	}
	got, err := svc.GetForecast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Confidence != "high" {
		t.Fatalf("api should not be required when local confidence is high: %+v", got)
	}

	trend, err := svc.GetTrendAnalysis()
	if err != nil {
		t.Fatal(err)
	}
	if trend.Trend == "no_data" || trend.CurrentMonth == 0 {
		t.Fatalf("trend = %+v", trend)
	}
}

func TestTrendDirectionsAndLowConfidenceForecast(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db, failingAzure())

	empty, err := svc.GetTrendAnalysis()
	if err != nil || empty.Trend != "no_data" {
		t.Fatalf("empty = %+v err=%v", empty, err)
	}
	low, err := svc.GetLocalForecast()
	if err != nil || low.Confidence != "low" || low.NextMonth != 0 {
		t.Fatalf("low = %+v err=%v", low, err)
	}
	got, err := svc.GetForecast(context.Background())
	if err != nil || got.Confidence != "low" {
		t.Fatalf("forecast with failed api = %+v err=%v", got, err)
	}

	seedMonths(t, db, []float64{80, 10})
	up, err := svc.GetTrendAnalysis()
	if err != nil {
		t.Fatal(err)
	}
	if up.Trend != "increasing" || up.ChangePercent <= 5 {
		t.Fatalf("increasing = %+v", up)
	}

	db = newTestDB(t)
	svc = NewService(db, failingAzure())
	seedMonths(t, db, []float64{10, 80})
	down, err := svc.GetTrendAnalysis()
	if err != nil {
		t.Fatal(err)
	}
	if down.Trend != "decreasing" || down.ChangePercent >= -5 {
		t.Fatalf("decreasing = %+v", down)
	}

	db = newTestDB(t)
	svc = NewService(db, azureReturning(`{"value":[{"properties":{"cost":9,"currency":"USD"}}]}`))
	seedMonths(t, db, []float64{100, 98})
	stable, err := svc.GetTrendAnalysis()
	if err != nil {
		t.Fatal(err)
	}
	if stable.Trend != "stable" {
		t.Fatalf("stable = %+v", stable)
	}
	medium, err := svc.GetLocalForecast()
	if err != nil || (medium.Confidence != "low" && medium.Confidence != "medium") {
		t.Fatalf("two months should not be high confidence: %+v err=%v", medium, err)
	}
	apiForecast, err := svc.GetForecast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if apiForecast.Confidence != "medium" || apiForecast.NextMonth != 9 {
		t.Fatalf("api forecast = %+v", apiForecast)
	}

	one := newTestDB(t)
	seedMonths(t, one, []float64{15})
	only, err := NewService(one, failingAzure()).GetTrendAnalysis()
	if err != nil || only.PreviousMonth != 0 || only.Trend != "stable" {
		t.Fatalf("single month = %+v err=%v", only, err)
	}
}

func TestCostSummaryGroupsAndReportPeriod(t *testing.T) {
	db := newTestDB(t)
	if err := db.SaveCostRecords([]storage.CostRecord{
		{SubscriptionID: "sub", ResourceGroup: "rg-a", ServiceName: "Storage", Cost: 2, Currency: "USD", Date: "2026-05-02"},
		{SubscriptionID: "sub", ResourceGroup: "rg-b", ServiceName: "Storage", Cost: 3, Currency: "USD", Date: "2026-05-03"},
		{SubscriptionID: "sub", ResourceGroup: "rg-a", ServiceName: "Functions", Cost: 1, Currency: "USD", Date: "2026-04-01"},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, failingAzure())
	summary, err := svc.GetCostSummary(CostFilter{StartDate: "2026-05-01", EndDate: "2026-05-31"})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalCost != 5 || summary.ByService["Storage"] != 5 || summary.ByResourceGroup["rg-a"] != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	if !strings.Contains(summary.Period, "2026-05-01") {
		t.Fatalf("period %s", summary.Period)
	}
	report, err := svc.GenerateReport()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalCost != 6 || len(report.MonthlyData) == 0 || report.Period == "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestFetchRejectsBadSubscriptionAndToken(t *testing.T) {
	db := newTestDB(t)
	bad := azure.NewCostClient("not-a-guid", func() (string, error) { return "t", nil })
	if err := NewService(db, bad).FetchAndStoreCosts(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected invalid subscription")
	}
	broken := azure.NewCostClient("11111111-1111-1111-1111-111111111111", func() (string, error) {
		return "", io.EOF
	})
	if err := NewService(db, broken).FetchAndStoreCosts(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected token error")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := azure.NewCostClient("11111111-1111-1111-1111-111111111111", nil)
	client.Token = "token"
	client.HTTPClient = &http.Client{Transport: rewriteHost{target: target}}
	if err := NewService(db, client).FetchAndStoreCosts(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected bad json")
	}
}

func newTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "costs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedMonths(t *testing.T, db *storage.DB, newestFirst []float64) {
	t.Helper()
	now := time.Now().UTC()
	var records []storage.CostRecord
	for i, cost := range newestFirst {
		day := time.Date(now.Year(), now.Month(), 2, 0, 0, 0, 0, time.UTC).AddDate(0, -i, 0)
		records = append(records, storage.CostRecord{
			SubscriptionID: "sub",
			ResourceGroup:  "rg",
			ServiceName:    "Virtual Machines",
			Cost:           cost,
			Currency:       "USD",
			Date:           day.Format("2006-01-02"),
		})
	}
	if err := db.SaveCostRecords(records); err != nil {
		t.Fatal(err)
	}
}

func failingAzure() *azure.CostClient {
	return azure.NewCostClient("not-a-guid", func() (string, error) { return "", io.EOF })
}

func azureReturning(payload string) *azure.CostClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	target, _ := url.Parse(srv.URL)
	client := azure.NewCostClient("11111111-1111-1111-1111-111111111111", nil)
	client.Token = "token"
	client.HTTPClient = &http.Client{Transport: rewriteHost{target: target}}
	return client
}

type rewriteHost struct {
	target *url.URL
}

func (h rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = h.target.Scheme
	clone.URL.Host = h.target.Host
	clone.Host = h.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}
