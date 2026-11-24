package main

import (
	"context"
	"time"

	"github.com/cdobratz/azguard/internal/account"
	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/guard"
	"github.com/cdobratz/azguard/internal/mcpserver"
	"github.com/cdobratz/azguard/internal/pricing"
	"github.com/cdobratz/azguard/internal/watch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol server for prevention checks",
		Long: `Serve prevention tools over stdio for an MCP client.

azguard mcp serve does not look up current spend. Prices are the bundled
list-price table (730 hours per month) and are approximate, not an invoice.`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Serve prevention tools on stdin and stdout",
		Long: `Speak the Model Context Protocol on stdin and stdout.

Tools:
  list_idle_resources            Idle and orphaned resources (azguard resources rules)
  list_new_billable_resources    Activity Log and CloudTrail creates since an RFC3339 time
  check_free_or_credit_status    Bundled free-tier limits and credit stored by azguard init
  estimate_monthly_cost          Approximate monthly price for one resource spec
  preflight                      terraform show -json plan, or a command such as az vm create

  azguard mcp serve

The process reads and writes newline-delimited JSON on stdio. Do not write logs
to stdout.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps := mcpserver.Deps{
				ListIdle: mcpListIdle,
				ListNew:  mcpListNew,
				Account:  mcpAccount,
			}
			return mcpserver.New(version, deps).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	})
	return cmd
}

func mcpListIdle(ctx context.Context, cloud string) (findings.Report, error) {
	opts := findings.Options{SnapshotAge: 30 * 24 * time.Hour}
	book := pricing.Bundled()
	var found []findings.Finding
	var errs []string
	if cloud == "all" || cloud == "azure" {
		azFound, azErrs := scanAzure(ctx, book, opts)
		found = append(found, azFound...)
		errs = append(errs, azErrs...)
	}
	if cloud == "all" || cloud == "aws" {
		awsFound, _, awsErrs := scanAWS(ctx, "", false, book, opts)
		found = append(found, awsFound...)
		errs = append(errs, awsErrs...)
	}
	found = findings.Reprice(found, book)
	return findings.NewReport(found, errs, time.Now().UTC()), nil
}

func mcpListNew(ctx context.Context, since time.Time, principal string) ([]watch.Alert, []string, error) {
	evs, notes, err := guardLiveEvents(ctx, since, time.Now())
	if err != nil && len(evs) == 0 {
		return nil, notes, err
	}
	return guard.CreatedDuring(evs, since, principal, nil), notes, nil
}

func mcpAccount() mcpserver.Account {
	az := account.AzureFromConfig(cfg.Account.Azure, cfg.Account.AzureCreditRemaining, cfg.Account.AzureCreditExpires)
	aws := account.AWSFromConfig(cfg.Account.AWS, cfg.Account.AWSSpendLimit, cfg.Account.AWSCreditRemaining, cfg.Account.AWSCreditExpires)
	return mcpserver.Account{
		AzureKind:          string(az.Kind),
		AzureCredit:        az.CreditRemaining,
		AzureCreditExpires: az.CreditExpires,
		AWSKind:            string(aws.Kind),
		AWSCredit:          aws.CreditRemaining,
		AWSCreditExpires:   aws.CreditExpires,
	}
}
