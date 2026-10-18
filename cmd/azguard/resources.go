package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	awscloud "github.com/cdobratz/azguard/internal/cloud/aws"
	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/inventory"
	"github.com/cdobratz/azguard/internal/pricing"
	"github.com/spf13/cobra"
)

func resourcesCmd() *cobra.Command {
	var (
		cloud        string
		region       string
		allRegions   bool
		snapshotDays int
		failOver     float64
		showSchema   bool
		showRules    bool
	)
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "Find idle and orphaned Azure and AWS resources",
		Long: `Find idle and orphaned resources that can keep billing after you forget them.

The estimate is a list price (Azure Retail Prices or the AWS Price List when those
APIs respond, otherwise a bundled list-price table, 730 hours per month). It is not
an invoice. safe_to_delete means nothing is attached; it does not mean deletion is
free of data loss. This command never deletes anything.

  azguard resources
  azguard resources --output json
  azguard resources --output csv --fail-over 25
  azguard resources --cloud aws --all-regions
  azguard resources --schema

Exit code 2 means the deduplicated estimated monthly total is greater than --fail-over.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showSchema {
				fmt.Print(findings.JSONSchema)
				return nil
			}
			if showRules {
				fmt.Println("RULE                                  PROVIDER  TITLE")
				for _, rule := range findings.Catalog() {
					fmt.Printf("%-36s  %-8s  %s\n", rule.ID, rule.Provider, rule.Title)
				}
				return nil
			}
			switch cloud {
			case "all", "azure", "aws":
			default:
				return fmt.Errorf("--cloud must be all, azure, or aws")
			}
			ctx := context.Background()
			opts := findings.Options{SnapshotAge: time.Duration(snapshotDays) * 24 * time.Hour}
			book := pricing.Bundled()
			var found []findings.Finding
			var errs []string

			if cloud == "all" || cloud == "azure" {
				azFound, azErrs := scanAzure(ctx, book, opts)
				found = append(found, azFound...)
				errs = append(errs, azErrs...)
			}
			var awsReader *awscloud.Reader
			if cloud == "all" || cloud == "aws" {
				var awsFound []findings.Finding
				var awsErrs []string
				awsFound, awsReader, awsErrs = scanAWS(ctx, region, allRegions, book, opts)
				found = append(found, awsFound...)
				errs = append(errs, awsErrs...)
			}

			if priceErrs := enrichPrices(ctx, found, book, awsReader); len(priceErrs) > 0 {
				errs = append(errs, priceErrs...)
			}
			found = findings.Reprice(found, book)
			report := findings.NewReport(found, errs, time.Now())
			if err := findings.Write(cmd.OutOrStdout(), report, outputFormat); err != nil {
				return err
			}
			if len(report.Findings) == 0 && len(report.Errors) > 0 {
				return fmt.Errorf("scan failed: %s", strings.Join(report.Errors, "; "))
			}
			if findings.Exceeds(report.TotalEstimatedMonthlyUSD, failOver) {
				return &findings.FailOverError{Total: report.TotalEstimatedMonthlyUSD, Limit: failOver}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cloud, "cloud", "all", "Which cloud to scan: all, azure, or aws")
	cmd.Flags().StringVar(&region, "region", "", "AWS region to scan (default: configured region)")
	cmd.Flags().BoolVar(&allRegions, "all-regions", false, "Scan every enabled AWS region (DescribeRegions)")
	cmd.Flags().IntVar(&snapshotDays, "snapshot-days", 30, "Flag snapshots older than this many days")
	cmd.Flags().Float64Var(&failOver, "fail-over", -1, "Exit 2 when estimated monthly USD is greater than this. Negative disables the check")
	cmd.Flags().BoolVar(&showSchema, "schema", false, "Print the JSON schema and exit")
	cmd.Flags().BoolVar(&showRules, "rules", false, "Print the rule catalog and exit")
	return cmd
}

func scanAzure(ctx context.Context, book *pricing.Book, opts findings.Options) ([]findings.Finding, []string) {
	if cfg.Azure.SubscriptionID == "" {
		return nil, []string{"Azure subscription is not configured; skipped Azure rules"}
	}
	token, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
		"tenant_id":     cfg.Azure.TenantID,
		"client_id":     cfg.Azure.ClientID,
		"client_secret": cfg.Azure.ClientSecret,
	})
	if err != nil {
		return nil, []string{"Azure auth: " + err.Error()}
	}
	reader := azure.NewReader(cfg.Azure.SubscriptionID, token)
	var found []findings.Finding
	var errs []string
	run := func(name, kql string, eval func([]map[string]any) []findings.Finding) bool {
		rows, qErr := reader.Query(ctx, kql)
		if qErr != nil {
			errs = append(errs, name+": "+qErr.Error())
			return !azureAuthFailure(qErr)
		}
		found = append(found, eval(rows)...)
		return true
	}
	steps := []struct {
		name string
		kql  string
		eval func([]map[string]any) []findings.Finding
	}{
		{"azure disks", azure.QueryUnattachedDisks, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzureDisks(rows, book)
		}},
		{"azure public ips", azure.QueryUnassociatedPublicIPs, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzurePublicIPs(rows, book)
		}},
		{"azure nics", azure.QueryUnattachedNICs, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzureNICs(rows, book)
		}},
		{"azure vms", azure.QueryStoppedAllocatedVMs, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzureStoppedVMs(rows, book)
		}},
		{"azure app service plans", azure.QueryEmptyAppServicePlans, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzurePlans(rows, book)
		}},
		{"azure snapshots", azure.QuerySnapshots, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzureSnapshots(rows, book, opts)
		}},
		{"azure nat gateways", azure.QueryNATGateways, func(rows []map[string]any) []findings.Finding {
			return findings.EvalAzureNATGateways(rows, book)
		}},
	}
	for _, step := range steps {
		if !run(step.name, step.kql, step.eval) {
			return found, errs
		}
	}

	wsRows, wsErr := reader.Query(ctx, azure.QueryWorkspaces)
	if wsErr != nil {
		errs = append(errs, "azure workspaces: "+wsErr.Error())
		return found, errs
	}
	solRows, solErr := reader.Query(ctx, azure.QueryWorkspaceSolutions)
	if solErr != nil {
		errs = append(errs, "azure workspace solutions: "+solErr.Error())
		return found, errs
	}
	solutions := map[string]bool{}
	for _, row := range solRows {
		if id, ok := row["workspaceId"].(string); ok && id != "" {
			solutions[strings.ToLower(id)] = true
		}
	}
	ingestion := map[string]float64{}
	for _, row := range wsRows {
		id, _ := row["id"].(string)
		if id == "" || solutions[strings.ToLower(id)] {
			continue
		}
		n, uErr := reader.WorkspaceIngestionBytes(ctx, id)
		if uErr != nil {
			errs = append(errs, "azure workspace usage "+id+": "+uErr.Error())
			continue
		}
		ingestion[id] = n
	}
	found = append(found, findings.EvalAzureWorkspaces(wsRows, solRows, ingestion, book, opts)...)
	return found, errs
}

func scanAWS(ctx context.Context, regionFlag string, allRegions bool, book *pricing.Book, opts findings.Options) ([]findings.Finding, *awscloud.Reader, []string) {
	costClient := awscloud.NewCostClient(cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SessionToken, cfg.AWS.Region)
	reader := awscloud.ReaderFromCost(costClient)
	if regionFlag != "" {
		reader.Region = regionFlag
	}
	if !reader.Enabled() {
		return nil, reader, []string{"AWS credentials are not configured; skipped AWS rules"}
	}
	regions := []string{reader.Region}
	var errs []string
	if allRegions {
		listed, err := reader.Regions(ctx)
		if err != nil {
			errs = append(errs, "aws regions: "+err.Error())
		} else if len(listed) > 0 {
			regions = listed
		}
	}
	var found []findings.Finding
	for _, region := range regions {
		f, e := scanAWSRegion(ctx, reader, region, book, opts)
		found = append(found, f...)
		errs = append(errs, e...)
	}
	return found, reader, errs
}

func scanAWSRegion(ctx context.Context, reader *awscloud.Reader, region string, book *pricing.Book, opts findings.Options) ([]findings.Finding, []string) {
	var found []findings.Finding
	var errs []string
	note := func(name string, err error) {
		if err != nil {
			errs = append(errs, fmt.Sprintf("aws %s (%s): %s", name, region, err.Error()))
		}
	}
	volumes, err := reader.Volumes(ctx, region)
	note("volumes", err)
	if err == nil {
		found = append(found, findings.EvalAWSEBS(volumes, book)...)
	}
	addrs, addrErr := reader.Addresses(ctx, region)
	note("addresses", addrErr)
	if addrErr == nil {
		found = append(found, findings.EvalAWSEIP(addrs, book)...)
	}
	enis, eniErr := reader.ENIPublicIPs(ctx, region)
	note("network interfaces", eniErr)
	if addrErr == nil || eniErr == nil {
		if addrErr != nil {
			addrs = nil
		}
		if eniErr != nil {
			enis = nil
		}
		found = append(found, findings.EvalAWSPublicIPv4(addrs, enis, book)...)
	}
	nats, err := reader.NATGateways(ctx, region)
	note("nat gateways", err)
	if err == nil {
		found = append(found, findings.EvalAWSNAT(nats, book, opts)...)
	}
	eps, err := reader.VPCEndpoints(ctx, region)
	note("vpc endpoints", err)
	if err == nil {
		found = append(found, findings.EvalAWSEndpoints(eps, book)...)
	}
	instances, err := reader.Instances(ctx, region)
	note("instances", err)
	if err == nil {
		if volumes != nil {
			instances = inventory.AttachVolumeSizes(instances, volumes)
		}
		found = append(found, findings.EvalAWSStopped(instances, book)...)
	}
	lbs, err := reader.LoadBalancers(ctx, region)
	note("load balancers", err)
	if err == nil {
		found = append(found, findings.EvalAWSELB(lbs, book)...)
	}
	snaps, err := reader.Snapshots(ctx, region)
	note("snapshots", err)
	if err == nil {
		found = append(found, findings.EvalAWSSnapshots(snaps, book, opts)...)
	}
	return found, errs
}

func azureAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 401") || strings.Contains(msg, "status 403") || strings.Contains(msg, "azure token")
}
