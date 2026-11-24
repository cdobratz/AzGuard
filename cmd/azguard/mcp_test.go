package main

import (
	"strings"
	"testing"
)

func TestMCPServeHelp(t *testing.T) {
	cmd := mcpCmd()
	serve, _, err := cmd.Find([]string{"serve"})
	if err != nil {
		t.Fatal(err)
	}
	help := serve.Long + serve.Short
	for _, name := range []string{
		"list_idle_resources",
		"list_new_billable_resources",
		"check_free_or_credit_status",
		"estimate_monthly_cost",
		"preflight",
		"stdin",
	} {
		if !strings.Contains(help, name) {
			t.Fatalf("help missing %s:\n%s", name, help)
		}
	}
}
