package watch

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/events"
	"github.com/cdobratz/azguard/internal/pricing"
)

// Alert is one new billable resource reported by `azguard watch`.
type Alert struct {
	ID                  string    `json:"id"`
	Provider            string    `json:"provider"`
	Operation           string    `json:"operation"`
	ResourceID          string    `json:"resource_id"`
	Region              string    `json:"region"`
	At                  time.Time `json:"at"`
	Principal           string    `json:"principal"`
	Automation          bool      `json:"automation"`
	AutomationReason    string    `json:"automation_reason,omitempty"`
	EstimatedMonthlyUSD float64   `json:"estimated_monthly_usd"`
	PriceSource         string    `json:"price_source"`
	Evidence            string    `json:"evidence"`
}

type pricedOp struct {
	key string
	qty float64
}

// AlertsFrom keeps succeeded billable operations and prices them from book.
func AlertsFrom(evs []events.Event, book *pricing.Book) []Alert {
	if book == nil {
		book = pricing.Bundled()
	}
	var out []Alert
	for _, ev := range evs {
		if !succeeded(ev) {
			continue
		}
		op, ok := priceOp(ev)
		if !ok {
			continue
		}
		usd, src := 0.0, pricing.SourceUnpriced
		if op.key != "" {
			var priced bool
			usd, src, priced = book.Monthly(op.key, ev.Region, op.qty)
			if !priced {
				usd, src = 0, pricing.SourceUnpriced
			}
		}
		auto, reason := events.Automation(ev.Principal)
		principal := ev.Principal.Display
		if principal == "" {
			principal = ev.Principal.Caller
		}
		evidence := fmt.Sprintf("%s %s by %s", ev.Name, empty(ev.ResourceID, "(resource id unknown)"), empty(principal, "unknown principal"))
		if auto {
			evidence += " [" + reason + "]"
		}
		if src == pricing.SourceUnpriced {
			evidence += ". Monthly estimate unavailable for this operation; bundled list prices cover the common SKUs."
		} else {
			evidence += fmt.Sprintf(". Estimated $%.2f/month from the bundled list-price table (730-hour month), not an invoice.", usd)
		}
		out = append(out, Alert{
			ID:                  ev.ID,
			Provider:            ev.Provider,
			Operation:           ev.Name,
			ResourceID:          ev.ResourceID,
			Region:              ev.Region,
			At:                  ev.At,
			Principal:           principal,
			Automation:          auto,
			AutomationReason:    reason,
			EstimatedMonthlyUSD: usd,
			PriceSource:         src,
			Evidence:            evidence,
		})
	}
	return out
}

func succeeded(ev events.Event) bool {
	if ev.Status == "" {
		return true
	}
	return strings.EqualFold(ev.Status, "Succeeded") || strings.EqualFold(ev.Status, "success")
}

func priceOp(ev events.Event) (pricedOp, bool) {
	name := strings.ToLower(ev.Name)
	switch ev.Provider {
	case "azure":
		switch name {
		case "microsoft.network/publicipaddresses/write":
			return pricedOp{key: pricing.KeyAzurePublicIPStandard, qty: 1}, true
		case "microsoft.network/natgateways/write":
			return pricedOp{key: pricing.KeyAzureNATGateway, qty: 1}, true
		case "microsoft.compute/virtualmachines/write":
			return pricedOp{key: "azure.vm.standard_b1s", qty: 1}, true
		case "microsoft.compute/disks/write":
			return pricedOp{key: "azure.disk.hdd.s10", qty: 1}, true
		case "microsoft.web/serverfarms/write":
			return pricedOp{key: "azure.plan.b1", qty: 1}, true
		case "microsoft.compute/snapshots/write":
			return pricedOp{key: pricing.KeyAzureSnapshotGB, qty: 32}, true
		case "microsoft.network/loadbalancers/write",
			"microsoft.network/applicationgateways/write",
			"microsoft.sql/servers/write",
			"microsoft.dbforpostgresql/flexibleservers/write",
			"microsoft.containerservice/managedclusters/write",
			"microsoft.operationalinsights/workspaces/write":
			return pricedOp{}, true
		default:
			return pricedOp{}, false
		}
	case "aws":
		switch ev.Name {
		case "AllocateAddress":
			return pricedOp{key: pricing.KeyAWSPublicIPv4, qty: 1}, true
		case "CreateNatGateway":
			return pricedOp{key: pricing.KeyAWSNATGateway, qty: 1}, true
		case "RunInstances":
			key := pricing.AWSEC2Key(detail(ev, "instanceType", "t3.micro"))
			return pricedOp{key: key, qty: detailFloat(ev, "count", 1)}, true
		case "CreateVolume":
			vtype := detail(ev, "volumeType", "gp3")
			return pricedOp{key: pricing.AWSEBSKey(vtype), qty: detailFloat(ev, "size", 8)}, true
		case "CreateVpcEndpoint":
			if strings.EqualFold(detail(ev, "vpcEndpointType", "Interface"), "Gateway") {
				return pricedOp{}, false
			}
			return pricedOp{key: pricing.KeyAWSVPCEndpointHour, qty: 1}, true
		case "CreateLoadBalancer":
			return pricedOp{key: pricing.KeyAWSALBHour, qty: 1}, true
		case "CreateStack", "CreateDBInstance":
			return pricedOp{}, true
		default:
			return pricedOp{}, false
		}
	default:
		return pricedOp{}, false
	}
}

func detail(ev events.Event, key, fallback string) string {
	if ev.Detail == nil {
		return fallback
	}
	if v := ev.Detail[key]; v != "" {
		return v
	}
	return fallback
}

func detailFloat(ev events.Event, key string, fallback float64) float64 {
	if ev.Detail == nil {
		return fallback
	}
	v := ev.Detail[key]
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func empty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// FormatLine is the one-line human report for an alert.
func FormatLine(a Alert) string {
	who := a.Principal
	if a.Automation {
		who += " (automation: " + a.AutomationReason + ")"
	}
	return fmt.Sprintf("%s %s %s %s principal=%s est=$%.2f/month",
		a.At.UTC().Format(time.RFC3339), a.Provider, a.Operation, empty(a.ResourceID, "-"), who, a.EstimatedMonthlyUSD)
}
