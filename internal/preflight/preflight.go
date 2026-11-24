// Package preflight estimates what a shell command or a Terraform plan would create.
//
// Prices come from the bundled list-price table (730 hours per month). They are
// approximate list prices, not an invoice. azguard guard and the GitHub Action
// call this package so command and plan parsing live in one place.
package preflight

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cdobratz/azguard/internal/iac"
	"github.com/cdobratz/azguard/internal/pricing"
)

// PriceNote is the sentence shown with every estimate.
const PriceNote = "Approximate list price from AzGuard's bundled table (730 hours per month). This is not an invoice."

// Item is one resource the input would create.
type Item struct {
	Address    string  `json:"address,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Label      string  `json:"label"`
	MonthlyUSD float64 `json:"monthly_usd"`
	Priced     bool    `json:"priced"`
}

// Result is the preflight estimate for one command, one plan, or both.
type Result struct {
	Format     string  `json:"format"`
	Billable   bool    `json:"billable"`
	Priced     bool    `json:"priced"`
	MonthlyUSD float64 `json:"monthly_usd"`
	Summary    string  `json:"summary,omitempty"`
	Message    string  `json:"message"`
	Note       string  `json:"note"`
	Items      []Item  `json:"items"`
}

// Inspect reads a terraform show -json plan or a shell command.
// A command such as `az vm create` or `aws ec2 run-instances` is priced from
// the bundled table. JSON that iac.Parse understands (Terraform, Pulumi, or
// CloudFormation) is priced the same way.
func Inspect(input string) (Result, error) {
	return InspectWithPlan(input, nil)
}

// InspectWithPlan prices command. When command is terraform apply, pulumi up,
// or cdk deploy, plan (a terraform show -json document or another plan iac.Parse
// accepts) supplies the monthly total. A plan is ignored for any other command,
// matching azguard guard.
func InspectWithPlan(command string, plan []byte) (Result, error) {
	command = strings.TrimSpace(command)
	plan = bytes.TrimSpace(plan)
	if len(plan) == 0 && looksLikePlan(command) {
		plan = []byte(command)
		command = ""
	}
	if command == "" && len(plan) == 0 {
		return Result{}, fmt.Errorf("preflight: empty input")
	}

	res := Result{Format: "command", Items: []Item{}}
	if command != "" {
		res = inspectCommand(command)
	}
	if len(plan) == 0 || (command != "" && !isIaC(command)) {
		return finish(res), nil
	}

	est, err := iac.Parse(plan, pricing.Bundled())
	if err != nil {
		return Result{}, err
	}
	notes := dropIaCNotes(res.Summary)
	onlyIaC := command == "" || len(notes) == 0
	res.Summary = strings.Join(notes, "; ")
	res.Items = append(res.Items, itemsFromEstimate(est)...)
	res.Format = est.Format
	if !est.Complete {
		res.Billable = true
		res.Priced = false
		res.Summary = joinNonEmpty(res.Summary, "plan has resources without a bundled price")
		return finish(res), nil
	}
	if len(est.Lines) == 0 && command == "" {
		res.Billable = false
		res.Priced = false
		res.MonthlyUSD = 0
		return finish(res), nil
	}
	res.Billable = true
	res.MonthlyUSD = pricing.RoundUSD(res.MonthlyUSD + est.MonthlyUSD)
	res.Summary = joinNonEmpty(res.Summary, est.Format+" plan")
	if onlyIaC {
		res.Priced = true
	}
	return finish(res), nil
}

func finish(res Result) Result {
	if res.Items == nil {
		res.Items = []Item{}
	}
	res.Note = PriceNote
	res.Message = renderMessage(res)
	return res
}

func itemsFromEstimate(est iac.Estimate) []Item {
	items := make([]Item, 0, len(est.Lines))
	for _, line := range est.Lines {
		label := labelForKind(line.Kind)
		if line.Kind == "" {
			label = line.Address
			if label == "" {
				label = "resource"
			}
		}
		items = append(items, Item{
			Address:    line.Address,
			Kind:       line.Kind,
			Label:      label,
			MonthlyUSD: line.MonthlyUSD,
			Priced:     line.Priced,
		})
	}
	return items
}

func labelForKind(kind string) string {
	switch kind {
	case "aws.nat", "azure.nat":
		return "NAT Gateway"
	case "aws.eip", "azure.pip":
		return "public IP"
	case "aws.instance":
		return "EC2 instance"
	case "azure.vm":
		return "virtual machine"
	case "aws.ebs":
		return "EBS volume"
	case "azure.disk":
		return "managed disk"
	case "aws.lb":
		return "load balancer"
	case "aws.endpoint":
		return "VPC endpoint"
	case "azure.plan":
		return "App Service plan"
	case "azure.snapshot":
		return "snapshot"
	default:
		if kind == "" {
			return "resource"
		}
		return kind
	}
}

func renderMessage(res Result) string {
	if !res.Billable {
		return "this command does not create a billable resource"
	}
	var parts []string
	unpriced := false
	for _, it := range res.Items {
		if it.Label == "" {
			continue
		}
		if !it.Priced {
			unpriced = true
			parts = append(parts, "a "+it.Label+" with no bundled price")
			continue
		}
		parts = append(parts, fmt.Sprintf("a %s at about $%.2f/month", it.Label, it.MonthlyUSD))
	}
	if len(parts) == 0 {
		label := res.Summary
		if label == "" {
			label = "this command"
		}
		return "this will run " + label + ", which can create billable resources, but AzGuard has no bundled monthly price for it"
	}
	msg := "this will create " + joinAnd(parts)
	pricedCount := 0
	for _, it := range res.Items {
		if it.Priced {
			pricedCount++
		}
	}
	if pricedCount > 1 && res.Priced {
		msg += fmt.Sprintf(". Estimated total is about $%.2f/month", res.MonthlyUSD)
	}
	if unpriced {
		msg += ". The total is incomplete, so a session budget cannot use it"
	}
	return msg
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
	}
}

func looksLikePlan(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	switch s[0] {
	case '{', '[':
		return true
	}
	return strings.HasPrefix(s, "AWSTemplateFormatVersion") || strings.Contains(s, "\nResources:")
}

func dropIaCNotes(summary string) []string {
	if summary == "" {
		return nil
	}
	var notes []string
	for _, part := range strings.Split(summary, "; ") {
		switch part {
		case "terraform apply", "pulumi up", "cdk deploy":
			continue
		default:
			notes = append(notes, part)
		}
	}
	return notes
}

func joinNonEmpty(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "; " + b
}
