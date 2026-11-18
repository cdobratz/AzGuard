package cleanup

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/findings"
)

func TestWriteFormats(t *testing.T) {
	res := Result{
		DryRun: true,
		WouldDelete: []Item{{
			ResourceID: "vol-1", Type: "aws.ebs.unattached", Region: "us-east-1", Age: "40d",
			EstimatedMonthlyUSD: 3.5, PriceSource: "unpriced", Hint: "Nothing is attached.",
		}},
		Deleted:         []Item{{ResourceID: "disk-1", Type: "azure.disk.unattached", Region: "eastus", Age: "10d", EstimatedMonthlyUSD: 5}},
		Skipped:         []Item{{ResourceID: "vol-keep", Type: "aws.ebs.unattached", Reason: "protected by tag azguard:keep=true"}},
		Failed:          []Item{{ResourceID: "vol-bad", Type: "aws.ebs.unattached", Reason: "access denied", PriceSource: "unpriced"}},
		DiscoveryErrors: []string{"aws volumes: status 403"},
	}
	var table bytes.Buffer
	if err := Write(&table, res, "table"); err != nil {
		t.Fatal(err)
	}
	text := table.String()
	for _, want := range []string{"Dry run", "WOULD DELETE", "vol-1", "hint: Nothing is attached.", "DELETED", "SKIPPED", "FAILED", "n/a", "discovery:", "Summary:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("table missing %q\n%s", want, text)
		}
	}

	res.Note = "Confirmation was not given. Nothing was deleted."
	res.DryRun = false
	var noted bytes.Buffer
	if err := Write(&noted, res, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(noted.String(), res.Note) {
		t.Fatalf("note:\n%s", noted.String())
	}

	applied := Result{Applied: true, Deleted: []Item{{ResourceID: "vol-1", Type: "ebs", Age: "1d", EstimatedMonthlyUSD: 1}}}
	var appliedBuf bytes.Buffer
	if err := Write(&appliedBuf, applied, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(appliedBuf.String(), "Cleanup applied.") {
		t.Fatalf("applied:\n%s", appliedBuf.String())
	}

	var js bytes.Buffer
	if err := Write(&js, res, "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js.String(), `"resource_id": "vol-1"`) {
		t.Fatalf("json %s", js.String())
	}
	var csvBuf bytes.Buffer
	if err := Write(&csvBuf, res, "csv"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvBuf.String(), "would-delete,vol-1") || !strings.Contains(csvBuf.String(), "failed,vol-bad") {
		t.Fatalf("csv %s", csvBuf.String())
	}
	if err := Write(&bytes.Buffer{}, res, "yaml"); err == nil {
		t.Fatal("expected unknown format")
	}

	empty := Result{}
	var nothing bytes.Buffer
	if err := Write(&nothing, empty, "table"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nothing.String(), "Nothing was deleted.") {
		t.Fatalf("empty:\n%s", nothing.String())
	}
}

func TestAgeAndEveryDeleteAction(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if FormatAge(time.Time{}, now) != "unknown" {
		t.Fatal("unknown age")
	}
	if FormatAge(now.Add(time.Hour), now) != "0m" {
		t.Fatal("future")
	}
	if FormatAge(now.Add(-30*time.Second), now) != "0m" || FormatAge(now.Add(-12*time.Minute), now) != "12m" || FormatAge(now.Add(-5*time.Hour), now) != "5h" || FormatAge(now.Add(-40*24*time.Hour), now) != "40d" {
		t.Fatal("age buckets")
	}
	if _, err := ParseOlderThan(""); err == nil {
		t.Fatal("empty duration")
	}
	if _, err := ParseOlderThan("-1d"); err == nil {
		t.Fatal("negative days")
	}
	if _, err := ParseOlderThan("-2h"); err == nil {
		t.Fatal("negative hours")
	}
	hours, err := ParseOlderThan("720h")
	if err != nil || hours != 720*time.Hour {
		t.Fatalf("hours %s %v", hours, err)
	}
	if _, _, err := ParseTagFlag("novalue"); err == nil {
		t.Fatal("expected tag error")
	}
	tags := map[string]string{}
	if err := PutTag(tags, "Env", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := PutTag(tags, "env", "dev"); err != nil {
		t.Fatal(err)
	}
	if err := PutTag(tags, "ENV", "prod"); err == nil {
		t.Fatal("expected conflicting tag")
	}
	if !Protected(map[string]string{"AzGuard:Keep": "1"}) || Protected(nil) || Protected(map[string]string{"other": "true"}) {
		t.Fatal("protect tag")
	}

	found := findingsSample()
	res := Plan(found, Options{Now: now, IncludeUnsafe: true})
	if len(res.WouldDelete) < 8 {
		t.Fatalf("would delete %d", len(res.WouldDelete))
	}
	actions := map[string]string{}
	for _, item := range res.WouldDelete {
		actions[item.ResourceID] = item.Action
	}
	want := map[string]string{
		"disk-1":     ActionAzureDelete,
		"pip-1":      ActionAzureDelete,
		"nic-1":      ActionAzureDelete,
		"vm-1":       ActionAzureDelete,
		"plan-1":     ActionAzureDelete,
		"snap-az":    ActionAzureDelete,
		"nat-az":     ActionAzureDelete,
		"law-1":      ActionAzureDelete,
		"vol-1":      ActionAWSDeleteVolume,
		"eipalloc-1": ActionAWSReleaseAddress,
		"nat-aws":    ActionAWSDeleteNatGateway,
		"vpce-1":     ActionAWSDeleteVpcEndpoints,
		"i-1":        ActionAWSTerminateInstances,
		"snap-aws":   ActionAWSDeleteSnapshot,
		"arn:elb":    ActionAWSDeleteLoadBalancer,
	}
	for id, action := range want {
		if actions[id] != action {
			t.Fatalf("%s action %q, want %q", id, actions[id], action)
		}
	}
	skipped := Plan(found, Options{Now: now})
	for _, item := range skipped.Skipped {
		if item.ResourceID == "vm-1" && !strings.Contains(item.Reason, "include-unsafe") {
			t.Fatalf("vm reason %s", item.Reason)
		}
	}
	auto := Plan([]findings.Finding{{
		ResourceID: "203.0.113.8", Provider: "aws", Region: "us-east-1", RuleID: "aws.ipv4.public",
		SafeToDelete: true, Tags: map[string]string{},
	}}, Options{Now: now})
	if len(auto.WouldDelete) != 0 || !strings.Contains(auto.Skipped[0].Reason, "no delete operation") {
		t.Fatalf("auto ip %+v", auto.Skipped)
	}

	typed := Plan(found, Options{Now: now, IncludeUnsafe: true, Types: []string{"disk", "load-balancer", "aws.snapshot.old"}})
	gotIDs := map[string]bool{}
	for _, item := range typed.WouldDelete {
		gotIDs[item.ResourceID] = true
	}
	if !gotIDs["disk-1"] || !gotIDs["arn:elb"] || !gotIDs["snap-aws"] || gotIDs["vol-1"] {
		t.Fatalf("type filter %+v", gotIDs)
	}

	ok, err := Proceed(Options{Apply: true, Confirm: func(int) (bool, error) { return false, nil }}, 1)
	if err != nil || ok {
		t.Fatalf("confirm false %v %v", ok, err)
	}
	if _, err := Proceed(Options{Apply: true, Confirm: func(int) (bool, error) { return false, errors.New("no") }}, 1); err == nil {
		t.Fatal("expected confirm error")
	}
	if _, err := Run(context.Background(), []findings.Finding{volume("vol-1", "us-east-1", now.Add(-48*time.Hour), map[string]string{}, 1)}, Options{Apply: true, Yes: true, Now: now}, nil); err == nil {
		t.Fatal("expected missing deleter")
	}
}

func findingsSample() []findings.Finding {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tags := map[string]string{}
	row := func(id, provider, rule string, safe bool) findings.Finding {
		return findings.Finding{
			ResourceID: id, Provider: provider, Region: "us-east-1", RuleID: rule,
			SafeToDelete: safe, Tags: tags, CreatedAt: now, EstimatedMonthlyUSD: 1,
			PriceSource: "bundled", SafeToDeleteHint: "hint",
		}
	}
	return []findings.Finding{
		row("disk-1", "azure", "azure.disk.unattached", true),
		row("pip-1", "azure", "azure.publicip.unassociated", true),
		row("nic-1", "azure", "azure.nic.unattached", true),
		row("vm-1", "azure", "azure.vm.stopped_allocated", false),
		row("plan-1", "azure", "azure.appserviceplan.empty", true),
		row("snap-az", "azure", "azure.snapshot.old", false),
		row("nat-az", "azure", "azure.natgateway.no_subnets", true),
		row("law-1", "azure", "azure.loganalytics.empty_ingestion", false),
		row("vol-1", "aws", "aws.ebs.unattached", true),
		row("eipalloc-1", "aws", "aws.ipv4.public", true),
		row("nat-aws", "aws", "aws.natgateway.idle", false),
		row("vpce-1", "aws", "aws.vpcendpoint.interface", false),
		row("i-1", "aws", "aws.ec2.stopped_with_ebs", false),
		row("snap-aws", "aws", "aws.snapshot.old", false),
		row("arn:elb", "aws", "aws.elb.no_healthy_targets", false),
		row("/unknown", "azure", "azure.unknown", true),
	}
}
