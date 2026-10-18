// Package findings is the idle and orphaned resource rule engine.
//
// A finding is one resource that matched one rule. The report total counts each
// resource once when two rules match the same id (for example an unassociated
// Elastic IP is both aws.eip.unassociated and aws.ipv4.public).
package findings

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/azguard/azguard/internal/pricing"
)

const (
	SchemaName    = "azguard.findings.v1"
	SchemaVersion = "1"
)

// Rule describes one idle or orphaned resource check.
type Rule struct {
	ID       string
	Provider string
	Title    string
}

// Catalog is the stable list of rules `azguard resources` runs.
func Catalog() []Rule {
	return []Rule{
		{ID: "azure.disk.unattached", Provider: "azure", Title: "Unattached managed disk"},
		{ID: "azure.publicip.unassociated", Provider: "azure", Title: "Unassociated public IP"},
		{ID: "azure.nic.unattached", Provider: "azure", Title: "Unattached network interface"},
		{ID: "azure.vm.stopped_allocated", Provider: "azure", Title: "VM stopped (allocated) and still billed"},
		{ID: "azure.appserviceplan.empty", Provider: "azure", Title: "App Service plan with no apps"},
		{ID: "azure.snapshot.old", Provider: "azure", Title: "Snapshot older than the age threshold"},
		{ID: "azure.natgateway.no_subnets", Provider: "azure", Title: "NAT gateway with no subnets"},
		{ID: "azure.loganalytics.empty_ingestion", Provider: "azure", Title: "Empty Log Analytics workspace that is still ingesting"},
		{ID: "aws.ebs.unattached", Provider: "aws", Title: "Unattached EBS volume"},
		{ID: "aws.eip.unassociated", Provider: "aws", Title: "Unassociated Elastic IP"},
		{ID: "aws.ipv4.public", Provider: "aws", Title: "Public IPv4 address"},
		{ID: "aws.natgateway.idle", Provider: "aws", Title: "NAT gateway with no bytes processed for 7 days"},
		{ID: "aws.vpcendpoint.interface", Provider: "aws", Title: "VPC interface endpoint"},
		{ID: "aws.ec2.stopped_with_ebs", Provider: "aws", Title: "Stopped EC2 instance with EBS volumes"},
		{ID: "aws.elb.no_healthy_targets", Provider: "aws", Title: "Load balancer with no healthy targets"},
		{ID: "aws.snapshot.old", Provider: "aws", Title: "Snapshot older than the age threshold"},
	}
}

// Finding is one matched resource.
//
// safe_to_delete means the resource is not attached to a running service.
// It does not mean deletion is free of data loss. Read safe_to_delete_hint.
// This command never deletes anything.
type Finding struct {
	ResourceID          string  `json:"resource_id"`
	Name                string  `json:"name"`
	Provider            string  `json:"provider"`
	Region              string  `json:"region"`
	RuleID              string  `json:"rule_id"`
	Title               string  `json:"title"`
	Evidence            string  `json:"evidence"`
	EstimatedMonthlyUSD float64 `json:"estimated_monthly_usd"`
	Currency            string  `json:"currency"`
	CreatedBy           string  `json:"created_by"`
	SafeToDelete        bool    `json:"safe_to_delete"`
	SafeToDeleteHint    string  `json:"safe_to_delete_hint"`
	PriceSource         string  `json:"price_source"`

	// PriceKey and PriceQuantity let a later Retail Prices / Price List response
	// replace the bundled estimate. They are not part of the output schema.
	PriceKey      string  `json:"-"`
	PriceQuantity float64 `json:"-"`
}

// Report is the documented JSON output of `azguard resources --output json`.
type Report struct {
	Schema                   string    `json:"schema"`
	SchemaVersion            string    `json:"schema_version"`
	GeneratedAt              string    `json:"generated_at"`
	Findings                 []Finding `json:"findings"`
	TotalEstimatedMonthlyUSD float64   `json:"total_estimated_monthly_usd"`
	Currency                 string    `json:"currency"`
	Errors                   []string  `json:"errors"`
}

// Options controls age and ingestion thresholds.
type Options struct {
	Now               time.Time
	SnapshotAge       time.Duration
	MinIngestionBytes float64
}

func (o Options) now() time.Time {
	if o.Now.IsZero() {
		return time.Now().UTC()
	}
	return o.Now.UTC()
}

func (o Options) snapshotAge() time.Duration {
	if o.SnapshotAge <= 0 {
		return 30 * 24 * time.Hour
	}
	return o.SnapshotAge
}

func (o Options) minIngestion() float64 {
	if o.MinIngestionBytes <= 0 {
		return 1
	}
	return o.MinIngestionBytes
}

// NewReport sorts findings and computes the deduplicated monthly total.
func NewReport(found []Finding, errs []string, at time.Time) Report {
	if found == nil {
		found = []Finding{}
	}
	if errs == nil {
		errs = []string{}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].EstimatedMonthlyUSD != found[j].EstimatedMonthlyUSD {
			return found[i].EstimatedMonthlyUSD > found[j].EstimatedMonthlyUSD
		}
		if found[i].RuleID != found[j].RuleID {
			return found[i].RuleID < found[j].RuleID
		}
		return found[i].ResourceID < found[j].ResourceID
	})
	if at.IsZero() {
		at = time.Now()
	}
	return Report{
		Schema:                   SchemaName,
		SchemaVersion:            SchemaVersion,
		GeneratedAt:              at.UTC().Format(time.RFC3339),
		Findings:                 found,
		TotalEstimatedMonthlyUSD: TotalMonthly(found),
		Currency:                 "USD",
		Errors:                   errs,
	}
}

// TotalMonthly sums estimated cost once per provider and resource id.
// When two rules match the same resource, the higher estimate is kept.
func TotalMonthly(found []Finding) float64 {
	best := map[string]float64{}
	for _, f := range found {
		key := dedupeKey(f)
		if cur, ok := best[key]; !ok || f.EstimatedMonthlyUSD > cur {
			best[key] = f.EstimatedMonthlyUSD
		}
	}
	var total float64
	for _, v := range best {
		total += v
	}
	return pricing.RoundUSD(total)
}

func dedupeKey(f Finding) string {
	id := f.ResourceID
	if f.Provider == "azure" {
		id = strings.ToLower(id)
	}
	return f.Provider + "\n" + id
}

// Reprice replaces bundled estimates when book has a price for the finding's key.
func Reprice(found []Finding, book *pricing.Book) []Finding {
	out := make([]Finding, len(found))
	for i, f := range found {
		if f.PriceKey != "" && book != nil {
			if usd, src, ok := book.Monthly(f.PriceKey, f.Region, f.PriceQuantity); ok {
				f.EstimatedMonthlyUSD = usd
				f.PriceSource = src
			}
		}
		out[i] = f
	}
	return out
}

// FailOverError is returned when the deduplicated total exceeds --fail-over.
// ExitCode is 2.
type FailOverError struct {
	Total float64
	Limit float64
}

func (e *FailOverError) Error() string {
	return fmt.Sprintf("estimated monthly cost $%.2f exceeds --fail-over $%.2f", e.Total, e.Limit)
}

func (e *FailOverError) ExitCode() int { return 2 }

// Exceeds reports whether total is greater than limit. A negative limit disables the check.
func Exceeds(total, limit float64) bool {
	if limit < 0 {
		return false
	}
	return total > limit
}

func titleOf(id string) string {
	for _, r := range Catalog() {
		if r.ID == id {
			return r.Title
		}
	}
	return id
}

func makeFinding(f Finding) Finding {
	if f.Currency == "" {
		f.Currency = "USD"
	}
	if f.Title == "" {
		f.Title = titleOf(f.RuleID)
	}
	f.EstimatedMonthlyUSD = pricing.RoundUSD(f.EstimatedMonthlyUSD)
	return f
}

func quote(book *pricing.Book, key, region string, qty float64) (usd float64, source string) {
	if book == nil {
		book = pricing.Bundled()
	}
	usd, source, ok := book.Monthly(key, region, qty)
	if !ok {
		return 0, pricing.SourceUnpriced
	}
	return usd, source
}
