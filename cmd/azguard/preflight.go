package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cdobratz/azguard/internal/preflight"
	"github.com/spf13/cobra"
)

func preflightCmd() *cobra.Command {
	var planPath, command string
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Estimate what a plan or a cloud command would create",
		Long: `Estimate the monthly list price of a terraform show -json plan or a cloud command.

  azguard preflight --command "az vm create -g rg -n vm --size Standard_B1s"
  azguard preflight --command "aws ec2 run-instances --instance-type t3.micro"
  terraform show -json tfplan > plan.json
  azguard preflight --plan plan.json
  azguard preflight --plan plan.json --command "terraform apply tfplan"

The figure is the bundled list price (730 hours per month), not an invoice.
For a full infrastructure estimate, use Infracost (https://www.infracost.io/).
Output is JSON on stdout.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := runPreflight(cmd, planPath, command)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		},
	}
	cmd.Flags().StringVar(&planPath, "plan", "", "Path to terraform show -json output, or another plan the preflight parser accepts")
	cmd.Flags().StringVar(&command, "command", "", "Shell command to check, such as az vm create or aws ec2 run-instances")
	return cmd
}

func runPreflight(cmd *cobra.Command, planPath, command string) (preflight.Result, error) {
	command = strings.TrimSpace(command)
	switch {
	case planPath != "" && command != "":
		body, err := os.ReadFile(planPath)
		if err != nil {
			return preflight.Result{}, err
		}
		return preflight.InspectWithPlan(command, body)
	case planPath != "":
		body, err := os.ReadFile(planPath)
		if err != nil {
			return preflight.Result{}, err
		}
		return preflight.Inspect(string(body))
	case command != "":
		return preflight.Inspect(command)
	default:
		body, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return preflight.Result{}, err
		}
		if strings.TrimSpace(string(body)) == "" {
			return preflight.Result{}, fmt.Errorf("preflight needs --plan, --command, or a plan on stdin")
		}
		return preflight.Inspect(string(body))
	}
}
