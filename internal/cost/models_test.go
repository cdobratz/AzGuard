package cost

import (
	"strings"
	"testing"
	"time"
)

func TestBillingPeriods(t *testing.T) {
	orig := timeNow
	t.Cleanup(func() { timeNow = orig })

	timeNow = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
	start, end := GetCurrentBillingPeriod()
	if start != "2026-06-15" || end != "2026-07-15" {
		t.Fatalf("billing period %s %s", start, end)
	}
	start, end = GetLastNMonths(3)
	if start != "2026-03-15" || end != "2026-06-15" {
		t.Fatalf("last n %s %s", start, end)
	}
	start, end = GetLastNDays(30)
	if start != "2026-05-16" || end != "2026-06-15" {
		t.Fatalf("last 30 days %s %s", start, end)
	}
	start, end = GetCurrentMonthDateRange()
	if start != "2026-06-01" || end != "2026-07-01" {
		t.Fatalf("june range %s %s", start, end)
	}

	timeNow = func() time.Time { return time.Date(2026, 12, 2, 0, 0, 0, 0, time.Local) }
	start, end = GetCurrentMonthDateRange()
	if start != "2026-12-01" || end != "2027-01-01" {
		t.Fatalf("december range %s %s", start, end)
	}
	if strings.Contains(start, " ") {
		t.Fatalf("date should be a calendar day: %s", start)
	}

	timeNow = func() time.Time { return time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC) }
	start, end = GetLastNDays(30)
	if start != "2025-12-11" || end != "2026-01-10" {
		t.Fatalf("last 30 days across new year %s %s", start, end)
	}
}
