package findings

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOutputFormatsAndFailOver(t *testing.T) {
	found := []Finding{
		makeFinding(Finding{
			ResourceID:          "eipalloc-idle",
			Name:                "203.0.113.10",
			Provider:            "aws",
			Region:              "us-east-1",
			RuleID:              "aws.eip.unassociated",
			Evidence:            "idle, has a comma",
			EstimatedMonthlyUSD: 3.65,
			CreatedBy:           "ci-bot",
			SafeToDelete:        true,
			SafeToDeleteHint:    "release it",
			PriceSource:         "bundled",
		}),
		makeFinding(Finding{
			ResourceID:          "eipalloc-idle",
			Name:                "203.0.113.10",
			Provider:            "aws",
			Region:              "us-east-1",
			RuleID:              "aws.ipv4.public",
			Evidence:            "same address",
			EstimatedMonthlyUSD: 3.65,
			CreatedBy:           "ci-bot",
			SafeToDelete:        true,
			SafeToDeleteHint:    "release it",
			PriceSource:         "bundled",
		}),
	}
	report := NewReport(found, []string{"azure skipped"}, time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	if report.Schema != SchemaName || report.TotalEstimatedMonthlyUSD != 3.65 {
		t.Fatalf("schema %s total %.2f", report.Schema, report.TotalEstimatedMonthlyUSD)
	}
	if !Exceeds(report.TotalEstimatedMonthlyUSD, 1) {
		t.Fatal("expected fail-over")
	}
	if Exceeds(report.TotalEstimatedMonthlyUSD, 3.65) {
		t.Fatal("equal to the limit is not over")
	}
	if Exceeds(report.TotalEstimatedMonthlyUSD, -1) {
		t.Fatal("negative limit is disabled")
	}
	err := (&FailOverError{Total: 3.65, Limit: 1}).Error()
	if !strings.Contains(err, "3.65") || (&FailOverError{}).ExitCode() != 2 {
		t.Fatalf("fail-over error: %s", err)
	}

	var buf bytes.Buffer
	if err := Write(&buf, report, "json"); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Findings[0].ResourceID == "" || decoded.Findings[0].RuleID == "" || decoded.Findings[0].Evidence == "" {
		t.Fatalf("json missing fields: %+v", decoded.Findings[0])
	}
	if decoded.Findings[0].CreatedBy != "ci-bot" || !decoded.Findings[0].SafeToDelete {
		t.Fatalf("json finding = %+v", decoded.Findings[0])
	}

	buf.Reset()
	if err := Write(&buf, report, "csv"); err != nil {
		t.Fatal(err)
	}
	csvText := buf.String()
	for _, col := range []string{"resource_id", "rule_id", "evidence", "estimated_monthly_usd", "created_by", "safe_to_delete", "safe_to_delete_hint"} {
		if !strings.Contains(csvText, col) {
			t.Errorf("csv missing column %s", col)
		}
	}
	if !strings.Contains(csvText, "\"idle, has a comma\"") {
		t.Fatalf("csv did not quote evidence: %s", csvText)
	}

	buf.Reset()
	if err := Write(&buf, report, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "aws.eip.unassociated") || !strings.Contains(buf.String(), "counted once") {
		t.Fatalf("table = %s", buf.String())
	}
	if err := Write(&buf, report, "yaml"); err == nil {
		t.Fatal("expected unknown format error")
	}
}

func TestSchemaMentionsRequiredFields(t *testing.T) {
	for _, field := range []string{"resource_id", "rule_id", "evidence", "estimated_monthly_usd", "created_by", "safe_to_delete", "safe_to_delete_hint", "price_source"} {
		if !strings.Contains(JSONSchema, field) {
			t.Errorf("schema missing %s", field)
		}
	}
}
