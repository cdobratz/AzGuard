package cleanup

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Write renders a cleanup result as table, json, or csv.
func Write(w io.Writer, res Result, format string) error {
	switch format {
	case "", "table":
		return writeTable(w, res)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	case "csv":
		return writeCSV(w, res)
	default:
		return fmt.Errorf("unknown output format %q (use table, json, or csv)", format)
	}
}

func writeTable(w io.Writer, res Result) error {
	switch {
	case res.Note != "":
		if _, err := fmt.Fprintln(w, res.Note); err != nil {
			return err
		}
	case res.Applied:
		if _, err := fmt.Fprintln(w, "Cleanup applied."); err != nil {
			return err
		}
	case res.DryRun:
		if _, err := fmt.Fprintln(w, "Dry run. No resources were deleted."); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintln(w, "Nothing was deleted."); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := writeActionTable(w, "WOULD DELETE", res.WouldDelete); err != nil {
		return err
	}
	if err := writeActionTable(w, "DELETED", res.Deleted); err != nil {
		return err
	}
	if err := writeReasonTable(w, "SKIPPED", res.Skipped); err != nil {
		return err
	}
	if err := writeReasonTable(w, "FAILED", res.Failed); err != nil {
		return err
	}
	for _, e := range res.DiscoveryErrors {
		if _, err := fmt.Fprintf(w, "discovery: %s\n", e); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "Summary: would delete %d, deleted %d, skipped %d, failed %d\n",
		len(res.WouldDelete), len(res.Deleted), len(res.Skipped), len(res.Failed))
	return err
}

func writeActionTable(w io.Writer, title string, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s\n", title); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-46s %-32s %-16s %-8s %10s\n", "RESOURCE ID", "TYPE", "REGION", "AGE", "EST $/MO"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "%-46s %-32s %-16s %-8s %10s\n",
			item.ResourceID, item.Type, item.Region, item.Age, formatCost(item)); err != nil {
			return err
		}
		if item.Hint != "" {
			if _, err := fmt.Fprintf(w, "    hint: %s\n", item.Hint); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeReasonTable(w io.Writer, title string, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s\n", title); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%-46s %-32s %s\n", "RESOURCE ID", "TYPE", "REASON"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, "%-46s %-32s %s\n", item.ResourceID, item.Type, item.Reason); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func formatCost(item Item) string {
	if item.PriceSource == "unpriced" {
		return "n/a"
	}
	return fmt.Sprintf("$%.2f", item.EstimatedMonthlyUSD)
}

func writeCSV(w io.Writer, res Result) error {
	cw := csv.NewWriter(w)
	header := []string{"status", "resource_id", "type", "provider", "region", "age", "estimated_monthly_usd", "reason"}
	if err := cw.Write(header); err != nil {
		return err
	}
	rows := []struct {
		status string
		items  []Item
	}{
		{"would-delete", res.WouldDelete},
		{"deleted", res.Deleted},
		{"skipped", res.Skipped},
		{"failed", res.Failed},
	}
	for _, group := range rows {
		for _, item := range group.items {
			if err := cw.Write([]string{
				group.status,
				item.ResourceID,
				item.Type,
				item.Provider,
				item.Region,
				item.Age,
				strconv.FormatFloat(item.EstimatedMonthlyUSD, 'f', 2, 64),
				item.Reason,
			}); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}
