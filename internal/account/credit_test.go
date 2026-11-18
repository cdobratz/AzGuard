package account

import (
	"strings"
	"testing"
	"time"
)

func TestDaysLeft(t *testing.T) {
	now := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	days, date, ok := DaysLeft("2026-12-01", now)
	if !ok || days != 13 || date != "2026-12-01" {
		t.Fatalf("future %d %s %v", days, date, ok)
	}
	days, _, ok = DaysLeft("2026-11-18T23:00:00Z", now)
	if !ok || days != 0 {
		t.Fatalf("today %d %v", days, ok)
	}
	days, date, ok = DaysLeft("2026-11-01", now)
	if !ok || days >= 0 || date != "2026-11-01" {
		t.Fatalf("past %d %s %v", days, date, ok)
	}
	if _, _, ok = DaysLeft("next month", now); ok {
		t.Fatal("expected unparseable date")
	}
	if _, _, ok = DaysLeft("", now); ok {
		t.Fatal("expected empty date")
	}
}

func TestApplyCreditPrefersAPIAndKeepsPaidFlat(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	stored := Profile{Kind: AzurePAYG, CreditRemaining: floatPtr(10), CreditExpires: "2026-01-01"}
	got := ApplyCredit(stored, stored, APICredit{
		Available: true,
		Remaining: 42.5,
		Expires:   "2026-12-01T00:00:00Z",
		Source:    "azure-consumption",
	})
	text := FormatAzureAt(got, 40, now)
	if !strings.Contains(text, "Pay-As-You-Go") || !strings.Contains(text, "42.50") || !strings.Contains(text, "Days left: 13") {
		t.Fatalf("payg credit:\n%s", text)
	}
	if !strings.Contains(text, "Credit source: azure-consumption") || !strings.Contains(text, "Config also stores") {
		t.Fatalf("source:\n%s", text)
	}
	assertNoTwoHundred(t, text)

	paid := ApplyCredit(Profile{Kind: AWSPaid}, Profile{Kind: AWSPaid, CreditRemaining: floatPtr(80)}, APICredit{Note: "AWS credit was not read from the Billing GetCredits API."})
	paidText := FormatAWSAt(paid, 12, now)
	if !strings.Contains(paidText, "Paid plan") || !strings.Contains(paidText, "80.00") || !strings.Contains(paidText, "Credit source: config") || !strings.Contains(paidText, "GetCredits") {
		t.Fatalf("paid config:\n%s", paidText)
	}
	assertNoTwoHundred(t, paidText)

	empty := ApplyCredit(Profile{Kind: AWSPaid}, Profile{Kind: AWSPaid}, APICredit{Note: "nothing stored"})
	emptyText := FormatAWSAt(empty, 1, now)
	if strings.Contains(emptyText, "Credit remaining") || !strings.Contains(emptyText, "Note: nothing stored") {
		t.Fatalf("empty:\n%s", emptyText)
	}
	assertNoTwoHundred(t, emptyText)
}

func TestFreeCreditDaysFromConfig(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	free := ApplyCredit(Profile{Kind: AzureFree}, Profile{Kind: AzureFree, CreditRemaining: floatPtr(50), CreditExpires: "2026-11-01"}, APICredit{Note: "config only"})
	text := FormatAzureAt(free, 3, now)
	if !strings.Contains(text, "50.00") || !strings.Contains(text, "Days left: 0 (expired 2026-11-01)") || !strings.Contains(text, "Credit source: config") {
		t.Fatalf("expired:\n%s", text)
	}
	awsFree := FormatAWSAt(Profile{Kind: AWSFree, CreditRemaining: floatPtr(80), CreditExpires: "2027-01-15", CreditSource: "aws-billing"}, 0, now)
	if !strings.Contains(awsFree, "80.00") || !strings.Contains(awsFree, "Days left: 58") || !strings.Contains(awsFree, "aws-billing") {
		t.Fatalf("aws free:\n%s", awsFree)
	}
	assertNoTwoHundred(t, awsFree)
}
