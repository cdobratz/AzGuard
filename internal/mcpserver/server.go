// Package mcpserver is the stdio Model Context Protocol server for azguard.
//
// The tools are prevention checks. They do not look up current spend. Prices
// are the bundled list-price table (730 hours per month), which is approximate
// and is not an invoice.
package mcpserver

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/watch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Account is the free-tier and credit snapshot stored by azguard init.
// Kinds are the account.Kind values, or empty when init has not been run.
type Account struct {
	AzureKind          string
	AzureCredit        *float64
	AzureCreditExpires string
	AWSKind            string
	AWSCredit          *float64
	AWSCreditExpires   string
}

// Deps are the cloud calls the server is allowed to make.
// Nil functions mean that check is not configured. Tests pass fakes.
// Preflight and estimate_monthly_cost do not use Deps.
type Deps struct {
	ListIdle func(ctx context.Context, cloud string) (findings.Report, error)
	ListNew  func(ctx context.Context, since time.Time, principal string) ([]watch.Alert, []string, error)
	Account  func() Account
}

// New builds a server that speaks newline-delimited JSON over the transport
// passed to Run. azguard mcp serve uses the stdio transport.
func New(version string, deps Deps) *mcp.Server {
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "azguard",
		Title:       "AzGuard",
		Version:     version,
		Description: "Prevention checks for idle resources, new billable creates, free-tier and credit status, and approximate monthly cost.",
	}, &mcp.ServerOptions{
		Instructions: "AzGuard tools help prevent new cloud spend. Estimates use a small bundled price table and are approximate. These tools do not look up an invoice or current spend.",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Capabilities: &mcp.ServerCapabilities{},
	})
	register(server, deps)
	return server
}

func register(server *mcp.Server, deps Deps) {
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_idle_resources",
		Description: "List idle and orphaned Azure and AWS resources, with an approximate monthly list price. Same rules as azguard resources. This does not delete anything and does not look up current spend.",
		Annotations: ro,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args idleArgs) (*mcp.CallToolResult, any, error) {
		return toolText(deps.idle(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_new_billable_resources",
		Description: "List billable resources created at or after since, from the Azure Activity Log and CloudTrail, filtered by time and an optional principal. Same events as azguard watch. This does not look up current spend.",
		Annotations: ro,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args newArgs) (*mcp.CallToolResult, any, error) {
		return toolText(deps.listNew(ctx, args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_free_or_credit_status",
		Description: "Show the bundled Azure and AWS free-tier limits and any credit remaining stored by azguard init. This does not query live usage or an invoice.",
		Annotations: ro,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args statusArgs) (*mcp.CallToolResult, any, error) {
		return toolText(deps.creditStatus(args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "estimate_monthly_cost",
		Description: "Estimate the monthly list price of one resource from the bundled price table. The figure is approximate (730 hours per month) and is not an invoice.",
		Annotations: ro,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args estimateArgs) (*mcp.CallToolResult, any, error) {
		return toolText(estimateMonthly(args))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "preflight",
		Description: "Say what a terraform show -json plan or a cloud command would create, with an approximate monthly price. Example: this will create a NAT Gateway at about $32.85/month. Accepts az vm create and aws ec2 run-instances.",
		Annotations: ro,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args preflightArgs) (*mcp.CallToolResult, any, error) {
		return toolText(runPreflight(args))
	})
}

func toolText(text string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

type idleArgs struct {
	Cloud string `json:"cloud,omitempty" jsonschema:"all, azure, or aws. Defaults to all."`
}

type newArgs struct {
	Since     string `json:"since" jsonschema:"RFC3339 timestamp. Only creates at or after this time are returned."`
	Principal string `json:"principal,omitempty" jsonschema:"Optional caller, UPN, or role ARN. Empty matches every principal."`
}

type statusArgs struct {
	Cloud string `json:"cloud,omitempty" jsonschema:"all, azure, or aws. Defaults to all."`
}

type estimateArgs struct {
	Provider string  `json:"provider" jsonschema:"azure or aws."`
	Kind     string  `json:"kind" jsonschema:"Resource kind such as vm, nat_gateway, public_ip, disk, ebs, instance, or load_balancer."`
	SKU      string  `json:"sku,omitempty" jsonschema:"SKU or size, such as Standard_B1s, t3.micro, gp3, or Standard_LRS."`
	Quantity float64 `json:"quantity,omitempty" jsonschema:"Count, or size in GB for disks, volumes, and snapshots. Zero selects the default."`
	Region   string  `json:"region,omitempty" jsonschema:"Optional region. The bundled table is not region specific."`
}

type preflightArgs struct {
	Input   string `json:"input,omitempty" jsonschema:"A terraform show -json plan, or a shell command such as az vm create or aws ec2 run-instances."`
	Plan    string `json:"plan,omitempty" jsonschema:"terraform show -json output. Use with command when the command is terraform apply, pulumi up, or cdk deploy."`
	Command string `json:"command,omitempty" jsonschema:"Shell command to check. Ignored when input is set."`
}
