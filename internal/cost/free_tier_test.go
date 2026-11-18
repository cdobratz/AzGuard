package cost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cdobratz/azguard/configs"
)

func TestEmbeddedLimitsHaveVersionAndKnownCaps(t *testing.T) {
	azure, err := parseLimitTable("azure", configs.AzureFreeTierYAML)
	if err != nil {
		t.Fatal(err)
	}
	if azure.Version != 1 {
		t.Fatalf("azure version %d", azure.Version)
	}
	vm := azure.Services["virtual_machines"]
	if vm.Limit != 750 || vm.Unit != "hours" {
		t.Fatalf("vm limit = %+v", vm)
	}
	sqlDB := azure.Services["sql_database"]
	if sqlDB.Limit != 32 || sqlDB.Unit != "MB" {
		t.Fatalf("sql limit = %+v", sqlDB)
	}
	if azure.Budgets["tiny"].Amount != 1 || azure.Budgets["moderate"].Amount != 20 {
		t.Fatalf("budgets = %+v", azure.Budgets)
	}

	aws, err := parseLimitTable("aws", configs.AWSFreeTierYAML)
	if err != nil {
		t.Fatal(err)
	}
	if aws.Version != 1 {
		t.Fatalf("aws version %d", aws.Version)
	}
	ec2 := aws.Services["ec2"]
	if ec2.Limit != 750 || ec2.WarningThreshold != 0.8 {
		t.Fatalf("ec2 = %+v", ec2)
	}
	if aws.Services["cognito"].Limit != 50000 {
		t.Fatalf("cognito = %+v", aws.Services["cognito"])
	}
}

func TestLoadUsesEmbeddedTableUnlessHomeOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	loaded, err := LoadFreeTierConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Services["functions"].Limit != 1000000 {
		t.Fatalf("functions = %+v", loaded.Services["functions"])
	}
	aws, err := LoadAWSFreeTierConfig()
	if err != nil {
		t.Fatal(err)
	}
	if aws.Services["lambda"].Limit != 1000000 {
		t.Fatalf("lambda = %+v", aws.Services["lambda"])
	}

	dir := filepath.Join(home, ".azguard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	override := []byte("version: 2\nservices:\n  virtual_machines:\n    limit: 12\n    unit: hours\nbudgets:\n  tiny:\n    amount: 3\n    description: override\n")
	if err := os.WriteFile(filepath.Join(dir, "free_tier_limits.yaml"), override, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadFreeTierConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 2 || loaded.Services["virtual_machines"].Limit != 12 || loaded.Budgets["tiny"].Amount != 3 {
		t.Fatalf("override = %+v", loaded)
	}

	if err := os.WriteFile(filepath.Join(dir, "aws_free_tier_limits.yaml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAWSFreeTierConfig(); err == nil {
		t.Fatal("expected missing version to fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "aws_free_tier_limits.yaml"), []byte("version: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAWSFreeTierConfig(); err == nil {
		t.Fatal("expected invalid yaml to fail")
	}
}

func TestCheckServiceUsage(t *testing.T) {
	limit := &ServiceLimit{Limit: 100, Unit: "hours", WarningThreshold: 0.8}
	cases := []struct {
		name   string
		usage  float64
		limit  *ServiceLimit
		status ResourceStatus
		pct    float64
	}{
		{name: "nil", usage: 5, status: StatusUnknown},
		{name: "zero limit", usage: 5, limit: &ServiceLimit{}, status: StatusUnknown},
		{name: "negative usage", usage: -4, limit: limit, status: StatusFree, pct: 0},
		{name: "free", usage: 79, limit: limit, status: StatusFree, pct: 79},
		{name: "warning", usage: 80, limit: limit, status: StatusWarning, pct: 80},
		{name: "over", usage: 100, limit: limit, status: StatusOverage, pct: 100},
		{name: "way over", usage: 150, limit: limit, status: StatusOverage, pct: 150},
		{name: "no warning threshold", usage: 90, limit: &ServiceLimit{Limit: 100, Unit: "GB"}, status: StatusFree, pct: 90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckServiceUsage(tc.usage, tc.limit)
			if got.Status != tc.status {
				t.Fatalf("status %s, want %s (%+v)", got.Status, tc.status, got)
			}
			if tc.limit != nil && tc.limit.Limit > 0 && got.PercentUsed != tc.pct {
				t.Fatalf("percent %v, want %v", got.PercentUsed, tc.pct)
			}
			if tc.limit != nil && tc.limit.Limit > 0 && got.Unit != tc.limit.Unit {
				t.Fatalf("unit %s", got.Unit)
			}
		})
	}
}
