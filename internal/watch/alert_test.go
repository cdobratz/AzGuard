package watch

import (
	"os"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/cloud/aws"
	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/cdobratz/azguard/internal/pricing"
)

func TestAlertsFromRecordedFixtures(t *testing.T) {
	book := pricing.Bundled()
	activity := mustRead(t, "testdata/activity.json")
	azEvents, _, err := azure.ParseActivity(activity)
	if err != nil {
		t.Fatal(err)
	}
	az := indexAlerts(AlertsFrom(azEvents, book))

	pip := az["evt-pip"]
	if pip.Principal == "" || !pip.Automation {
		t.Fatalf("public IP alert = %+v", pip)
	}
	if pip.EstimatedMonthlyUSD != 3.65 {
		t.Fatalf("public IP monthly = %v, want 3.65", pip.EstimatedMonthlyUSD)
	}
	if pip.ResourceID == "" || pip.Provider != "azure" {
		t.Fatalf("public IP identity = %+v", pip)
	}
	vm := az["evt-vm-user"]
	if vm.Automation || vm.Principal != "casey@contoso.com" {
		t.Fatalf("vm alert = %+v", vm)
	}
	if vm.EstimatedMonthlyUSD != 7.59 {
		t.Fatalf("vm monthly = %v, want 7.59", vm.EstimatedMonthlyUSD)
	}
	if _, ok := az["evt-fail"]; ok {
		t.Fatal("failed write produced an alert")
	}
	if _, ok := az["evt-read"]; ok {
		t.Fatal("read produced an alert")
	}

	trail := mustRead(t, "testdata/cloudtrail.json")
	awsEvents, _, err := aws.ParseLookupEvents(trail)
	if err != nil {
		t.Fatal(err)
	}
	got := indexAlerts(AlertsFrom(awsEvents, book))

	eip := got["evt-eip"]
	if !eip.Automation || eip.Principal != "ci-bot" || eip.EstimatedMonthlyUSD != 3.65 {
		t.Fatalf("eip alert = %+v", eip)
	}
	if eip.ResourceID != "eipalloc-1" || eip.Region != "us-east-1" {
		t.Fatalf("eip identity = %+v", eip)
	}
	nat := got["evt-nat"]
	if nat.EstimatedMonthlyUSD != 32.85 || nat.ResourceID != "nat-1" {
		t.Fatalf("nat alert = %+v", nat)
	}
	sso := got["evt-sso"]
	if sso.Automation || sso.EstimatedMonthlyUSD != 7.59 || sso.ResourceID != "i-1" {
		t.Fatalf("sso alert = %+v", sso)
	}
	if _, ok := got["evt-gw"]; ok {
		t.Fatal("gateway VPC endpoint produced an alert")
	}
	if _, ok := got["evt-describe"]; ok {
		t.Fatal("DescribeInstances produced an alert")
	}
	if _, ok := got["evt-denied"]; ok {
		t.Fatal("failed RunInstances produced an alert")
	}
}

func TestFormatLineIncludesPrincipalAndCost(t *testing.T) {
	line := FormatLine(Alert{
		At:                  time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
		Provider:            "aws",
		Operation:           "AllocateAddress",
		ResourceID:          "eipalloc-1",
		Principal:           "ci-bot",
		Automation:          true,
		AutomationReason:    "IAM user access key",
		EstimatedMonthlyUSD: 3.65,
	})
	for _, part := range []string{"ci-bot", "automation: IAM user access key", "est=$3.65/month", "eipalloc-1"} {
		if !contains(line, part) {
			t.Fatalf("line %q missing %q", line, part)
		}
	}
}

func indexAlerts(alerts []Alert) map[string]Alert {
	out := map[string]Alert{}
	for _, a := range alerts {
		out[a.ID] = a
	}
	return out
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || indexOf(s, part) >= 0)
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
