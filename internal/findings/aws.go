package findings

import (
	"fmt"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/inventory"
	"github.com/cdobratz/azguard/internal/pricing"
)

// EvalAWSEBS flags volumes in the available state with no attachments.
func EvalAWSEBS(volumes []inventory.Volume, book *pricing.Book) []Finding {
	var out []Finding
	for _, v := range volumes {
		if !strings.EqualFold(v.State, "available") || v.AttachmentCount > 0 || v.ID == "" {
			continue
		}
		key := pricing.AWSEBSKey(v.VolumeType)
		usd, src := quote(book, key, v.Region, v.SizeGB)
		out = append(out, makeFinding(Finding{
			ResourceID:          v.ID,
			Name:                v.Name,
			Provider:            "aws",
			Region:              v.Region,
			RuleID:              "aws.ebs.unattached",
			Evidence:            fmt.Sprintf("EBS volume is available with no attachments, %s, %.0f GB.", emptyAs(v.VolumeType, "unknown type"), v.SizeGB),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           v.CreatedBy,
			SafeToDelete:        true,
			SafeToDeleteHint:    "Nothing is attached. Snapshot the volume before deleting it if you might need the data.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       v.SizeGB,
		}))
	}
	return out
}

// EvalAWSEIP flags Elastic IPs that are not associated with a resource.
func EvalAWSEIP(addrs []inventory.Address, book *pricing.Book) []Finding {
	var out []Finding
	for _, a := range addrs {
		if a.Associated || a.AllocationID == "" {
			continue
		}
		usd, src := quote(book, pricing.KeyAWSPublicIPv4, a.Region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          a.AllocationID,
			Name:                a.PublicIP,
			Provider:            "aws",
			Region:              a.Region,
			RuleID:              "aws.eip.unassociated",
			Evidence:            fmt.Sprintf("Elastic IP %s is not associated with an instance or network interface.", a.PublicIP),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           a.CreatedBy,
			SafeToDelete:        true,
			SafeToDeleteHint:    "The address is idle. Releasing it stops the public IPv4 charge.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSPublicIPv4,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAWSPublicIPv4 flags every public IPv4 address: unassociated Elastic IPs,
// associated Elastic IPs, and auto-assigned addresses on network interfaces.
// AWS charges $0.005 per hour whether or not the address is attached.
// An address already reported from an allocation is not repeated for its ENI.
func EvalAWSPublicIPv4(addrs []inventory.Address, enis []inventory.ENIPublic, book *pricing.Book) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, a := range addrs {
		if a.AllocationID == "" && a.PublicIP == "" {
			continue
		}
		id := a.AllocationID
		if id == "" {
			id = a.PublicIP
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		usd, src := quote(book, pricing.KeyAWSPublicIPv4, a.Region, 1)
		hint := "This address is in use. Releasing it changes connectivity for whatever it is attached to. The charge applies whether or not it is attached."
		if !a.Associated {
			hint = "This Elastic IP is not associated. Releasing it stops the charge. The unassociated-EIP rule reports the same address."
		}
		state := "in use"
		if !a.Associated {
			state = "not associated"
		}
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                a.PublicIP,
			Provider:            "aws",
			Region:              a.Region,
			RuleID:              "aws.ipv4.public",
			Evidence:            fmt.Sprintf("Public IPv4 %s is %s. AWS charges $0.005 per hour for public IPv4 addresses whether or not they are attached.", a.PublicIP, state),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           a.CreatedBy,
			SafeToDelete:        !a.Associated,
			SafeToDeleteHint:    hint,
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSPublicIPv4,
			PriceQuantity:       1,
		}))
	}
	for _, e := range enis {
		id := e.AllocationID
		if id == "" {
			id = e.ENIID
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		usd, src := quote(book, pricing.KeyAWSPublicIPv4, e.Region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          id,
			Name:                e.PublicIP,
			Provider:            "aws",
			Region:              e.Region,
			RuleID:              "aws.ipv4.public",
			Evidence:            fmt.Sprintf("Public IPv4 %s is attached to %s. AWS charges $0.005 per hour whether or not the address is attached.", e.PublicIP, e.ENIID),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           e.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "The address is in use. Releasing or replacing it changes connectivity. The charge applies either way.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSPublicIPv4,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAWSNAT flags available NAT gateways that processed 0 bytes over the lookback
// and were created at least 7 days ago. A missing create time is still flagged.
func EvalAWSNAT(nats []inventory.NATGateway, book *pricing.Book, opts Options) []Finding {
	cutoff := opts.now().Add(-7 * 24 * time.Hour)
	var out []Finding
	for _, n := range nats {
		if n.ID == "" || !strings.EqualFold(n.State, "available") {
			continue
		}
		if !n.CreatedAt.IsZero() && n.CreatedAt.After(cutoff) {
			continue
		}
		if n.BytesOut != 0 {
			continue
		}
		usd, src := quote(book, pricing.KeyAWSNATGateway, n.Region, 1)
		when := "create time unknown"
		if !n.CreatedAt.IsZero() {
			when = "created " + n.CreatedAt.UTC().Format("2006-01-02")
		}
		out = append(out, makeFinding(Finding{
			ResourceID:          n.ID,
			Name:                n.ID,
			Provider:            "aws",
			Region:              n.Region,
			RuleID:              "aws.natgateway.idle",
			Evidence:            fmt.Sprintf("NAT gateway processed 0 bytes (BytesOutToDestination) over 7 days, %s. Estimate is the hourly charge only; data processing is extra.", when),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           n.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "No bytes were processed in the window. Confirm no subnet still routes through it before deleting.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSNATGateway,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAWSEndpoints flags Interface VPC endpoints. Gateway endpoints are free and ignored.
func EvalAWSEndpoints(eps []inventory.VPCEndpoint, book *pricing.Book) []Finding {
	var out []Finding
	for _, e := range eps {
		if e.ID == "" || !strings.EqualFold(e.EndpointType, "Interface") {
			continue
		}
		if strings.EqualFold(e.State, "deleted") || strings.EqualFold(e.State, "deleting") {
			continue
		}
		az := e.AZCount
		if az < 1 {
			az = 1
		}
		usd, src := quote(book, pricing.KeyAWSVPCEndpointHour, e.Region, float64(az))
		out = append(out, makeFinding(Finding{
			ResourceID:          e.ID,
			Name:                e.ServiceName,
			Provider:            "aws",
			Region:              e.Region,
			RuleID:              "aws.vpcendpoint.interface",
			Evidence:            fmt.Sprintf("Interface VPC endpoint for %s in %d AZ(s), state %s. Gateway endpoints are free and are not listed. Estimate is the hourly charge per AZ.", emptyAs(e.ServiceName, "unknown service"), az, emptyAs(e.State, "unknown")),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           e.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "Interface endpoints bill per AZ even with little traffic. Confirm no clients use it before deleting.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSVPCEndpointHour,
			PriceQuantity:       float64(az),
		}))
	}
	return out
}

// EvalAWSStopped flags stopped instances that still have EBS volumes attached.
// The estimate is the volumes, not the instance: stopped instances are not billed for compute.
func EvalAWSStopped(instances []inventory.Instance, book *pricing.Book) []Finding {
	var out []Finding
	for _, inst := range instances {
		if !strings.EqualFold(inst.State, "stopped") || len(inst.Volumes) == 0 || inst.ID == "" {
			continue
		}
		var cost float64
		var src string
		var parts []string
		known := true
		for _, vol := range inst.Volumes {
			key := pricing.AWSEBSKey(vol.VolumeType)
			usd, s := quote(book, key, inst.Region, vol.SizeGB)
			if s == pricing.SourceUnpriced {
				known = false
			}
			cost += usd
			if src == "" {
				src = s
			}
			parts = append(parts, fmt.Sprintf("%s %s %.0fGB", vol.ID, emptyAs(vol.VolumeType, "?"), vol.SizeGB))
		}
		if !known {
			src = pricing.SourceUnpriced
		}
		out = append(out, makeFinding(Finding{
			ResourceID:          inst.ID,
			Name:                inst.Name,
			Provider:            "aws",
			Region:              inst.Region,
			RuleID:              "aws.ec2.stopped_with_ebs",
			Evidence:            fmt.Sprintf("Instance is stopped with EBS still attached (%s). Compute is not billed while stopped; the estimate is the volumes.", strings.Join(parts, ", ")),
			EstimatedMonthlyUSD: pricing.RoundUSD(cost),
			CreatedBy:           inst.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "Deleting the instance can delete volumes whose DeleteOnTermination flag is set. Detach or snapshot volumes you need first.",
			PriceSource:         src,
		}))
	}
	return out
}

// EvalAWSELB flags load balancers with no healthy targets, including balancers with no targets at all.
func EvalAWSELB(lbs []inventory.LoadBalancer, book *pricing.Book) []Finding {
	var out []Finding
	for _, lb := range lbs {
		if lb.ARN == "" || lb.Healthy > 0 {
			continue
		}
		key := pricing.KeyAWSALBHour
		kind := lb.Type
		if strings.EqualFold(kind, "network") {
			key = pricing.KeyAWSNLBHour
		}
		if kind == "" {
			kind = "application"
		}
		usd, src := quote(book, key, lb.Region, 1)
		out = append(out, makeFinding(Finding{
			ResourceID:          lb.ARN,
			Name:                lb.Name,
			Provider:            "aws",
			Region:              lb.Region,
			RuleID:              "aws.elb.no_healthy_targets",
			Evidence:            fmt.Sprintf("%s load balancer has %d healthy targets out of %d registered. Estimate is the hourly load balancer charge, not LCUs.", kind, lb.Healthy, lb.Total),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           lb.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "Nothing healthy is registered. Deleting the load balancer releases its DNS name. Confirm nothing still points at it.",
			PriceSource:         src,
			PriceKey:            key,
			PriceQuantity:       1,
		}))
	}
	return out
}

// EvalAWSSnapshots flags snapshots older than Options.SnapshotAge (default 30 days).
func EvalAWSSnapshots(snaps []inventory.Snapshot, book *pricing.Book, opts Options) []Finding {
	cutoff := opts.now().Add(-opts.snapshotAge())
	var out []Finding
	for _, s := range snaps {
		if s.ID == "" || s.CreatedAt.IsZero() || !s.CreatedAt.Before(cutoff) {
			continue
		}
		usd, src := quote(book, pricing.KeyAWSSnapshotGB, s.Region, s.SizeGB)
		ageDays := int(opts.now().Sub(s.CreatedAt).Hours() / 24)
		out = append(out, makeFinding(Finding{
			ResourceID:          s.ID,
			Name:                s.Name,
			Provider:            "aws",
			Region:              s.Region,
			RuleID:              "aws.snapshot.old",
			Evidence:            fmt.Sprintf("Snapshot is %d days old (threshold %d days), %.0f GB source volume. Estimate uses the source volume size as an upper bound; incremental snapshot storage is often lower.", ageDays, int(opts.snapshotAge().Hours()/24), s.SizeGB),
			EstimatedMonthlyUSD: usd,
			CreatedBy:           s.CreatedBy,
			SafeToDelete:        false,
			SafeToDeleteHint:    "Deleting a snapshot is permanent. Confirm no AMI or restore still depends on it.",
			PriceSource:         src,
			PriceKey:            pricing.KeyAWSSnapshotGB,
			PriceQuantity:       s.SizeGB,
		}))
	}
	return out
}
