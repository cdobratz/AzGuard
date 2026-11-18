package aws

import (
	"strings"
	"testing"
	"time"
)

func TestParseEC2Fixtures(t *testing.T) {
	volumes, next, err := ParseVolumes([]byte(volumeXML), "us-west-2")
	if err != nil {
		t.Fatal(err)
	}
	if next != "page-2" || len(volumes) != 1 {
		t.Fatalf("volumes %+v next %s", volumes, next)
	}
	if volumes[0].ID != "vol-1" || volumes[0].Region != "us-east-1" || volumes[0].Name != "data" || volumes[0].CreatedBy != "agent" || volumes[0].SizeGB != 8 || volumes[0].AttachmentCount != 1 {
		t.Fatalf("volume %+v", volumes[0])
	}
	if volumes[0].CreatedAt.Format(time.RFC3339) != "2026-01-02T03:04:05Z" {
		t.Fatalf("created %s", volumes[0].CreatedAt)
	}

	addrs, _, err := ParseAddresses([]byte(addressXML), "us-east-1")
	if err != nil || len(addrs) != 2 {
		t.Fatalf("addrs %+v err=%v", addrs, err)
	}
	if addrs[0].AllocationID != "eipalloc-1" || !addrs[0].Associated || addrs[1].AllocationID != "203.0.113.5" || addrs[1].Associated {
		t.Fatalf("addrs %+v", addrs)
	}

	enis, _, err := ParseENIPublicIPs([]byte(eniXML), "us-east-1")
	if err != nil || len(enis) != 1 || enis[0].PublicIP != "203.0.113.9" || enis[0].Region != "us-east-1" {
		t.Fatalf("enis %+v err=%v", enis, err)
	}

	nats, _, err := ParseNATGateways([]byte(natXML), "us-east-1")
	if err != nil || len(nats) != 1 || nats[0].ID != "nat-1" || nats[0].State != "available" {
		t.Fatalf("nats %+v err=%v", nats, err)
	}
	sum, err := ParseMetricSum([]byte(metricXML))
	if err != nil || sum != 15 {
		t.Fatalf("sum %v err=%v", sum, err)
	}

	eps, _, err := ParseVPCEndpoints([]byte(endpointXML), "us-east-1")
	if err != nil || len(eps) != 1 || eps[0].EndpointType != "Interface" || eps[0].AZCount != 2 {
		t.Fatalf("endpoints %+v err=%v", eps, err)
	}

	instances, _, err := ParseInstances([]byte(instanceXML), "us-west-2")
	if err != nil || len(instances) != 1 || instances[0].State != "stopped" || len(instances[0].Volumes) != 1 || instances[0].Volumes[0].ID != "vol-9" {
		t.Fatalf("instances %+v err=%v", instances, err)
	}
	if instances[0].Region != "us-east-1" || instances[0].Name != "web" || instances[0].CreatedBy != "agent" {
		t.Fatalf("instance meta %+v", instances[0])
	}

	snaps, _, err := ParseSnapshots([]byte(snapshotXML), "us-east-1")
	if err != nil || len(snaps) != 1 || snaps[0].ID != "snap-done" || snaps[0].SizeGB != 20 {
		t.Fatalf("snaps %+v err=%v", snaps, err)
	}

	regions, err := ParseRegions([]byte(regionXML))
	if err != nil || len(regions) != 2 || regions[0] != "us-east-1" || regions[1] != "eu-west-1" {
		t.Fatalf("regions %v err=%v", regions, err)
	}
}

func TestParseLoadBalancerFixtures(t *testing.T) {
	lbs, next, err := ParseLoadBalancers([]byte(elbXML))
	if err != nil || next != "m2" || len(lbs) != 1 || lbs[0].Name != "web" || lbs[0].Region != "us-east-1" {
		t.Fatalf("lbs %+v next %s err=%v", lbs, next, err)
	}
	groups, marker, err := ParseTargetGroups([]byte(targetGroupXML))
	if err != nil || marker != "g2" || len(groups) != 1 || groups[0].LoadBalancerARNs[0] != lbs[0].ARN {
		t.Fatalf("groups %+v marker %s err=%v", groups, marker, err)
	}
	healthy, total, err := ParseTargetHealth([]byte(healthXML))
	if err != nil || healthy != 1 || total != 2 {
		t.Fatalf("health %d/%d err=%v", healthy, total, err)
	}
	merged := ApplyTargetHealth(lbs, groups, map[string][2]int{groups[0].ARN: {healthy, total}})
	if merged[0].Healthy != 1 || merged[0].Total != 2 {
		t.Fatalf("merged %+v", merged[0])
	}
	both := append(groups, TargetGroup{ARN: "tg-2", LoadBalancerARNs: []string{lbs[0].ARN}})
	again := ApplyTargetHealth(lbs, both, map[string][2]int{groups[0].ARN: {healthy, total}, "tg-2": {1, 1}})
	if again[0].Healthy != 2 || again[0].Total != 3 {
		t.Fatalf("combined %+v", again[0])
	}

	tags, err := ParseELBTags([]byte(elbTagXML))
	if err != nil || tags[lbs[0].ARN]["Name"] != "web" || len(tags[lbs[0].ARN]) != 1 {
		t.Fatalf("tags %+v err=%v", tags, err)
	}
}

func TestParseHelpersAndErrors(t *testing.T) {
	if RegionFromAZ("us-east-1a") != "us-east-1" || RegionFromAZ("us-east-1") != "us-east-1" || RegionFromAZ("ab") != "ab" || RegionFromAZ("a") != "a" {
		t.Fatal("region from az")
	}
	if !parseTime("2026-01-02T03:04:05Z").Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatal("rfc3339")
	}
	if !parseTime("").IsZero() || !parseTime("not-a-time").IsZero() {
		t.Fatal("bad time")
	}
	if regionFromARN("arn:aws:elasticloadbalancing:eu-west-1:1:loadbalancer/app/web/x") != "eu-west-1" {
		t.Fatal("arn region")
	}
	if regionFromARN("short") != "" {
		t.Fatal("short arn")
	}

	parsers := []struct {
		name string
		fn   func([]byte) error
	}{
		{"volumes", func(b []byte) error { _, _, err := ParseVolumes(b, "us-east-1"); return err }},
		{"addresses", func(b []byte) error { _, _, err := ParseAddresses(b, "us-east-1"); return err }},
		{"eni", func(b []byte) error { _, _, err := ParseENIPublicIPs(b, "us-east-1"); return err }},
		{"nat", func(b []byte) error { _, _, err := ParseNATGateways(b, "us-east-1"); return err }},
		{"metric", func(b []byte) error { _, err := ParseMetricSum(b); return err }},
		{"endpoint", func(b []byte) error { _, _, err := ParseVPCEndpoints(b, "us-east-1"); return err }},
		{"instances", func(b []byte) error { _, _, err := ParseInstances(b, "us-east-1"); return err }},
		{"elb", func(b []byte) error { _, _, err := ParseLoadBalancers(b); return err }},
		{"groups", func(b []byte) error { _, _, err := ParseTargetGroups(b); return err }},
		{"health", func(b []byte) error { _, _, err := ParseTargetHealth(b); return err }},
		{"snaps", func(b []byte) error { _, _, err := ParseSnapshots(b, "us-east-1"); return err }},
		{"regions", func(b []byte) error { _, err := ParseRegions(b); return err }},
		{"tags", func(b []byte) error { _, err := ParseELBTags(b); return err }},
	}
	for _, p := range parsers {
		if err := p.fn([]byte("<not-xml")); err == nil {
			t.Fatalf("%s accepted broken xml", p.name)
		} else if !strings.Contains(err.Error(), "parse") {
			t.Fatalf("%s error %v", p.name, err)
		}
	}
}

const volumeXML = `
<DescribeVolumesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <volumeSet>
    <item>
      <volumeId>vol-1</volumeId>
      <size>8</size>
      <status>available</status>
      <availabilityZone>us-east-1a</availabilityZone>
      <volumeType>gp3</volumeType>
      <createTime>2026-01-02T03:04:05.000Z</createTime>
      <attachmentSet><item><volumeId>vol-1</volumeId></item></attachmentSet>
      <tagSet>
        <item><key>Name</key><value>data</value></item>
        <item><key>CreatedBy</key><value>agent</value></item>
        <item><key></key><value>skip</value></item>
      </tagSet>
    </item>
  </volumeSet>
  <nextToken>page-2</nextToken>
</DescribeVolumesResponse>`

const addressXML = `
<DescribeAddressesResponse>
  <addressesSet>
    <item>
      <publicIp>203.0.113.4</publicIp>
      <allocationId>eipalloc-1</allocationId>
      <associationId>eipassoc-1</associationId>
      <tagSet><item><key>Owner</key><value>agent</value></item></tagSet>
    </item>
    <item>
      <publicIp>203.0.113.5</publicIp>
    </item>
  </addressesSet>
</DescribeAddressesResponse>`

const eniXML = `
<DescribeNetworkInterfacesResponse>
  <networkInterfaceSet>
    <item>
      <networkInterfaceId>eni-1</networkInterfaceId>
      <availabilityZone>us-east-1b</availabilityZone>
      <association><publicIp>203.0.113.9</publicIp><allocationId>eipalloc-9</allocationId></association>
      <tagSet><item><key>aws:createdBy</key><value>agent</value></item></tagSet>
    </item>
    <item>
      <networkInterfaceId>eni-2</networkInterfaceId>
    </item>
  </networkInterfaceSet>
</DescribeNetworkInterfacesResponse>`

const natXML = `
<DescribeNatGatewaysResponse>
  <natGatewaySet>
    <item>
      <natGatewayId>nat-1</natGatewayId>
      <state>available</state>
      <createTime>2026-01-02T03:04:05Z</createTime>
      <tagSet><item><key>Name</key><value>nat</value></item></tagSet>
    </item>
  </natGatewaySet>
</DescribeNatGatewaysResponse>`

const metricXML = `
<GetMetricStatisticsResponse>
  <GetMetricStatisticsResult>
    <Datapoints>
      <member><Sum>10</Sum></member>
      <member><Sum>5</Sum></member>
    </Datapoints>
  </GetMetricStatisticsResult>
</GetMetricStatisticsResponse>`

const endpointXML = `
<DescribeVpcEndpointsResponse>
  <vpcEndpointSet>
    <item>
      <vpcEndpointId>vpce-1</vpcEndpointId>
      <vpcEndpointType>Interface</vpcEndpointType>
      <serviceName>com.amazonaws.us-east-1.s3</serviceName>
      <state>available</state>
      <creationTimestamp>2026-01-02T03:04:05Z</creationTimestamp>
      <subnetIds><item>subnet-a</item><item>subnet-b</item></subnetIds>
      <tagSet><item><key>created-by</key><value>agent</value></item></tagSet>
    </item>
  </vpcEndpointSet>
</DescribeVpcEndpointsResponse>`

const instanceXML = `
<DescribeInstancesResponse>
  <reservationSet>
    <item>
      <instancesSet>
        <item>
          <instanceId>i-1</instanceId>
          <instanceType>t3.micro</instanceType>
          <instanceState><name>stopped</name></instanceState>
          <launchTime>2026-01-02T03:04:05Z</launchTime>
          <placement><availabilityZone>us-east-1a</availabilityZone></placement>
          <blockDeviceMapping>
            <item><ebs><volumeId>vol-9</volumeId><status>attached</status></ebs></item>
            <item><ebs><status>attaching</status></ebs></item>
          </blockDeviceMapping>
          <tagSet>
            <item><key>Name</key><value>web</value></item>
            <item><key>Owner</key><value>agent</value></item>
          </tagSet>
        </item>
      </instancesSet>
    </item>
  </reservationSet>
</DescribeInstancesResponse>`

const snapshotXML = `
<DescribeSnapshotsResponse>
  <snapshotSet>
    <item>
      <snapshotId>snap-done</snapshotId>
      <volumeSize>20</volumeSize>
      <startTime>2026-01-02T03:04:05Z</startTime>
      <status>completed</status>
      <tagSet><item><key>Name</key><value>old</value></item></tagSet>
    </item>
    <item>
      <snapshotId>snap-pending</snapshotId>
      <volumeSize>4</volumeSize>
      <status>pending</status>
    </item>
  </snapshotSet>
</DescribeSnapshotsResponse>`

const regionXML = `
<DescribeRegionsResponse>
  <regionInfo>
    <item><regionName>us-east-1</regionName><optInStatus>opt-in-not-required</optInStatus></item>
    <item><regionName>af-south-1</regionName><optInStatus>not-opted-in</optInStatus></item>
    <item><regionName>eu-west-1</regionName><optInStatus>opted-in</optInStatus></item>
  </regionInfo>
</DescribeRegionsResponse>`

const elbXML = `
<DescribeLoadBalancersResponse>
  <DescribeLoadBalancersResult>
    <NextMarker>m2</NextMarker>
    <LoadBalancers>
      <member>
        <LoadBalancerArn>arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/web/abc</LoadBalancerArn>
        <LoadBalancerName>web</LoadBalancerName>
        <Type>application</Type>
        <CreatedTime>2026-01-02T03:04:05Z</CreatedTime>
      </member>
    </LoadBalancers>
  </DescribeLoadBalancersResult>
</DescribeLoadBalancersResponse>`

const targetGroupXML = `
<DescribeTargetGroupsResponse>
  <DescribeTargetGroupsResult>
    <NextMarker>g2</NextMarker>
    <TargetGroups>
      <member>
        <TargetGroupArn>arn:aws:elasticloadbalancing:us-east-1:123:targetgroup/web/abc</TargetGroupArn>
        <LoadBalancerArns>
          <member>arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/web/abc</member>
        </LoadBalancerArns>
      </member>
    </TargetGroups>
  </DescribeTargetGroupsResult>
</DescribeTargetGroupsResponse>`

const healthXML = `
<DescribeTargetHealthResponse>
  <DescribeTargetHealthResult>
    <TargetHealthDescriptions>
      <member><TargetHealth><State>healthy</State></TargetHealth></member>
      <member><TargetHealth><State>unhealthy</State></TargetHealth></member>
    </TargetHealthDescriptions>
  </DescribeTargetHealthResult>
</DescribeTargetHealthResponse>`

const elbTagXML = `
<DescribeTagsResponse>
  <DescribeTagsResult>
    <TagDescriptions>
      <member>
        <ResourceArn>arn:aws:elasticloadbalancing:us-east-1:123:loadbalancer/app/web/abc</ResourceArn>
        <Tags>
          <member><Key>Name</Key><Value>web</Value></member>
          <member><Key></Key><Value>skip</Value></member>
        </Tags>
      </member>
      <member><ResourceArn></ResourceArn></member>
    </TagDescriptions>
  </DescribeTagsResult>
</DescribeTagsResponse>`
