package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/cleanup"
	"github.com/cdobratz/azguard/internal/findings"
)

func TestCleanupCommandHelp(t *testing.T) {
	cmd := cleanupCmd()
	if cmd.Flags().Lookup("apply").DefValue != "false" {
		t.Fatalf("apply default = %s", cmd.Flags().Lookup("apply").DefValue)
	}
	if cmd.Flags().Lookup("yes").DefValue != "false" {
		t.Fatalf("yes default = %s", cmd.Flags().Lookup("yes").DefValue)
	}
	if cmd.Flags().Lookup("older-than") == nil || cmd.Flags().Lookup("include-unsafe") == nil {
		t.Fatal("missing filter flags")
	}
	for _, want := range []string{"--apply", "--yes", "--older-than", "azguard:keep=true", "safe_to_delete"} {
		if !strings.Contains(cmd.Long, want) && want != "--apply" && want != "--yes" && want != "--older-than" {
			t.Fatalf("help missing %q", want)
		}
	}
	if !strings.Contains(cmd.Long, "azguard:keep=true") || !strings.Contains(cmd.Long, "--apply") || !strings.Contains(cmd.Long, "--older-than") {
		t.Fatal(cmd.Long)
	}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	help := buf.String()
	for _, flag := range []string{"--apply", "--yes", "--older-than", "--type", "--tag", "--region", "--include-unsafe"} {
		if !strings.Contains(help, flag) {
			t.Fatalf("help missing %s\n%s", flag, help)
		}
	}
}

func TestConfirmCleanup(t *testing.T) {
	yes, err := confirmCleanup(strings.NewReader("yes\n"), ioDiscard{}, 2)
	if err != nil || !yes {
		t.Fatalf("yes = %v %v", yes, err)
	}
	no, err := confirmCleanup(strings.NewReader("no\n"), ioDiscard{}, 1)
	if err != nil || no {
		t.Fatalf("no = %v %v", no, err)
	}
	empty, err := confirmCleanup(strings.NewReader(""), ioDiscard{}, 1)
	if err != nil || empty {
		t.Fatalf("eof = %v %v", empty, err)
	}
}

func TestRunCleanupDryRunAndUnconfirmed(t *testing.T) {
	prev := outputFormat
	outputFormat = "table"
	t.Cleanup(func() { outputFormat = prev })

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	found := []findings.Finding{{
		ResourceID:          "vol-1",
		Provider:            "aws",
		Region:              "us-east-1",
		RuleID:              "aws.ebs.unattached",
		SafeToDelete:        true,
		EstimatedMonthlyUSD: 4,
		PriceSource:         "bundled",
		Tags:                map[string]string{},
		CreatedAt:           now.Add(-48 * time.Hour),
	}}
	cmd := cleanupCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	del := &recordingDeleter{}
	if err := runCleanup(cmd, found, cleanup.Options{Apply: false, Yes: true, Now: now}, del); err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 {
		t.Fatalf("dry run deleted %v", del.ids)
	}
	if !strings.Contains(out.String(), "vol-1") || !strings.Contains(out.String(), "Dry run") {
		t.Fatalf("output:\n%s", out.String())
	}

	out.Reset()
	cmd.SetIn(strings.NewReader("no\n"))
	if err := runCleanup(cmd, found, cleanup.Options{
		Apply: true,
		Now:   now,
		Confirm: func(n int) (bool, error) {
			return confirmCleanup(cmd.InOrStdin(), &out, n)
		},
	}, del); err != nil {
		t.Fatal(err)
	}
	if len(del.ids) != 0 {
		t.Fatalf("unconfirmed command deleted %v", del.ids)
	}
	if !strings.Contains(out.String(), "Nothing was deleted") {
		t.Fatalf("output:\n%s", out.String())
	}
}

type recordingDeleter struct {
	ids []string
}

func (r *recordingDeleter) Delete(_ context.Context, item cleanup.Item) error {
	r.ids = append(r.ids, item.ResourceID)
	return nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
