package main

import (
	"strings"
	"testing"

	"github.com/azguard/azguard/internal/findings"
)

func TestResourcesCommandFlags(t *testing.T) {
	cmd := resourcesCmd()
	if cmd.Flags().Lookup("fail-over").DefValue != "-1" {
		t.Fatalf("fail-over default = %s", cmd.Flags().Lookup("fail-over").DefValue)
	}
	if cmd.Flags().Lookup("snapshot-days").DefValue != "30" {
		t.Fatalf("snapshot-days default = %s", cmd.Flags().Lookup("snapshot-days").DefValue)
	}
	if !strings.Contains(cmd.Long, "Exit code 2") {
		t.Fatal("help should document exit code 2")
	}
	azure, aws := 0, 0
	for _, rule := range findings.Catalog() {
		switch rule.Provider {
		case "azure":
			azure++
		case "aws":
			aws++
		}
	}
	if azure < 8 || aws < 8 {
		t.Fatalf("catalog has %d azure and %d aws rules", azure, aws)
	}
}
