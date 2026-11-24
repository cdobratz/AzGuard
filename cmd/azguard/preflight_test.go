package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPreflightCommandFixtures(t *testing.T) {
	plan := mustRepoFile(t, "internal/iac/testdata/terraform.json")
	var buf bytes.Buffer
	cmd := preflightCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--plan", plan, "--command", "terraform apply tfplan"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "this will create a NAT Gateway at about $32.85/month") || !strings.Contains(out, "70.63") {
		t.Fatalf("plan output = %s", out)
	}

	buf.Reset()
	vm := strings.TrimSpace(string(mustReadRepo(t, "internal/preflight/testdata/az_vm_create.txt")))
	awsCmd := strings.TrimSpace(string(mustReadRepo(t, "internal/preflight/testdata/aws_run_instances.txt")))
	for _, command := range []string{vm, awsCmd} {
		buf.Reset()
		cmd = preflightCmd()
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{"--command", command})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "7.59") {
			t.Fatalf("%s: %s", command, buf.String())
		}
	}
}

func mustRepoFile(t *testing.T, rel string) string {
	t.Helper()
	return mustFindRepo(t, rel)
}

func mustReadRepo(t *testing.T, rel string) []byte {
	t.Helper()
	body, err := os.ReadFile(mustFindRepo(t, rel))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustFindRepo(t *testing.T, rel string) string {
	t.Helper()
	candidates := []string{rel, "../../" + rel}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Fatalf("fixture %s not found", rel)
	return ""
}
