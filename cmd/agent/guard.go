package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	awscloud "github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/events"
	"github.com/azguard/azguard/internal/guard"
	"github.com/spf13/cobra"
)

func guardCmd() *cobra.Command {
	var budget float64
	var sessionPath string
	var planPath string
	cmd := &cobra.Command{
		Use:   "guard -- <command>",
		Short: "Block billable cloud commands over a session budget",
		Long: `Recognize billable cloud commands and block them when a session budget is exceeded.

With no command, guard reads a Claude Code PreToolUse hook payload on stdin and writes
hookSpecificOutput JSON on stdout. The process exits 0 so Claude Code can read a deny decision.

  azguard guard --budget 25 -- aws ec2 run-instances --instance-type t3.micro
  azguard guard report --fixture activity.json

A budget of 0 (the default) prints the estimate and allows the command.
Read-only commands (describe, list, show, terraform plan, pulumi preview, cdk diff) pass through.
terraform apply, pulumi up, and cdk deploy are billable. Pass --plan with a terraform show -json
plan, Pulumi preview JSON, or a CDK synth / CloudFormation template to price them from the bundled
table. Without --plan, a budget does not block those commands.

The estimate is reserved when the command is allowed. Exit code 2 means the wrapper did not run it.

Claude Code settings snippet (.claude/settings.json):

  {
    "hooks": {
      "PreToolUse": [
        {
          "matcher": "Bash",
          "hooks": [{ "type": "command", "command": "azguard guard --budget 25" }]
        }
      ]
    }
  }`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var plan []byte
			if planPath != "" {
				body, err := os.ReadFile(planPath)
				if err != nil {
					return err
				}
				plan = body
			}
			opts := guard.Options{BudgetUSD: budget, Path: guardSessionPath(sessionPath), Plan: plan}
			if len(args) > 0 {
				return guard.RunCommand(args, opts)
			}
			return guard.RunHook(os.Stdin, cmd.OutOrStdout(), opts)
		},
	}
	cmd.PersistentFlags().Float64Var(&budget, "budget", 0, "Session budget in USD per month. 0 warns and allows")
	cmd.PersistentFlags().StringVar(&sessionPath, "session", "", "Session file (default: ~/.azguard/guard-session.json)")
	cmd.PersistentFlags().StringVar(&planPath, "plan", "", "terraform show -json, Pulumi preview JSON, or CDK/CloudFormation template")
	cmd.AddCommand(guardReportCmd(&sessionPath))
	return cmd
}

func guardReportCmd(sessionPath *string) *cobra.Command {
	var (
		fixtures  []string
		sinceRaw  string
		principal string
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "List billable resources created during the guard session",
		Long: `List billable creates from the Azure Activity Log and CloudTrail LookupEvents
since the guard session started, or since --since.

--fixture reads a recorded Activity Log page or LookupEvents page instead of calling the cloud APIs.
Filter with --principal (display name, caller, UPN, or role ARN).`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := guardSessionPath(*sessionPath)
			since := time.Time{}
			if sinceRaw != "" {
				parsed, err := time.Parse(time.RFC3339, sinceRaw)
				if err != nil {
					return fmt.Errorf("--since must be RFC3339: %w", err)
				}
				since = parsed
			} else if body, err := os.ReadFile(path); err == nil {
				var session guard.Session
				if err := json.Unmarshal(body, &session); err == nil {
					since = session.StartedAt
				}
			}
			if since.IsZero() && len(fixtures) == 0 {
				return fmt.Errorf("pass --since or run azguard guard first so a session file exists")
			}
			var evs []events.Event
			if len(fixtures) > 0 {
				for _, fixture := range fixtures {
					body, err := os.ReadFile(fixture)
					if err != nil {
						return err
					}
					parsed, err := guard.EventsFromFixture(body)
					if err != nil {
						return err
					}
					evs = append(evs, parsed...)
				}
			} else {
				live, notes, err := guardLiveEvents(cmd.Context(), since, time.Now())
				for _, note := range notes {
					fmt.Fprintln(cmd.ErrOrStderr(), note)
				}
				if err != nil && len(live) == 0 {
					return err
				}
				evs = live
			}
			alerts := guard.CreatedDuring(evs, since, principal, nil)
			guard.WriteReport(cmd.OutOrStdout(), alerts)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&fixtures, "fixture", nil, "Recorded Activity Log or LookupEvents JSON (repeatable)")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "RFC3339 start time (default: session started_at)")
	cmd.Flags().StringVar(&principal, "principal", "", "Only events from this caller, UPN, or role ARN")
	return cmd
}

func guardSessionPath(path string) string {
	if path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".azguard", "guard-session.json")
}

func guardLiveEvents(ctx context.Context, since, until time.Time) ([]events.Event, []string, error) {
	var evs []events.Event
	var notes []string
	var first error
	if cfg.Azure.SubscriptionID != "" && azure.ValidateSubscriptionID(cfg.Azure.SubscriptionID) == nil {
		token, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
			"tenant_id":     cfg.Azure.TenantID,
			"client_id":     cfg.Azure.ClientID,
			"client_secret": cfg.Azure.ClientSecret,
		})
		if err != nil {
			notes = append(notes, "Azure auth: "+err.Error())
			first = err
		} else {
			parsed, err := azure.NewReader(cfg.Azure.SubscriptionID, token).ActivityEvents(ctx, since, until)
			if err != nil {
				notes = append(notes, "Azure Activity Log: "+err.Error())
				if first == nil {
					first = err
				}
			} else {
				evs = append(evs, parsed...)
			}
		}
	} else {
		notes = append(notes, "Azure subscription is not configured; skipped Activity Log")
	}
	costClient := awscloud.NewCostClient(cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SessionToken, cfg.AWS.Region)
	reader := awscloud.ReaderFromCost(costClient)
	if reader.Enabled() {
		parsed, err := reader.LookupEvents(ctx, reader.Region, since, until)
		if err != nil {
			notes = append(notes, "CloudTrail: "+err.Error())
			if first == nil {
				first = err
			}
		} else {
			evs = append(evs, parsed...)
		}
	} else {
		notes = append(notes, "AWS credentials are not configured; skipped CloudTrail")
	}
	return evs, notes, first
}
