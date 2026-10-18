package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/events"
	"github.com/azguard/azguard/internal/pricing"
	"github.com/azguard/azguard/internal/watch"
)

// EventsFromFixture parses an Activity Log page or a CloudTrail LookupEvents page.
func EventsFromFixture(body []byte) ([]events.Event, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("guard fixture: %w", err)
	}
	if _, ok := probe["Events"]; ok {
		ev, _, err := aws.ParseLookupEvents(body)
		return ev, err
	}
	if _, ok := probe["value"]; ok {
		ev, _, err := azure.ParseActivity(body)
		return ev, err
	}
	return nil, fmt.Errorf("guard fixture: expected an Activity Log page or a LookupEvents page")
}

// CreatedDuring keeps succeeded billable events at or after since whose principal matches.
// An empty principal matches every caller.
func CreatedDuring(evs []events.Event, since time.Time, principal string, book *pricing.Book) []watch.Alert {
	var kept []events.Event
	for _, ev := range evs {
		if !since.IsZero() && !ev.At.IsZero() && ev.At.Before(since) {
			continue
		}
		if principal != "" && !principalMatch(ev.Principal, principal) {
			continue
		}
		kept = append(kept, ev)
	}
	return watch.AlertsFrom(kept, book)
}

func principalMatch(p events.Principal, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return true
	}
	for _, v := range []string{p.Display, p.Caller, p.UPN, p.ARN} {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}

// WriteReport prints resources created during the session.
func WriteReport(w io.Writer, alerts []watch.Alert) {
	if len(alerts) == 0 {
		fmt.Fprintln(w, "No billable resources in this window.")
		return
	}
	fmt.Fprintln(w, "WHEN                  PROVIDER  OPERATION                          PRINCIPAL                         EST/MONTH  RESOURCE")
	for _, a := range alerts {
		who := a.Principal
		if a.Automation {
			who += " (automation)"
		}
		fmt.Fprintf(w, "%s  %-8s  %-32s  %-32s  %8.2f  %s\n",
			a.At.UTC().Format(time.RFC3339), a.Provider, trim(a.Operation, 32), trim(who, 32), a.EstimatedMonthlyUSD, a.ResourceID)
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
