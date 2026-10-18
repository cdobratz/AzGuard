package findings

import (
	"testing"
	"time"

	awscloud "github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/inventory"
	"github.com/azguard/azguard/internal/pricing"
)

func TestAWSRulesFromFixtures(t *testing.T) {
	book := pricing.Bundled()
	opts := Options{Now: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	region := "us-east-1"
	seen := map[string]bool{}

	volumes := mustVolumes(t, region)
	addresses := mustAddresses(t, region)

	t.Run("unattached ebs", func(t *testing.T) {
		got := EvalAWSEBS(volumes, book)
		f := one(t, got, "vol-free")
		assertFinding(t, f, "aws.ebs.unattached", "us-east-1", 8.00, true, "ci-bot")
		absent(t, got, "vol-attached")
		seen[f.RuleID] = true
	})

	t.Run("unassociated eip", func(t *testing.T) {
		got := EvalAWSEIP(addresses, book)
		f := one(t, got, "eipalloc-idle")
		assertFinding(t, f, "aws.eip.unassociated", "us-east-1", 3.65, true, "ci-bot")
		absent(t, got, "eipalloc-used")
		seen[f.RuleID] = true
	})

	t.Run("public ipv4", func(t *testing.T) {
		enis := mustENIs(t, region)
		got := EvalAWSPublicIPv4(addresses, enis, book)
		idle := one(t, got, "eipalloc-idle")
		assertFinding(t, idle, "aws.ipv4.public", "us-east-1", 3.65, true, "ci-bot")
		used := one(t, got, "eipalloc-used")
		assertFinding(t, used, "aws.ipv4.public", "us-east-1", 3.65, false, "human")
		auto := one(t, got, "eni-auto")
		assertFinding(t, auto, "aws.ipv4.public", "us-east-1", 3.65, false, "terraform")
		if len(got) != 3 {
			t.Fatalf("got %d public ipv4 findings, want 3", len(got))
		}
		eips := EvalAWSEIP(addresses, book)
		total := TotalMonthly(append(got, eips...))
		if total != 10.95 {
			t.Fatalf("deduped total = %.2f, want 10.95 (idle EIP counted once)", total)
		}
		seen[idle.RuleID] = true
	})

	t.Run("idle nat gateway", func(t *testing.T) {
		nats, _, err := awscloud.ParseNATGateways(mustRead(t, "testdata/aws/nat_gateways.xml"), region)
		if err != nil {
			t.Fatal(err)
		}
		idle, err := awscloud.ParseMetricSum(mustRead(t, "testdata/aws/nat_metrics_idle.xml"))
		if err != nil {
			t.Fatal(err)
		}
		busy, err := awscloud.ParseMetricSum(mustRead(t, "testdata/aws/nat_metrics_busy.xml"))
		if err != nil {
			t.Fatal(err)
		}
		if idle != 0 {
			t.Fatalf("idle metric sum = %v, want 0", idle)
		}
		if busy <= 0 {
			t.Fatalf("busy metric sum = %v, want > 0", busy)
		}
		for i := range nats {
			switch nats[i].ID {
			case "nat-busy":
				nats[i].BytesOut = busy
			default:
				nats[i].BytesOut = idle
			}
		}
		got := EvalAWSNAT(nats, book, opts)
		f := one(t, got, "nat-idle")
		assertFinding(t, f, "aws.natgateway.idle", "us-east-1", 32.85, false, "ci-bot")
		absent(t, got, "nat-busy")
		absent(t, got, "nat-new")
		absent(t, got, "nat-deleted")
		seen[f.RuleID] = true
	})

	t.Run("interface vpc endpoint", func(t *testing.T) {
		eps, _, err := awscloud.ParseVPCEndpoints(mustRead(t, "testdata/aws/vpc_endpoints.xml"), region)
		if err != nil {
			t.Fatal(err)
		}
		got := EvalAWSEndpoints(eps, book)
		f := one(t, got, "vpce-interface")
		assertFinding(t, f, "aws.vpcendpoint.interface", "us-east-1", 14.60, false, "terraform")
		absent(t, got, "vpce-gateway")
		absent(t, got, "vpce-deleted")
		seen[f.RuleID] = true
	})

	t.Run("stopped instance with ebs", func(t *testing.T) {
		instances, _, err := awscloud.ParseInstances(mustRead(t, "testdata/aws/instances.xml"), region)
		if err != nil {
			t.Fatal(err)
		}
		instances = inventory.AttachVolumeSizes(instances, volumes)
		got := EvalAWSStopped(instances, book)
		f := one(t, got, "i-stopped")
		assertFinding(t, f, "aws.ec2.stopped_with_ebs", "us-east-1", 2.40, false, "ci-bot")
		absent(t, got, "i-running")
		absent(t, got, "i-stopped-empty")
		seen[f.RuleID] = true
	})

	t.Run("load balancer without healthy targets", func(t *testing.T) {
		lbs, _, err := awscloud.ParseLoadBalancers(mustRead(t, "testdata/aws/load_balancers.xml"))
		if err != nil {
			t.Fatal(err)
		}
		groups, _, err := awscloud.ParseTargetGroups(mustRead(t, "testdata/aws/target_groups.xml"))
		if err != nil {
			t.Fatal(err)
		}
		unhealthy, unTotal, err := awscloud.ParseTargetHealth(mustRead(t, "testdata/aws/target_health_unhealthy.xml"))
		if err != nil {
			t.Fatal(err)
		}
		healthy, hTotal, err := awscloud.ParseTargetHealth(mustRead(t, "testdata/aws/target_health_healthy.xml"))
		if err != nil {
			t.Fatal(err)
		}
		if unhealthy != 0 || unTotal != 2 || healthy != 1 || hTotal != 1 {
			t.Fatalf("health counts unhealthy=%d/%d healthy=%d/%d", unhealthy, unTotal, healthy, hTotal)
		}
		health := map[string][2]int{
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/web/111": {unhealthy, unTotal},
			"arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/api/222": {healthy, hTotal},
		}
		got := EvalAWSELB(awscloud.ApplyTargetHealth(lbs, groups, health), book)
		web := one(t, got, "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/web/abc")
		assertFinding(t, web, "aws.elb.no_healthy_targets", "us-east-1", 16.43, false, "")
		lonely := one(t, got, "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/lonely/ghi")
		assertFinding(t, lonely, "aws.elb.no_healthy_targets", "us-east-1", 16.43, false, "")
		absent(t, got, "loadbalancer/app/api/")
		seen[web.RuleID] = true
	})

	t.Run("old snapshot", func(t *testing.T) {
		snaps, _, err := awscloud.ParseSnapshots(mustRead(t, "testdata/aws/snapshots.xml"), region)
		if err != nil {
			t.Fatal(err)
		}
		got := EvalAWSSnapshots(snaps, book, opts)
		f := one(t, got, "snap-old")
		assertFinding(t, f, "aws.snapshot.old", "us-east-1", 2.50, false, "ci-bot")
		absent(t, got, "snap-recent")
		seen[f.RuleID] = true
	})

	for _, rule := range Catalog() {
		if rule.Provider == "aws" && !seen[rule.ID] {
			t.Errorf("catalog rule %s has no fixture finding", rule.ID)
		}
	}
}

func mustVolumes(t *testing.T, region string) []inventory.Volume {
	t.Helper()
	vols, _, err := awscloud.ParseVolumes(mustRead(t, "testdata/aws/volumes.xml"), region)
	if err != nil {
		t.Fatal(err)
	}
	return vols
}

func mustAddresses(t *testing.T, region string) []inventory.Address {
	t.Helper()
	addrs, _, err := awscloud.ParseAddresses(mustRead(t, "testdata/aws/addresses.xml"), region)
	if err != nil {
		t.Fatal(err)
	}
	return addrs
}

func mustENIs(t *testing.T, region string) []inventory.ENIPublic {
	t.Helper()
	enis, _, err := awscloud.ParseENIPublicIPs(mustRead(t, "testdata/aws/network_interfaces.xml"), region)
	if err != nil {
		t.Fatal(err)
	}
	return enis
}
