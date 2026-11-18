// Package cleanup turns idle and orphaned findings into a delete plan.
// The default is a dry run. Deletion runs only when Apply is set and the
// caller confirms, or when Yes is set for a script.
package cleanup

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/findings"
)

const (
	// ProtectKey is the tag that cleanup never deletes.
	// azguard:keep=true (also yes and 1) is protected even with --include-unsafe.
	ProtectKey = "azguard:keep"

	ActionAzureDelete           = "azure.delete"
	ActionAWSDeleteVolume       = "aws.ec2.DeleteVolume"
	ActionAWSReleaseAddress     = "aws.ec2.ReleaseAddress"
	ActionAWSDeleteNatGateway   = "aws.ec2.DeleteNatGateway"
	ActionAWSDeleteVpcEndpoints = "aws.ec2.DeleteVpcEndpoints"
	ActionAWSTerminateInstances = "aws.ec2.TerminateInstances"
	ActionAWSDeleteSnapshot     = "aws.ec2.DeleteSnapshot"
	ActionAWSDeleteLoadBalancer = "aws.elb.DeleteLoadBalancer"
)

// Options selects which findings are eligible and whether deletion is allowed.
type Options struct {
	Apply           bool
	Yes             bool
	Confirm         func(n int) (bool, error)
	OlderThan       time.Duration
	Regions         []string
	Types           []string
	Tags            map[string]string
	IncludeUnsafe   bool
	Now             time.Time
	DiscoveryErrors []string
}

// Item is one resource in a cleanup result.
type Item struct {
	ResourceID          string            `json:"resource_id"`
	Name                string            `json:"name"`
	Provider            string            `json:"provider"`
	Region              string            `json:"region"`
	Type                string            `json:"type"`
	RuleIDs             []string          `json:"rule_ids"`
	Age                 string            `json:"age"`
	CreatedAt           time.Time         `json:"created_at,omitempty"`
	EstimatedMonthlyUSD float64           `json:"estimated_monthly_usd"`
	PriceSource         string            `json:"price_source,omitempty"`
	Action              string            `json:"action,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	Hint                string            `json:"hint,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
}

// Result is the cleanup summary. Empty slices are encoded as [] so scripts can rely on them.
type Result struct {
	DryRun          bool     `json:"dry_run"`
	Applied         bool     `json:"applied"`
	Note            string   `json:"note,omitempty"`
	WouldDelete     []Item   `json:"would_delete"`
	Deleted         []Item   `json:"deleted"`
	Skipped         []Item   `json:"skipped"`
	Failed          []Item   `json:"failed"`
	DiscoveryErrors []string `json:"discovery_errors"`
}

// Deleter removes one planned resource. Tests supply a fake.
type Deleter interface {
	Delete(ctx context.Context, item Item) error
}

// Run plans, refuses to delete unless Apply is confirmed, then deletes.
// A nil Confirm and Yes false means confirmation was not given.
func Run(ctx context.Context, found []findings.Finding, opt Options, del Deleter) (Result, error) {
	res := Plan(found, opt)
	ok, err := Proceed(opt, len(res.WouldDelete))
	if err != nil {
		return res, err
	}
	if !ok {
		if opt.Apply && len(res.WouldDelete) > 0 {
			res.Note = "Confirmation was not given. Nothing was deleted."
		}
		return res, res.Err()
	}
	if del == nil {
		return res, fmt.Errorf("cleanup: deleter is not configured")
	}
	res = Apply(ctx, res, del)
	return res, res.Err()
}

// Proceed reports whether deletions should run.
// Apply must be set. Yes skips the prompt. A nil Confirm func does not confirm.
func Proceed(opt Options, pending int) (bool, error) {
	if !opt.Apply || pending == 0 {
		return false, nil
	}
	if opt.Yes {
		return true, nil
	}
	if opt.Confirm == nil {
		return false, nil
	}
	return opt.Confirm(pending)
}

// Plan classifies findings. It does not delete.
func Plan(found []findings.Finding, opt Options) Result {
	now := opt.now()
	res := Result{
		DryRun:          !opt.Apply,
		WouldDelete:     []Item{},
		Deleted:         []Item{},
		Skipped:         []Item{},
		Failed:          []Item{},
		DiscoveryErrors: append([]string{}, opt.DiscoveryErrors...),
	}
	for _, c := range mergeFindings(found) {
		item := c.item(now)
		if reason, skip := c.skipReason(opt, now); skip {
			item.Reason = reason
			res.Skipped = append(res.Skipped, item)
			continue
		}
		res.WouldDelete = append(res.WouldDelete, item)
	}
	sort.Slice(res.WouldDelete, func(i, j int) bool {
		if res.WouldDelete[i].EstimatedMonthlyUSD != res.WouldDelete[j].EstimatedMonthlyUSD {
			return res.WouldDelete[i].EstimatedMonthlyUSD > res.WouldDelete[j].EstimatedMonthlyUSD
		}
		return res.WouldDelete[i].ResourceID < res.WouldDelete[j].ResourceID
	})
	sort.Slice(res.Skipped, func(i, j int) bool {
		if res.Skipped[i].Reason != res.Skipped[j].Reason {
			return res.Skipped[i].Reason < res.Skipped[j].Reason
		}
		return res.Skipped[i].ResourceID < res.Skipped[j].ResourceID
	})
	return res
}

// Apply deletes every would-delete item. Failures are recorded and the rest continue.
func Apply(ctx context.Context, res Result, del Deleter) Result {
	out := res
	out.DryRun = false
	out.Applied = true
	out.Note = ""
	out.Deleted = []Item{}
	out.Failed = []Item{}
	out.WouldDelete = []Item{}
	for _, item := range res.WouldDelete {
		if del == nil {
			item.Reason = "deleter is not configured"
			out.Failed = append(out.Failed, item)
			continue
		}
		if err := del.Delete(ctx, item); err != nil {
			item.Reason = err.Error()
			out.Failed = append(out.Failed, item)
			continue
		}
		item.Reason = ""
		out.Deleted = append(out.Deleted, item)
	}
	return out
}

// Err is non-nil when a delete failed, discovery failed, or nothing was found
// because every scanned cloud is not configured.
func (r Result) Err() error {
	hard := hardDiscovery(r.DiscoveryErrors)
	if len(r.Failed) > 0 || len(hard) > 0 {
		return &RunError{Failed: len(r.Failed), Discovery: hard}
	}
	if len(r.WouldDelete)+len(r.Deleted)+len(r.Skipped) == 0 && len(r.DiscoveryErrors) > 0 {
		return &RunError{Discovery: append([]string{}, r.DiscoveryErrors...)}
	}
	return nil
}

// RunError is returned for failed deletes or discovery problems. ExitCode is 1.
type RunError struct {
	Failed    int
	Discovery []string
}

func (e *RunError) Error() string {
	var parts []string
	if e.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d resource(s) failed", e.Failed))
	}
	if len(e.Discovery) > 0 {
		parts = append(parts, "discovery errors: "+strings.Join(e.Discovery, "; "))
	}
	if len(parts) == 0 {
		return "cleanup failed"
	}
	return "cleanup: " + strings.Join(parts, "; ")
}

func (e *RunError) ExitCode() int { return 1 }

// ParseOlderThan accepts Go durations (720h) and a day suffix (30d).
func ParseOlderThan(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("invalid --older-than %q (examples: 720h, 30d)", s)
	}
	if strings.HasSuffix(strings.ToLower(s), "d") {
		n, err := strconv.Atoi(strings.TrimSpace(s[:len(s)-1]))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid --older-than %q (examples: 720h, 30d)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid --older-than %q (examples: 720h, 30d)", s)
	}
	return d, nil
}

// ParseTagFlag splits key=value on the first equals sign.
func ParseTagFlag(s string) (string, string, error) {
	key, val, ok := strings.Cut(s, "=")
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	if !ok || key == "" {
		return "", "", fmt.Errorf("invalid --tag %q (use key=value)", s)
	}
	return key, val, nil
}

// PutTag records a required tag. The same key with a different value is an error.
func PutTag(dst map[string]string, key, val string) error {
	for k, existing := range dst {
		if strings.EqualFold(k, key) {
			if !strings.EqualFold(existing, val) {
				return fmt.Errorf("repeated --tag %s with different values", key)
			}
			return nil
		}
	}
	dst[key] = val
	return nil
}

// FormatAge renders a create time as 40d, 5h, 12m, or unknown.
func FormatAge(created, now time.Time) string {
	if created.IsZero() {
		return "unknown"
	}
	d := now.Sub(created)
	if d < 0 {
		d = 0
	}
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return "0m"
}

// Protected reports whether tags contain azguard:keep set to a true value.
func Protected(tags map[string]string) bool {
	for k, v := range tags {
		if strings.EqualFold(strings.TrimSpace(k), ProtectKey) && truthy(v) {
			return true
		}
	}
	return false
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func (o Options) now() time.Time {
	if o.Now.IsZero() {
		return time.Now().UTC()
	}
	return o.Now.UTC()
}

func hardDiscovery(errs []string) []string {
	var out []string
	for _, e := range errs {
		if strings.TrimSpace(e) == "" || benignDiscovery(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func benignDiscovery(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not configured") || strings.Contains(m, "skipped azure") || strings.Contains(m, "skipped aws")
}

type candidate struct {
	ResourceID          string
	Name                string
	Provider            string
	Region              string
	RuleIDs             []string
	EstimatedMonthlyUSD float64
	PriceSource         string
	CreatedAt           time.Time
	Tags                map[string]string
	Safe                bool
	seen                bool
	Hint                string
	hintFromSafe        bool
	Action              string
	Type                string
}

func (c *candidate) add(f findings.Finding) {
	c.RuleIDs = append(c.RuleIDs, f.RuleID)
	if c.Name == "" {
		c.Name = f.Name
	}
	if c.Region == "" {
		c.Region = f.Region
	}
	if !c.seen || f.EstimatedMonthlyUSD > c.EstimatedMonthlyUSD {
		c.EstimatedMonthlyUSD = f.EstimatedMonthlyUSD
		c.PriceSource = f.PriceSource
	}
	if !c.seen {
		c.Safe = f.SafeToDelete
	} else {
		c.Safe = c.Safe && f.SafeToDelete
	}
	c.seen = true
	c.Tags = mergeTags(c.Tags, f.Tags)
	if !f.CreatedAt.IsZero() && (c.CreatedAt.IsZero() || f.CreatedAt.Before(c.CreatedAt)) {
		c.CreatedAt = f.CreatedAt
	}
	if f.SafeToDelete && f.SafeToDeleteHint != "" {
		if !c.hintFromSafe {
			c.Hint = f.SafeToDeleteHint
			c.hintFromSafe = true
		}
	} else if !c.hintFromSafe && c.Hint == "" {
		c.Hint = f.SafeToDeleteHint
	}
}

func (c candidate) item(now time.Time) Item {
	tags := c.Tags
	if tags != nil {
		cp := make(map[string]string, len(tags))
		for k, v := range tags {
			cp[k] = v
		}
		tags = cp
	}
	rules := append([]string{}, c.RuleIDs...)
	return Item{
		ResourceID:          c.ResourceID,
		Name:                c.Name,
		Provider:            c.Provider,
		Region:              c.Region,
		Type:                c.Type,
		RuleIDs:             rules,
		Age:                 FormatAge(c.CreatedAt, now),
		CreatedAt:           c.CreatedAt,
		EstimatedMonthlyUSD: c.EstimatedMonthlyUSD,
		PriceSource:         c.PriceSource,
		Action:              c.Action,
		Hint:                c.Hint,
		Tags:                tags,
	}
}

func (c candidate) skipReason(opt Options, now time.Time) (string, bool) {
	if len(opt.Regions) > 0 && !containsFold(opt.Regions, c.Region) {
		return "region does not match --region", true
	}
	if len(opt.Types) > 0 && !typeMatch(opt.Types, c.RuleIDs) {
		return "type does not match --type", true
	}
	if len(opt.Tags) > 0 && !hasTags(c.Tags, opt.Tags) {
		return "tags do not match --tag", true
	}
	if c.Tags == nil {
		return "tags were not loaded, so azguard:keep could not be checked", true
	}
	if Protected(c.Tags) {
		return "protected by tag azguard:keep=true", true
	}
	if !c.Safe && !opt.IncludeUnsafe {
		return "not marked safe to delete (pass --include-unsafe to include it)", true
	}
	if opt.OlderThan > 0 {
		if c.CreatedAt.IsZero() {
			return "age is unknown, so --older-than cannot be applied", true
		}
		if !c.CreatedAt.Before(now.Add(-opt.OlderThan)) {
			return "not older than " + formatDuration(opt.OlderThan), true
		}
	}
	if c.Action == "" {
		return "no delete operation for this resource", true
	}
	return "", false
}

func mergeFindings(found []findings.Finding) []candidate {
	var order []string
	groups := map[string]*candidate{}
	for _, f := range found {
		if strings.TrimSpace(f.ResourceID) == "" {
			continue
		}
		k := dedupeKey(f)
		c := groups[k]
		if c == nil {
			c = &candidate{ResourceID: f.ResourceID, Provider: f.Provider}
			groups[k] = c
			order = append(order, k)
		}
		c.add(f)
	}
	out := make([]candidate, 0, len(order))
	for _, k := range order {
		c := groups[k]
		c.RuleIDs = uniqueFold(c.RuleIDs)
		sort.Strings(c.RuleIDs)
		c.Action = actionFor(c.Provider, c.ResourceID, c.RuleIDs)
		c.Type = strings.Join(c.RuleIDs, ",")
		out = append(out, *c)
	}
	return out
}

func dedupeKey(f findings.Finding) string {
	id := f.ResourceID
	if f.Provider == "azure" {
		id = strings.ToLower(id)
	}
	return f.Provider + "\n" + id
}

func mergeTags(dst, src map[string]string) map[string]string {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = map[string]string{}
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func actionFor(provider, resourceID string, ruleIDs []string) string {
	for _, id := range ruleIDs {
		if a := actionForRule(provider, resourceID, id); a != "" {
			return a
		}
	}
	return ""
}

func actionForRule(provider, resourceID, rule string) string {
	if provider == "azure" {
		switch rule {
		case "azure.disk.unattached",
			"azure.publicip.unassociated",
			"azure.nic.unattached",
			"azure.vm.stopped_allocated",
			"azure.appserviceplan.empty",
			"azure.snapshot.old",
			"azure.natgateway.no_subnets",
			"azure.loganalytics.empty_ingestion":
			return ActionAzureDelete
		default:
			return ""
		}
	}
	switch rule {
	case "aws.ebs.unattached":
		return ActionAWSDeleteVolume
	case "aws.eip.unassociated":
		return ActionAWSReleaseAddress
	case "aws.ipv4.public":
		if strings.HasPrefix(resourceID, "eipalloc-") {
			return ActionAWSReleaseAddress
		}
		return ""
	case "aws.natgateway.idle":
		return ActionAWSDeleteNatGateway
	case "aws.vpcendpoint.interface":
		return ActionAWSDeleteVpcEndpoints
	case "aws.ec2.stopped_with_ebs":
		return ActionAWSTerminateInstances
	case "aws.snapshot.old":
		return ActionAWSDeleteSnapshot
	case "aws.elb.no_healthy_targets":
		return ActionAWSDeleteLoadBalancer
	default:
		return ""
	}
}

func typeMatch(filters, ruleIDs []string) bool {
	for _, f := range filters {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if ids, ok := typeAliases[f]; ok {
			for _, id := range ids {
				if contains(ruleIDs, id) {
					return true
				}
			}
			continue
		}
		for _, id := range ruleIDs {
			if strings.EqualFold(id, f) || strings.HasSuffix(strings.ToLower(id), "."+f) {
				return true
			}
		}
	}
	return false
}

var typeAliases = map[string][]string{
	"disk":           {"azure.disk.unattached"},
	"ebs":            {"aws.ebs.unattached"},
	"volume":         {"azure.disk.unattached", "aws.ebs.unattached"},
	"public-ip":      {"azure.publicip.unassociated", "aws.eip.unassociated", "aws.ipv4.public"},
	"publicip":       {"azure.publicip.unassociated", "aws.eip.unassociated", "aws.ipv4.public"},
	"eip":            {"aws.eip.unassociated", "aws.ipv4.public"},
	"nic":            {"azure.nic.unattached"},
	"vm":             {"azure.vm.stopped_allocated"},
	"ec2":            {"aws.ec2.stopped_with_ebs"},
	"instance":       {"azure.vm.stopped_allocated", "aws.ec2.stopped_with_ebs"},
	"plan":           {"azure.appserviceplan.empty"},
	"appservice":     {"azure.appserviceplan.empty"},
	"appserviceplan": {"azure.appserviceplan.empty"},
	"snapshot":       {"azure.snapshot.old", "aws.snapshot.old"},
	"nat":            {"azure.natgateway.no_subnets", "aws.natgateway.idle"},
	"natgateway":     {"azure.natgateway.no_subnets", "aws.natgateway.idle"},
	"workspace":      {"azure.loganalytics.empty_ingestion"},
	"loganalytics":   {"azure.loganalytics.empty_ingestion"},
	"vpc-endpoint":   {"aws.vpcendpoint.interface"},
	"endpoint":       {"aws.vpcendpoint.interface"},
	"elb":            {"aws.elb.no_healthy_targets"},
	"loadbalancer":   {"aws.elb.no_healthy_targets"},
	"load-balancer":  {"aws.elb.no_healthy_targets"},
	"ipv4":           {"aws.ipv4.public"},
}

func hasTags(have, want map[string]string) bool {
	if have == nil {
		return false
	}
	for wk, wv := range want {
		found := false
		for hk, hv := range have {
			if strings.EqualFold(hk, wk) && strings.EqualFold(hv, wv) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func uniqueFold(ids []string) []string {
	seen := map[string]string{}
	var out []string
	for _, id := range ids {
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = id
		out = append(out, id)
	}
	return out
}

func formatDuration(d time.Duration) string {
	if d > 0 && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
	return d.String()
}
