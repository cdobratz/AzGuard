package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/cost"
	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/guard"
	"github.com/cdobratz/azguard/internal/preflight"
	"github.com/cdobratz/azguard/internal/pricing"
	"github.com/cdobratz/azguard/internal/watch"
)

const creditNote = "Bundled free-tier limits and any credit stored by azguard init. This does not query live usage or current spend."

func (d Deps) idle(ctx context.Context, args idleArgs) (string, error) {
	cloud := strings.ToLower(strings.TrimSpace(args.Cloud))
	if cloud == "" {
		cloud = "all"
	}
	switch cloud {
	case "all", "azure", "aws":
	default:
		return "", fmt.Errorf("cloud must be all, azure, or aws")
	}
	if d.ListIdle == nil {
		return encode(idleResult{Message: "idle resource scan is not configured"})
	}
	report, err := d.ListIdle(ctx, cloud)
	if err != nil {
		return "", err
	}
	msg := "No idle or orphaned resources found."
	if len(report.Findings) > 0 {
		msg = fmt.Sprintf("Found %d idle or orphaned finding(s), about $%.2f/month.", len(report.Findings), report.TotalEstimatedMonthlyUSD)
	}
	if len(report.Errors) > 0 && len(report.Findings) == 0 {
		msg = "Idle scan did not return findings. " + strings.Join(report.Errors, " ")
	}
	return encode(idleResult{Message: msg, Report: report})
}

type idleResult struct {
	Message string `json:"message"`
	findings.Report
}

func (d Deps) listNew(ctx context.Context, args newArgs) (string, error) {
	since, err := time.Parse(time.RFC3339, strings.TrimSpace(args.Since))
	if err != nil {
		return "", fmt.Errorf("since must be RFC3339: %w", err)
	}
	if d.ListNew == nil {
		return encode(newResult{
			Message: "new billable resource scan is not configured",
			Since:   since.UTC().Format(time.RFC3339),
			Alerts:  []watch.Alert{},
			Notes:   []string{},
		})
	}
	alerts, notes, err := d.ListNew(ctx, since, strings.TrimSpace(args.Principal))
	if err != nil {
		return "", err
	}
	if alerts == nil {
		alerts = []watch.Alert{}
	}
	if notes == nil {
		notes = []string{}
	}
	msg := "No billable resources in this window."
	if len(alerts) > 0 {
		var total float64
		for _, a := range alerts {
			total += a.EstimatedMonthlyUSD
		}
		msg = fmt.Sprintf("Found %d billable create(s) since %s, about $%.2f/month.", len(alerts), since.UTC().Format(time.RFC3339), pricing.RoundUSD(total))
	}
	return encode(newResult{
		Message: msg,
		Since:   since.UTC().Format(time.RFC3339),
		Alerts:  alerts,
		Notes:   notes,
	})
}

type newResult struct {
	Message string        `json:"message"`
	Since   string        `json:"since"`
	Alerts  []watch.Alert `json:"alerts"`
	Notes   []string      `json:"notes"`
}

// AlertsFromFixture parses a recorded Activity Log page or CloudTrail LookupEvents
// page and keeps billable creates at or after since for principal.
func AlertsFromFixture(body []byte, since time.Time, principal string) ([]watch.Alert, error) {
	ev, err := guard.EventsFromFixture(body)
	if err != nil {
		return nil, err
	}
	return guard.CreatedDuring(ev, since, principal, pricing.Bundled()), nil
}

func (d Deps) creditStatus(args statusArgs) (string, error) {
	cloud := strings.ToLower(strings.TrimSpace(args.Cloud))
	if cloud == "" {
		cloud = "all"
	}
	switch cloud {
	case "all", "azure", "aws":
	default:
		return "", fmt.Errorf("cloud must be all, azure, or aws")
	}
	var acct Account
	if d.Account != nil {
		acct = d.Account()
	}
	var clouds []cloudCredit
	if cloud == "all" || cloud == "azure" {
		limits, err := cost.LoadFreeTierConfig()
		if err != nil {
			return "", err
		}
		clouds = append(clouds, cloudCredit{
			Cloud:           "azure",
			AccountKind:     emptyKind(acct.AzureKind),
			CreditRemaining: acct.AzureCredit,
			CreditExpires:   acct.AzureCreditExpires,
			Services:        servicesOf(limits),
		})
	}
	if cloud == "all" || cloud == "aws" {
		limits, err := cost.LoadAWSFreeTierConfig()
		if err != nil {
			return "", err
		}
		clouds = append(clouds, cloudCredit{
			Cloud:           "aws",
			AccountKind:     emptyKind(acct.AWSKind),
			CreditRemaining: acct.AWSCredit,
			CreditExpires:   acct.AWSCreditExpires,
			Services:        servicesOf(limits),
		})
	}
	return encode(creditResult{Message: creditNote, Clouds: clouds, Note: creditNote})
}

func emptyKind(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "unknown"
	}
	return kind
}

func servicesOf(cfg *cost.FreeTierConfig) []freeService {
	if cfg == nil {
		return []freeService{}
	}
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]freeService, 0, len(names))
	for _, name := range names {
		svc := cfg.Services[name]
		out = append(out, freeService{
			Name:        name,
			Description: svc.Description,
			Limit:       svc.Limit,
			Unit:        svc.Unit,
			Duration:    svc.Duration,
		})
	}
	return out
}

type creditResult struct {
	Message string        `json:"message"`
	Clouds  []cloudCredit `json:"clouds"`
	Note    string        `json:"note"`
}

type cloudCredit struct {
	Cloud           string        `json:"cloud"`
	AccountKind     string        `json:"account_kind"`
	CreditRemaining *float64      `json:"credit_remaining,omitempty"`
	CreditExpires   string        `json:"credit_expires,omitempty"`
	Services        []freeService `json:"services"`
}

type freeService struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Limit       float64 `json:"limit"`
	Unit        string  `json:"unit"`
	Duration    string  `json:"duration"`
}

func estimateMonthly(args estimateArgs) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(args.Provider))
	kind := normalizeKind(args.Kind)
	if provider == "" || kind == "" {
		return "", fmt.Errorf("provider and kind are required")
	}
	key, qty, label, ok := priceSpec(provider, kind, args.SKU, args.Quantity)
	res := estimateResult{
		Provider: provider,
		Kind:     kind,
		SKU:      args.SKU,
		Quantity: qty,
		Region:   args.Region,
		Label:    label,
		Note:     preflight.PriceNote,
	}
	if !ok {
		res.Message = fmt.Sprintf("AzGuard has no bundled monthly price for %s %s.", provider, kind)
		res.Source = pricing.SourceUnpriced
		return encode(res)
	}
	usd, src, priced := pricing.Bundled().Monthly(key, args.Region, qty)
	res.PriceKey = key
	res.Source = src
	res.Priced = priced
	res.MonthlyUSD = usd
	if !priced {
		res.Message = fmt.Sprintf("AzGuard has no bundled monthly price for %s.", label)
		res.Source = pricing.SourceUnpriced
		return encode(res)
	}
	res.Message = fmt.Sprintf("a %s is about $%.2f/month", label, usd)
	return encode(res)
}

type estimateResult struct {
	Message    string  `json:"message"`
	Provider   string  `json:"provider"`
	Kind       string  `json:"kind"`
	SKU        string  `json:"sku,omitempty"`
	Quantity   float64 `json:"quantity"`
	Region     string  `json:"region,omitempty"`
	Label      string  `json:"label"`
	PriceKey   string  `json:"price_key,omitempty"`
	MonthlyUSD float64 `json:"monthly_usd"`
	Priced     bool    `json:"priced"`
	Source     string  `json:"source"`
	Note       string  `json:"note"`
}

func normalizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	kind = strings.ReplaceAll(kind, "-", "_")
	kind = strings.ReplaceAll(kind, " ", "_")
	return kind
}

func priceSpec(provider, kind, sku string, qty float64) (key string, quantity float64, label string, ok bool) {
	if qty < 0 {
		qty = 0
	}
	switch provider + "." + kind {
	case "azure.nat_gateway", "azure.nat":
		return pricing.KeyAzureNATGateway, one(qty), "NAT Gateway", true
	case "aws.nat_gateway", "aws.nat":
		return pricing.KeyAWSNATGateway, one(qty), "NAT Gateway", true
	case "azure.public_ip", "azure.pip", "azure.publicip":
		return pricing.KeyAzurePublicIPStandard, one(qty), "public IP", true
	case "aws.public_ip", "aws.eip", "aws.ipv4":
		return pricing.KeyAWSPublicIPv4, one(qty), "public IP", true
	case "azure.vm", "azure.virtual_machine":
		size := sku
		if size == "" {
			size = "Standard_B1s"
		}
		return pricing.AzureVMKey(size), one(qty), "virtual machine (" + size + ")", true
	case "aws.instance", "aws.ec2", "aws.vm":
		it := sku
		if it == "" {
			it = "t3.micro"
		}
		return pricing.AWSEC2Key(it), one(qty), "EC2 instance (" + it + ")", true
	case "aws.ebs", "aws.volume":
		vt := sku
		if vt == "" {
			vt = "gp3"
		}
		size := qty
		if size <= 0 {
			size = 8
		}
		return pricing.AWSEBSKey(vt), size, "EBS volume", true
	case "azure.disk", "azure.managed_disk":
		size := qty
		if size <= 0 {
			size = 128
		}
		return pricing.AzureDiskKey(sku, size), 1, "managed disk", true
	case "azure.snapshot":
		size := qty
		if size <= 0 {
			size = 32
		}
		return pricing.KeyAzureSnapshotGB, size, "snapshot", true
	case "aws.snapshot":
		size := qty
		if size <= 0 {
			size = 32
		}
		return pricing.KeyAWSSnapshotGB, size, "snapshot", true
	case "azure.plan", "azure.app_service_plan":
		name := sku
		if name == "" {
			name = "B1"
		}
		return pricing.AzurePlanKey(name), 1, "App Service plan (" + name + ")", true
	case "aws.load_balancer", "aws.lb", "aws.alb":
		return pricing.KeyAWSALBHour, one(qty), "load balancer", true
	case "aws.nlb":
		return pricing.KeyAWSNLBHour, one(qty), "network load balancer", true
	case "aws.vpc_endpoint", "aws.endpoint":
		return pricing.KeyAWSVPCEndpointHour, one(qty), "VPC endpoint", true
	default:
		return "", qty, provider + " " + kind, false
	}
}

func one(qty float64) float64 {
	if qty <= 0 {
		return 1
	}
	return qty
}

func runPreflight(args preflightArgs) (string, error) {
	input := strings.TrimSpace(args.Input)
	command := strings.TrimSpace(args.Command)
	plan := strings.TrimSpace(args.Plan)
	if input == "" && command == "" && plan == "" {
		return "", fmt.Errorf("preflight needs input, command, or plan")
	}
	var (
		res preflight.Result
		err error
	)
	switch {
	case input != "":
		res, err = preflight.Inspect(input)
	case plan != "" && command != "":
		res, err = preflight.InspectWithPlan(command, []byte(plan))
	case plan != "":
		res, err = preflight.Inspect(plan)
	default:
		res, err = preflight.Inspect(command)
	}
	if err != nil {
		return "", err
	}
	return encode(res)
}

func encode(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}
