package account

import (
	"strings"
	"testing"
)

func TestFourAccountStates(t *testing.T) {
	payg := FormatAzure(Profile{Kind: AzurePAYG}, 40)
	if !strings.Contains(payg, "No spending limit") || !strings.Contains(payg, "Pay-As-You-Go") {
		t.Fatalf("payg = %q", payg)
	}
	assertNoTwoHundred(t, payg)

	free := FormatAzure(Profile{Kind: AzureFree, SpendingLimit: boolPtr(true), CreditRemaining: floatPtr(50), CreditExpires: "2026-12-31"}, 10)
	if !strings.Contains(free, "Azure free account") || !strings.Contains(free, "Spending limit: on") || !strings.Contains(free, "50.00") || !strings.Contains(free, "2026-12-31") {
		t.Fatalf("azure free = %q", free)
	}
	assertNoTwoHundred(t, free)

	paid := FormatAWS(Profile{Kind: AWSPaid, SpendingLimit: boolPtr(false)}, 12.5)
	if !strings.Contains(paid, "Paid plan") || !strings.Contains(paid, "Charges apply") {
		t.Fatalf("paid = %q", paid)
	}
	assertNoTwoHundred(t, paid)

	awsFree := FormatAWS(Profile{Kind: AWSFree, CreditRemaining: floatPtr(80), CreditExpires: "2027-01-15"}, 0)
	if !strings.Contains(awsFree, "Free plan") || !strings.Contains(awsFree, "https://aws.amazon.com/free/") || !strings.Contains(awsFree, "80.00") {
		t.Fatalf("aws free = %q", awsFree)
	}
	assertNoTwoHundred(t, awsFree)
}

func TestParseAzureAccount(t *testing.T) {
	payg, err := ParseAzureAccount([]byte(`{"subscriptionPolicies":{"quotaId":"PayAsYouGo_2014-09-01","spendingLimit":"Off"}}`))
	if err != nil || payg.Kind != AzurePAYG || payg.SpendingLimit == nil || *payg.SpendingLimit {
		t.Fatalf("payg = %+v err=%v", payg, err)
	}
	trial, err := ParseAzureAccount([]byte(`{"subscriptionPolicies":{"quotaId":"FreeTrial_2014-09-01","spendingLimit":"On"}}`))
	if err != nil || trial.Kind != AzureFree || trial.SpendingLimit == nil || !*trial.SpendingLimit {
		t.Fatalf("trial = %+v err=%v", trial, err)
	}
}

func assertNoTwoHundred(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, "200") || strings.Contains(text, "% of") || strings.Contains(strings.ToLower(text), "free tier") {
		t.Fatalf("status still describes a $200 free tier:\n%s", text)
	}
}

func boolPtr(v bool) *bool        { return &v }
func floatPtr(v float64) *float64 { return &v }
