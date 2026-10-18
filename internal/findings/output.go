package findings

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Write renders a report as table, json, or csv.
func Write(w io.Writer, report Report, format string) error {
	switch format {
	case "", "table":
		return writeTable(w, report)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "csv":
		return writeCSV(w, report)
	default:
		return fmt.Errorf("unknown output format %q (use table, json, or csv)", format)
	}
}

func writeTable(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "Idle and orphaned resources  (%s)\n", report.GeneratedAt); err != nil {
		return err
	}
	if len(report.Findings) == 0 {
		if _, err := fmt.Fprintln(w, "No idle or orphaned resources found."); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%-36s %-16s %10s  %-5s  %s\n", "RULE", "REGION", "MONTHLY", "SAFE", "RESOURCE"); err != nil {
			return err
		}
		for _, f := range report.Findings {
			safe := "no"
			if f.SafeToDelete {
				safe = "yes"
			}
			if _, err := fmt.Fprintf(w, "%-36s %-16s %10s  %-5s  %s\n", f.RuleID, f.Region, fmt.Sprintf("$%.2f", f.EstimatedMonthlyUSD), safe, f.ResourceID); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "    %s\n", f.Evidence); err != nil {
				return err
			}
			if f.CreatedBy != "" {
				if _, err := fmt.Fprintf(w, "    created by: %s\n", f.CreatedBy); err != nil {
					return err
				}
			}
			if f.SafeToDeleteHint != "" {
				if _, err := fmt.Fprintf(w, "    hint: %s\n", f.SafeToDeleteHint); err != nil {
					return err
				}
			}
		}
	}
	if _, err := fmt.Fprintf(w, "\nTotal estimated monthly (each resource counted once): $%.2f\n", report.TotalEstimatedMonthlyUSD); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "safe_to_delete means nothing is attached. It does not mean deletion is free of data loss. This command does not delete anything."); err != nil {
		return err
	}
	for _, e := range report.Errors {
		if _, err := fmt.Fprintf(w, "error: %s\n", e); err != nil {
			return err
		}
	}
	return nil
}

func writeCSV(w io.Writer, report Report) error {
	cw := csv.NewWriter(w)
	header := []string{
		"resource_id", "name", "provider", "region", "rule_id", "title", "evidence",
		"estimated_monthly_usd", "currency", "created_by", "safe_to_delete", "safe_to_delete_hint", "price_source",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, f := range report.Findings {
		if err := cw.Write([]string{
			f.ResourceID,
			f.Name,
			f.Provider,
			f.Region,
			f.RuleID,
			f.Title,
			f.Evidence,
			strconv.FormatFloat(f.EstimatedMonthlyUSD, 'f', 2, 64),
			f.Currency,
			f.CreatedBy,
			strconv.FormatBool(f.SafeToDelete),
			f.SafeToDeleteHint,
			f.PriceSource,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
