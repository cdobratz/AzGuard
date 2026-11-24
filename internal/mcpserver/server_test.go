package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/pricing"
	"github.com/cdobratz/azguard/internal/watch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsUseFixtures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	deps := Deps{
		ListIdle: idleFromDiskFixture,
		ListNew:  newFromActivityFixture,
		Account: func() Account {
			credit := 50.0
			return Account{AzureKind: "azure-free", AzureCredit: &credit, AzureCreditExpires: "2026-12-31"}
		},
	}

	idle, err := deps.idle(context.Background(), idleArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(idle, "orphan") || !strings.Contains(idle, "5.89") {
		t.Fatalf("idle = %s", idle)
	}

	since := "2026-10-17T12:04:00Z"
	created, err := deps.listNew(context.Background(), newArgs{Since: since, Principal: "casey@contoso.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created, "vm1") || strings.Contains(created, "pip-fail") {
		t.Fatalf("new = %s", created)
	}

	status, err := deps.creditStatus(statusArgs{Cloud: "azure"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "virtual_machines") || !strings.Contains(status, "750") || !strings.Contains(status, `"credit_remaining": 50`) {
		t.Fatalf("status = %s", status)
	}
	if !strings.Contains(status, "does not query live usage") {
		t.Fatalf("status = %s", status)
	}

	est, err := estimateMonthly(estimateArgs{Provider: "azure", Kind: "nat_gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(est, "32.85") || !strings.Contains(est, "bundled") {
		t.Fatalf("estimate = %s", est)
	}

	body, err := os.ReadFile("../preflight/testdata/az_nat_gateway.txt")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := os.ReadFile("../iac/testdata/terraform.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := runPreflight(preflightArgs{Command: strings.TrimSpace(string(body))})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "this will create a NAT Gateway at about $32.85/month") {
		t.Fatalf("preflight = %s", got)
	}
	tf, err := runPreflight(preflightArgs{Input: string(plan)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tf, "NAT Gateway") || !strings.Contains(tf, "70.63") {
		t.Fatalf("terraform preflight = %s", tf)
	}
	vm, err := os.ReadFile("../preflight/testdata/az_vm_create.txt")
	if err != nil {
		t.Fatal(err)
	}
	awsCmd, err := os.ReadFile("../preflight/testdata/aws_run_instances.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []string{string(vm), string(awsCmd)} {
		out, err := runPreflight(preflightArgs{Input: fixture})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "7.59") {
			t.Fatalf("%s: %s", fixture, out)
		}
	}
}

func TestStdioServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientWriter.Close()
		_ = serverWriter.Close()
	})

	server := New("test", Deps{
		ListIdle: func(context.Context, string) (findings.Report, error) {
			return findings.NewReport(nil, []string{"Azure subscription is not configured; skipped Azure rules"}, time.Unix(0, 0).UTC()), nil
		},
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, &mcp.IOTransport{Reader: serverReader, Writer: serverWriter})
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "azguard-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "list_new_billable_resources" && !strings.Contains(string(raw), "since") {
			t.Fatalf("schema = %s", raw)
		}
		if tool.Name == "preflight" && !strings.Contains(string(raw), "input") {
			t.Fatalf("schema = %s", raw)
		}
	}
	for _, name := range []string{
		"list_idle_resources",
		"list_new_billable_resources",
		"check_free_or_credit_status",
		"estimate_monthly_cost",
		"preflight",
	} {
		if !names[name] {
			t.Fatalf("missing tool %s in %+v", name, names)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "preflight",
		Arguments: map[string]any{"command": "aws ec2 create-nat-gateway"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := contentText(res)
	if res.IsError || !strings.Contains(text, "this will create a NAT Gateway at about $32.85/month") {
		t.Fatalf("preflight result = %s", text)
	}

	idle, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_idle_resources"})
	if err != nil {
		t.Fatal(err)
	}
	if idle.IsError || !strings.Contains(contentText(idle), "not configured") {
		t.Fatalf("idle = %s", contentText(idle))
	}
}

func idleFromDiskFixture(context.Context, string) (findings.Report, error) {
	body, err := os.ReadFile("../findings/testdata/azure/disks.json")
	if err != nil {
		return findings.Report{}, err
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return findings.Report{}, err
	}
	found := findings.EvalAzureDisks(page.Data, pricing.Bundled())
	return findings.NewReport(found, nil, time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)), nil
}

func newFromActivityFixture(_ context.Context, since time.Time, principal string) ([]watch.Alert, []string, error) {
	body, err := os.ReadFile("../watch/testdata/activity.json")
	if err != nil {
		return nil, nil, err
	}
	alerts, err := AlertsFromFixture(body, since, principal)
	return alerts, []string{"fixture"}, err
}

func contentText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}
