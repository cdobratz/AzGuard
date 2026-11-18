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

func TestDryRunDeletesNothing(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	del := &fakeDeleter{}
	res, err := Run(context.Background(), []findings.Finding{
		volume("vol-old", "us-east-1", now.Add(-40*24*time.Hour), map[string]string{}, 8),
	}, Options{Apply: false, Yes: true, Now: now}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 {
		t.Fatalf("dry run deleted %v", del.ids)
	}
	if len(res.Deleted) != 0 || len(res.WouldDelete) != 1 {
		t.Fatalf("result deleted=%d would=%d", len(res.Deleted), len(res.WouldDelete))
	}
	item := res.WouldDelete[0]
	if item.ResourceID != "vol-old" || item.Type != "aws.ebs.unattached" || item.Region != "us-east-1" {
		t.Fatalf("item = %+v", item)
	}
	if item.Age != "40d" {
		t.Fatalf("age = %s", item.Age)
	}
	var buf bytes.Buffer
	if err := Write(&buf, res, "table"); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{"vol-old", "aws.ebs.unattached", "us-east-1", "40d", "$8.00", "Dry run", "deleted 0", "failed 0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestApplyWithoutConfirmationDeletesNothing(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	found := []findings.Finding{volume("vol-old", "us-east-1", now.Add(-10*24*time.Hour), map[string]string{}, 4)}
	del := &fakeDeleter{}
	res, err := Run(context.Background(), found, Options{Apply: true, Now: now}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 || len(res.Deleted) != 0 {
		t.Fatalf("unconfirmed apply deleted ids=%v deleted=%d", del.ids, len(res.Deleted))
	}
	if res.Note == "" || len(res.WouldDelete) != 1 {
		t.Fatalf("note=%q would=%d", res.Note, len(res.WouldDelete))
	}

	del = &fakeDeleter{}
	res, err = Run(context.Background(), found, Options{
		Apply: true,
		Now:   now,
		Confirm: func(n int) (bool, error) {
			if n != 1 {
				t.Fatalf("confirm count = %d", n)
			}
			return false, nil
		},
	}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 || len(res.Deleted) != 0 {
		t.Fatalf("declined confirm deleted ids=%v", del.ids)
	}
}

func TestProtectionTagIsRespected(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	kept := volume("vol-keep", "us-east-1", now.Add(-90*24*time.Hour), map[string]string{"AzGuard:Keep": "TRUE"}, 12)
	gone := volume("vol-old", "us-east-1", now.Add(-90*24*time.Hour), map[string]string{"env": "dev"}, 3)
	unknown := volume("vol-notags", "us-east-1", now.Add(-90*24*time.Hour), nil, 9)
	del := &fakeDeleter{}
	res, err := Run(context.Background(), []findings.Finding{kept, gone, unknown}, Options{
		Apply:         true,
		Yes:           true,
		IncludeUnsafe: true,
		Now:           now,
	}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 1 || del.ids[0] != "vol-old" {
		t.Fatalf("deleted %v", del.ids)
	}
	reasons := map[string]string{}
	for _, item := range res.Skipped {
		reasons[item.ResourceID] = item.Reason
	}
	if !strings.Contains(reasons["vol-keep"], "azguard:keep=true") {
		t.Fatalf("keep reason = %q", reasons["vol-keep"])
	}
	if !strings.Contains(reasons["vol-notags"], "tags were not loaded") {
		t.Fatalf("missing tags reason = %q", reasons["vol-notags"])
	}
}

func TestFilters(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	oldEast := volume("vol-east", "us-east-1", now.Add(-40*24*time.Hour), map[string]string{"env": "dev"}, 8)
	youngEast := volume("vol-young", "us-east-1", now.Add(-2*time.Hour), map[string]string{"env": "dev"}, 8)
	oldWest := volume("vol-west", "us-west-2", now.Add(-40*24*time.Hour), map[string]string{"env": "dev"}, 8)
	noAge := volume("vol-unknown", "us-east-1", time.Time{}, map[string]string{"env": "dev"}, 8)
	eip := findings.Finding{
		ResourceID:          "eipalloc-1",
		Provider:            "aws",
		Region:              "us-east-1",
		RuleID:              "aws.eip.unassociated",
		SafeToDelete:        true,
		EstimatedMonthlyUSD: 3.65,
		PriceSource:         "bundled",
		Tags:                map[string]string{"env": "prod"},
		CreatedAt:           now.Add(-40 * 24 * time.Hour),
	}
	found := []findings.Finding{oldEast, youngEast, oldWest, noAge, eip}

	t.Run("region type tag and age", func(t *testing.T) {
		del := &fakeDeleter{}
		res, err := Run(context.Background(), found, Options{
			Apply:     true,
			Yes:       true,
			Now:       now,
			Regions:   []string{"US-EAST-1"},
			Types:     []string{"ebs"},
			Tags:      map[string]string{"Env": "Dev"},
			OlderThan: 30 * 24 * time.Hour,
		}, del)
		if err != nil {
			t.Fatal(err)
		}
		if len(del.ids) != 1 || del.ids[0] != "vol-east" {
			t.Fatalf("deleted %v skipped=%d", del.ids, len(res.Skipped))
		}
		skipped := map[string]string{}
		for _, item := range res.Skipped {
			skipped[item.ResourceID] = item.Reason
		}
		if !strings.Contains(skipped["vol-west"], "region") {
			t.Fatalf("west: %q", skipped["vol-west"])
		}
		if !strings.Contains(skipped["eipalloc-1"], "tag") && !strings.Contains(skipped["eipalloc-1"], "type") {
			t.Fatalf("eip: %q", skipped["eipalloc-1"])
		}
		if !strings.Contains(skipped["vol-young"], "not older than") {
			t.Fatalf("young: %q", skipped["vol-young"])
		}
		if !strings.Contains(skipped["vol-unknown"], "age is unknown") {
			t.Fatalf("unknown: %q", skipped["vol-unknown"])
		}
	})

	t.Run("type alias selects the elastic ip once", func(t *testing.T) {
		dup := eip
		dup.RuleID = "aws.ipv4.public"
		del := &fakeDeleter{}
		_, err := Run(context.Background(), []findings.Finding{eip, dup}, Options{
			Apply: true,
			Yes:   true,
			Now:   now,
			Types: []string{"eip"},
		}, del)
		if err != nil {
			t.Fatal(err)
		}
		if len(del.ids) != 1 || del.ids[0] != "eipalloc-1" {
			t.Fatalf("deleted %v", del.ids)
		}
		if del.actions[0] != ActionAWSReleaseAddress {
			t.Fatalf("action %s", del.actions[0])
		}
	})
}

func TestPartialFailure(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	del := &fakeDeleter{fail: map[string]error{
		"vol-b": errors.New("access denied"),
	}}
	res, err := Run(context.Background(), []findings.Finding{
		volume("vol-a", "us-east-1", now.Add(-10*24*time.Hour), map[string]string{}, 2),
		volume("vol-b", "us-east-1", now.Add(-10*24*time.Hour), map[string]string{}, 5),
		volume("vol-keep", "us-east-1", now.Add(-10*24*time.Hour), map[string]string{"azguard:keep": "true"}, 9),
	}, Options{Apply: true, Yes: true, Now: now}, del)
	if err == nil {
		t.Fatal("expected a non-zero failure")
	}
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.ExitCode() != 1 || runErr.Failed != 1 {
		t.Fatalf("err = %v", err)
	}
	if len(res.Deleted) != 1 || res.Deleted[0].ResourceID != "vol-a" {
		t.Fatalf("deleted %+v", res.Deleted)
	}
	if len(res.Failed) != 1 || res.Failed[0].ResourceID != "vol-b" || !strings.Contains(res.Failed[0].Reason, "access denied") {
		t.Fatalf("failed %+v", res.Failed)
	}
	for _, id := range del.ids {
		if id == "vol-keep" {
			t.Fatal("protected resource was sent to the deleter")
		}
	}
	var buf bytes.Buffer
	if err := Write(&buf, res, "table"); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "deleted 1") || !strings.Contains(text, "failed 1") || !strings.Contains(text, "skipped 1") {
		t.Fatalf("summary missing counts:\n%s", text)
	}
}

func TestUnsafeSkippedUnlessRequested(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snap := findings.Finding{
		ResourceID:          "snap-1",
		Provider:            "aws",
		Region:              "us-east-1",
		RuleID:              "aws.snapshot.old",
		SafeToDelete:        false,
		SafeToDeleteHint:    "Deleting a snapshot is permanent.",
		EstimatedMonthlyUSD: 1,
		PriceSource:         "bundled",
		Tags:                map[string]string{},
		CreatedAt:           now.Add(-60 * 24 * time.Hour),
	}
	del := &fakeDeleter{}
	res, err := Run(context.Background(), []findings.Finding{snap}, Options{Apply: true, Yes: true, Now: now}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("ids=%v skipped=%d", del.ids, len(res.Skipped))
	}
	res, err = Run(context.Background(), []findings.Finding{snap}, Options{Apply: true, Yes: true, IncludeUnsafe: true, Now: now}, del)
	if err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 1 || del.actions[0] != ActionAWSDeleteSnapshot {
		t.Fatalf("ids=%v actions=%v", del.ids, del.actions)
	}
}

func TestDiscoveryErrors(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	found := []findings.Finding{volume("vol-old", "us-east-1", now.Add(-10*24*time.Hour), map[string]string{}, 1)}
	res, err := Run(context.Background(), found, Options{
		Now:             now,
		DiscoveryErrors: []string{"AWS credentials are not configured; skipped AWS rules"},
	}, &fakeDeleter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.WouldDelete) != 1 {
		t.Fatalf("would=%d", len(res.WouldDelete))
	}
	_, err = Run(context.Background(), nil, Options{
		Now:             now,
		DiscoveryErrors: []string{"aws volumes (us-east-1): status 403"},
	}, &fakeDeleter{})
	if err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseOlderThanAndTags(t *testing.T) {
	d, err := ParseOlderThan("30d")
	if err != nil || d != 30*24*time.Hour {
		t.Fatalf("30d = %s %v", d, err)
	}
	if _, err := ParseOlderThan("nope"); err == nil {
		t.Fatal("expected error")
	}
	key, val, err := ParseTagFlag(" env = dev ")
	if err != nil || key != "env" || val != "dev" {
		t.Fatalf("tag %q %q %v", key, val, err)
	}
	if !Protected(map[string]string{"azguard:keep": "yes"}) {
		t.Fatal("yes should be protected")
	}
	if Protected(map[string]string{"azguard:keep": "false"}) {
		t.Fatal("false is not protected")
	}
}

type fakeDeleter struct {
	ids     []string
	actions []string
	fail    map[string]error
}

func (f *fakeDeleter) Delete(_ context.Context, item Item) error {
	f.ids = append(f.ids, item.ResourceID)
	f.actions = append(f.actions, item.Action)
	if err := f.fail[item.ResourceID]; err != nil {
		return err
	}
	return nil
}

func volume(id, region string, created time.Time, tags map[string]string, cost float64) findings.Finding {
	return findings.Finding{
		ResourceID:          id,
		Name:                id,
		Provider:            "aws",
		Region:              region,
		RuleID:              "aws.ebs.unattached",
		Title:               "Unattached EBS volume",
		SafeToDelete:        true,
		SafeToDeleteHint:    "Nothing is attached.",
		EstimatedMonthlyUSD: cost,
		PriceSource:         "bundled",
		Currency:            "USD",
		Tags:                tags,
		CreatedAt:           created,
	}
}
