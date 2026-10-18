package findings

import (
	"math"
	"os"
	"strings"
	"testing"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func one(t *testing.T, got []Finding, idPart string) Finding {
	t.Helper()
	var match []Finding
	for _, f := range got {
		if f.ResourceID == idPart || strings.Contains(f.ResourceID, idPart) {
			match = append(match, f)
		}
	}
	if len(match) != 1 {
		t.Fatalf("resource %s: got %d findings (%v), want 1", idPart, len(match), ids(got))
	}
	return match[0]
}

func absent(t *testing.T, got []Finding, idPart string) {
	t.Helper()
	for _, f := range got {
		if strings.Contains(f.ResourceID, idPart) || f.Name == idPart {
			t.Fatalf("did not expect finding for %s (%s)", idPart, f.ResourceID)
		}
	}
}

func ids(got []Finding) []string {
	out := make([]string, len(got))
	for i, f := range got {
		out[i] = f.ResourceID
	}
	return out
}

func assertFinding(t *testing.T, f Finding, rule, region string, cost float64, safe bool, createdBy string) {
	t.Helper()
	if f.RuleID != rule {
		t.Errorf("rule = %s, want %s", f.RuleID, rule)
	}
	if f.Region != region {
		t.Errorf("region = %s, want %s", f.Region, region)
	}
	if math.Abs(f.EstimatedMonthlyUSD-cost) > 0.02 {
		t.Errorf("cost = %.4f, want %.2f", f.EstimatedMonthlyUSD, cost)
	}
	if f.SafeToDelete != safe {
		t.Errorf("safe_to_delete = %v, want %v", f.SafeToDelete, safe)
	}
	if f.SafeToDeleteHint == "" {
		t.Error("missing safe_to_delete_hint")
	}
	if f.Evidence == "" {
		t.Error("missing evidence")
	}
	if f.CreatedBy != createdBy {
		t.Errorf("created_by = %q, want %q", f.CreatedBy, createdBy)
	}
	if f.Currency != "USD" {
		t.Errorf("currency = %s", f.Currency)
	}
	if f.Provider != "azure" && f.Provider != "aws" {
		t.Errorf("provider = %s", f.Provider)
	}
}
