package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cdobratz/azguard/internal/config"
)

func TestGuardHookSubcommandAllowsReadOnly(t *testing.T) {
	cmd := guardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"az group list"}}`))
	cmd.SetArgs([]string{"hook", "--budget", "1", "--session", filepath.Join(t.TempDir(), "session.json")})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"permissionDecision": "allow"`) {
		t.Fatalf("hook output = %s", buf.String())
	}
}

func TestGuardHookUsesConfigBudget(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })
	cfg = &config.Config{Guard: config.GuardConfig{BudgetUSD: 1}}

	cmd := guardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"aws ec2 run-instances"}}`))
	cmd.SetArgs([]string{"hook", "--session", filepath.Join(t.TempDir(), "session.json")})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"permissionDecision": "deny"`) || !strings.Contains(out, "session budget") {
		t.Fatalf("hook output = %s", out)
	}
}

func TestGuardFlagBudgetOverridesConfig(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })
	cfg = &config.Config{Guard: config.GuardConfig{BudgetUSD: 1}}

	cmd := guardCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"aws ec2 run-instances"}}`))
	cmd.SetArgs([]string{"hook", "--budget", "0", "--session", filepath.Join(t.TempDir(), "session.json")})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"permissionDecision": "allow"`) || strings.Contains(out, `"permissionDecision": "deny"`) {
		t.Fatalf("hook output = %s", out)
	}
}
