package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteCostConfigAndAlerts(t *testing.T) {
	db := openTestDB(t)

	if err := db.SetConfig("azure.subscription_id", "sub-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConfig("azure.subscription_id", "sub-2"); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetConfig("azure.subscription_id")
	if err != nil || got != "sub-2" {
		t.Fatalf("config %q err=%v", got, err)
	}
	missing, err := db.GetConfig("missing")
	if err != nil || missing != "" {
		t.Fatalf("missing %q err=%v", missing, err)
	}

	records := []CostRecord{
		{SubscriptionID: "sub", ResourceGroup: "rg-a", ServiceName: "Storage", Cost: 1.5, Currency: "USD", Date: "2026-05-01"},
		{SubscriptionID: "sub", ResourceGroup: "rg-b", ServiceName: "Functions", Cost: 2.5, Currency: "USD", Date: "2026-05-15"},
		{SubscriptionID: "sub", ResourceGroup: "rg-a", ServiceName: "Storage", Cost: 1, Currency: "USD", Date: "2026-04-01"},
	}
	if err := db.SaveCostRecord(records[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCostRecords(records[1:]); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCostRecords(nil); err != nil {
		t.Fatal(err)
	}

	all, err := db.GetCostRecords(CostFilter{})
	if err != nil || len(all) != 3 {
		t.Fatalf("all = %+v err=%v", all, err)
	}
	filtered, err := db.GetCostRecords(CostFilter{StartDate: "2026-05-01", EndDate: "2026-05-31", ServiceName: "Functions"})
	if err != nil || len(filtered) != 1 || filtered[0].Cost != 2.5 {
		t.Fatalf("filtered = %+v err=%v", filtered, err)
	}

	byService, err := db.GetAggregatedCosts(CostFilter{StartDate: "2026-05-01", EndDate: "2026-05-31", GroupBy: "ServiceName"})
	if err != nil || byService["Storage"] != 1.5 || byService["Functions"] != 2.5 {
		t.Fatalf("by service = %+v err=%v", byService, err)
	}
	byRG, err := db.GetAggregatedCosts(CostFilter{GroupBy: "ResourceGroup"})
	if err != nil || byRG["rg-a"] != 2.5 || byRG["rg-b"] != 2.5 {
		t.Fatalf("by group = %+v err=%v", byRG, err)
	}
	total, err := db.GetTotalCost(CostFilter{StartDate: "2026-05-01", EndDate: "2026-05-31"})
	if err != nil || total != 4 {
		t.Fatalf("total %v err=%v", total, err)
	}
	none, err := db.GetTotalCost(CostFilter{StartDate: "1999-01-01", EndDate: "1999-01-02"})
	if err != nil || none != 0 {
		t.Fatalf("empty total %v err=%v", none, err)
	}

	months, err := db.GetMonthlyCosts(120)
	if err != nil || len(months) < 2 {
		t.Fatalf("months = %+v err=%v", months, err)
	}

	if err := db.SaveAlert(Alert{Name: "budget-5", Threshold: 5, SubscriptionID: "sub", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAlert(Alert{Name: "budget-1", Threshold: 1, SubscriptionID: "sub", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	alerts, err := db.GetAlerts()
	if err != nil || len(alerts) != 2 {
		t.Fatalf("alerts = %+v err=%v", alerts, err)
	}
	one, err := db.GetAlertByName("budget-5")
	if err != nil || one == nil || !one.Enabled || one.Threshold != 5 {
		t.Fatalf("alert = %+v err=%v", one, err)
	}
	absent, err := db.GetAlertByName("nope")
	if err != nil || absent != nil {
		t.Fatalf("absent = %+v err=%v", absent, err)
	}
	if err := db.DeleteAlert("budget-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAlert("budget-1"); err != nil {
		t.Fatal(err)
	}
	left, err := db.GetAlerts()
	if err != nil || len(left) != 1 || left[0].Name != "budget-5" {
		t.Fatalf("left = %+v err=%v", left, err)
	}
}

func TestNewRejectsFileAsDirectory(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(parent, "child.db")); err == nil {
		t.Fatal("expected mkdir to fail")
	}
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
