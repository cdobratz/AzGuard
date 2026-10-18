package findings

import (
	"fmt"
	"strings"

	"github.com/azguard/azguard/internal/pricing"
)

// EvalAzureDisks flags managed disks whose diskState is Unattached.
func EvalAzureDisks(rows []map[string]any, book *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		if !strings.EqualFold(rowString(row, "diskState"), "Unattached") {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		sku := rowString(row, "skuName", "sku")
		size := rowFloat(row, "sizeGB")
		region := rowString(row, "location", "region")
		key := pricing.AzureDiskKey(sku, size)
		usd, src := quote(book, key, region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.disk.unattached",
			Evidence:            fmt.Sprintf("Unattached managed disk, %.0f GB, sku %s.", size, emptyAs(sku, "unknown")),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        true,
			SafeToDeleteHint:    "No VM is using this disk. Snapshot it before deleting if you might need the data.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAzurePublicIPs flags public IP addresses with no ipConfiguration.
func EvalAzurePublicIPs(rows []map[string]any, book *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		if rowString(row, "ipConfiguration") != "" || rowBool(row, "associated") {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		sku := rowString(row, "skuName", "sku")
		region := rowString(row, "location", "region")
		key := pricing.KeyAzurePublicIPStandard
		if strings.EqualFold(sku, "Basic") {
			key = pricing.KeyAzurePublicIPBasic
		}
		usd, src := quote(book, key, region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.publicip.unassociated",
			Evidence:            fmt.Sprintf("Public IP is not associated with a resource (sku %s).", emptyAs(sku, "unknown")),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        true,
			SafeToDeleteHint:    "The address is not attached. Deleting it releases the IP. Standard IPv4 addresses are billed whether or not they are associated; this one is not.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAzureNICs flags NICs with no VM and no private endpoint.
func EvalAzureNICs(rows []map[string]any, _ *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		if rowString(row, "virtualMachine", "vmId") != "" || rowBool(row, "hasPrivateEndpoint") {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		region := rowString(row, "location", "region")
		out = append(out, makeFinding(Finding{
			ResourceID:       id,
			Name:             rowString(row, "name"),
			Provider:         "azure",
			Region:           region,
			RuleID:           "azure.nic.unattached",
			Evidence:         "Network interface is not attached to a VM or private endpoint. The NIC itself is not billed; a public IP on it is reported separately.",
			CreatedBy:        rowString(row, "createdBy"),
			SafeToDelete:     true,
			SafeToDeleteHint: "No VM or private endpoint is using this NIC. Deleting it does not remove a running service.",
			PriceSource:      pricing.SourceUnpriced,
		}))
	}
	return out
}

// EvalAzureStoppedVMs flags VMs in the Stopped (allocated) power state.
// Deallocated VMs are not flagged: compute is not billed in that state.
func EvalAzureStoppedVMs(rows []map[string]any, book *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		state := rowString(row, "powerState")
		if !stoppedAllocated(state) {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		size := rowString(row, "vmSize", "skuName")
		region := rowString(row, "location", "region")
		key := pricing.AzureVMKey(size)
		usd, src := quote(book, key, region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.vm.stopped_allocated",
			Evidence:            fmt.Sprintf("Power state is Stopped (allocated) (%s), size %s. Compute is still billed until the VM is deallocated. Estimate is compute only.", state, emptyAs(size, "unknown")),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        false,
			SafeToDeleteHint:    "Deallocate the VM to stop compute charges. Do not delete it from this hint. Disks and public IPs can keep billing after deallocation and are separate findings.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAzurePlans flags App Service plans with zero sites.
func EvalAzurePlans(rows []map[string]any, book *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		if _, ok := row["numberOfSites"]; !ok {
			continue
		}
		if rowFloat(row, "numberOfSites") != 0 {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		sku := rowString(row, "skuName", "sku")
		region := rowString(row, "location", "region")
		key := pricing.AzurePlanKey(sku)
		usd, src := quote(book, key, region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.appserviceplan.empty",
			Evidence:            fmt.Sprintf("App Service plan sku %s has no apps.", emptyAs(sku, "unknown")),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        true,
			SafeToDeleteHint:    "The plan has no sites, so deleting it does not delete app content. Free and Shared SKUs have no compute charge.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAzureSnapshots flags snapshots older than Options.SnapshotAge (default 30 days).
func EvalAzureSnapshots(rows []map[string]any, book *pricing.Book, opts Options) []Finding {
	cutoff := opts.now().Add(-opts.snapshotAge())
	var out []Finding
	for _, row := range rows {
		created := rowTime(row, "timeCreated", "createdAt")
		if created.IsZero() || !created.Before(cutoff) {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		size := rowFloat(row, "sizeGB")
		region := rowString(row, "location", "region")
		usd, src := quote(book, pricing.KeyAzureSnapshotGB, region, size)
		ageDays := int(opts.now().Sub(created).Hours() / 24)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.snapshot.old",
			Evidence:            fmt.Sprintf("Snapshot is %d days old (threshold %d days), %.0f GB. Estimate uses the full size, which can be higher than incremental storage.", ageDays, int(opts.snapshotAge().Hours()/24), size),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        false,
			SafeToDeleteHint:    "Deleting a snapshot is permanent. Confirm nothing still restores from it.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAzureSnapshotGB,
			PriceQuantity:       size,
		}))
	}
	return out
}

// EvalAzureNATGateways flags NAT gateways with no subnets.
func EvalAzureNATGateways(rows []map[string]any, book *pricing.Book) []Finding {
	var out []Finding
	for _, row := range rows {
		if _, ok := row["subnetCount"]; !ok {
			continue
		}
		if rowFloat(row, "subnetCount") != 0 {
			continue
		}
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		region := rowString(row, "location", "region")
		usd, src := quote(book, pricing.KeyAzureNATGateway, region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.natgateway.no_subnets",
			Evidence:            "NAT gateway has no subnets, so it is not serving traffic. Estimate is the hourly gateway charge only; data processing is extra and is not included.",
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        true,
			SafeToDeleteHint:    "No subnet is associated. Deleting it does not disconnect a subnet.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAzureNATGateway,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAzureWorkspaces flags workspaces with no linked solutions and non-zero ingestion.
// solutionRows come from the solutions Resource Graph query. ingestion maps a workspace
// resource id to bytes reported by the workspace usages API. Missing usage is not a finding.
func EvalAzureWorkspaces(rows, solutionRows []map[string]any, ingestion map[string]float64, book *pricing.Book, opts Options) []Finding {
	counts := map[string]int{}
	for _, row := range solutionRows {
		id := strings.ToLower(rowString(row, "workspaceId"))
		if id != "" {
			counts[id]++
		}
	}
	var out []Finding
	for _, row := range rows {
		id := rowString(row, "id")
		if id == "" {
			continue
		}
		if counts[strings.ToLower(id)] > 0 || rowFloat(row, "solutionCount") > 0 {
			continue
		}
		bytes, known := lookupIngestion(ingestion, id)
		if !known || bytes < opts.minIngestion() {
			continue
		}
		region := rowString(row, "location", "region")
		gb := bytes / (1024 * 1024 * 1024)
		usd, src := quote(book, pricing.KeyAzureLogAnalyticsGB, region, gb)
		retention := rowFloat(row, "retentionDays")
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                rowString(row, "name"),
			Provider:            "azure",
			Region:              region,
			RuleID:              "azure.loganalytics.empty_ingestion",
			Evidence:            fmt.Sprintf("Workspace has no solutions and ingested %.2f GB (retention %.0f days). Estimate is ingestion only.", gb, retention),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           rowString(row, "createdBy"),
			SafeToDelete:        false,
			SafeToDeleteHint:    "The workspace is still ingesting. Deleting it removes retained logs. Stop the sources first if you do not need them.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAzureLogAnalyticsGB,
			PriceQuantity:       gb,
		}))
	}
	return out
}

func lookupIngestion(ingestion map[string]float64, id string) (float64, bool) {
	if ingestion == nil {
		return 0, false
	}
	if v, ok := ingestion[id]; ok {
		return v, true
	}
	if v, ok := ingestion[strings.ToLower(id)]; ok {
		return v, true
	}
	return 0, false
}

func emptyAs(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
