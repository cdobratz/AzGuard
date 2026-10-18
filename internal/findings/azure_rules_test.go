package findings

import (
	"testing"
	"time"

	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/pricing"
)

func TestAzureRulesFromFixtures(t *testing.T) {
	book := pricing.Bundled()
	opts := Options{Now: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	seen := map[string]bool{}

	t.Run("unattached disk", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/disks.json")
		got := EvalAzureDisks(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Compute/disks/orphan")
		assertFinding(t, f, "azure.disk.unattached", "eastus", 5.89, true, "agent-sp")
		absent(t, got, "osdisk")
		seen[f.RuleID] = true
	})

	t.Run("unassociated public ip", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/public_ips.json")
		got := EvalAzurePublicIPs(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Network/publicIPAddresses/pip-idle")
		assertFinding(t, f, "azure.publicip.unassociated", "eastus", 3.65, true, "11111111-2222-3333-4444-555555555555")
		absent(t, got, "pip-used")
		seen[f.RuleID] = true
	})

	t.Run("unattached nic", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/nics.json")
		got := EvalAzureNICs(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Network/networkInterfaces/nic-orphan")
		assertFinding(t, f, "azure.nic.unattached", "eastus", 0, true, "agent-sp")
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1", len(got))
		}
		seen[f.RuleID] = true
	})

	t.Run("stopped allocated vm", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/vms.json")
		got := EvalAzureStoppedVMs(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm-stopped")
		assertFinding(t, f, "azure.vm.stopped_allocated", "eastus", 7.59, false, "agent-sp")
		absent(t, got, "vm-deallocated")
		absent(t, got, "vm-running")
		seen[f.RuleID] = true
	})

	t.Run("empty app service plan", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/plans.json")
		got := EvalAzurePlans(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Web/serverfarms/plan-empty-b1")
		assertFinding(t, f, "azure.appserviceplan.empty", "eastus", 13.14, true, "agent-sp")
		free := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Web/serverfarms/plan-empty-free")
		assertFinding(t, free, "azure.appserviceplan.empty", "eastus", 0, true, "human@contoso.com")
		absent(t, got, "plan-used")
		seen[f.RuleID] = true
	})

	t.Run("old snapshot", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/snapshots.json")
		got := EvalAzureSnapshots(rows, book, opts)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Compute/snapshots/snap-old")
		assertFinding(t, f, "azure.snapshot.old", "eastus", 2.50, false, "agent-sp")
		absent(t, got, "snap-recent")
		seen[f.RuleID] = true
	})

	t.Run("nat gateway without subnets", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/nat_gateways.json")
		got := EvalAzureNATGateways(rows, book)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Network/natGateways/nat-idle")
		assertFinding(t, f, "azure.natgateway.no_subnets", "eastus", 32.85, true, "agent-sp")
		absent(t, got, "nat-used")
		seen[f.RuleID] = true
	})

	t.Run("empty log analytics workspace with ingestion", func(t *testing.T) {
		rows := graphRows(t, "testdata/azure/workspaces.json")
		solutions := graphRows(t, "testdata/azure/solutions.json")
		inUse, err := azure.ParseWorkspaceIngestion(mustRead(t, "testdata/azure/workspace_usage_bytes.json"))
		if err != nil {
			t.Fatal(err)
		}
		quiet, err := azure.ParseWorkspaceIngestion(mustRead(t, "testdata/azure/workspace_usage_zero.json"))
		if err != nil {
			t.Fatal(err)
		}
		ingestion := map[string]float64{
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws-empty": inUse,
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws-quiet": quiet,
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws-used":  inUse,
		}
		got := EvalAzureWorkspaces(rows, solutions, ingestion, book, opts)
		f := one(t, got, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws-empty")
		assertFinding(t, f, "azure.loganalytics.empty_ingestion", "eastus", 11.50, false, "agent-sp")
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1", len(got))
		}
		seen[f.RuleID] = true
	})

	for _, rule := range Catalog() {
		if rule.Provider == "azure" && !seen[rule.ID] {
			t.Errorf("catalog rule %s has no fixture finding", rule.ID)
		}
	}
}

func graphRows(t *testing.T, path string) []map[string]any {
	t.Helper()
	rows, _, err := azure.ParseGraphPage(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
