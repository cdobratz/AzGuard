package aws

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/azguard/azguard/internal/inventory"
)

var xmlnsRe = regexp.MustCompile(`\sxmlns(:[A-Za-z0-9_]+)?="[^"]*"`)

func unmarshalXML(data []byte, dest any) error {
	cleaned := xmlnsRe.ReplaceAll(data, nil)
	dec := xml.NewDecoder(bytes.NewReader(cleaned))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	return dec.Decode(dest)
}

type xmlTags struct {
	Items []struct {
		Key   string `xml:"key"`
		Value string `xml:"value"`
	} `xml:"item"`
}

func (t xmlTags) get(keys ...string) string {
	for _, item := range t.Items {
		for _, k := range keys {
			if strings.EqualFold(item.Key, k) {
				return item.Value
			}
		}
	}
	return ""
}

func (t xmlTags) createdBy() string {
	return t.get("CreatedBy", "created-by", "aws:createdBy", "Owner")
}

func (t xmlTags) name() string {
	return t.get("Name")
}

// RegionFromAZ strips a trailing availability-zone letter (us-east-1a → us-east-1).
func RegionFromAZ(az string) string {
	if len(az) < 2 {
		return az
	}
	last := az[len(az)-1]
	prev := az[len(az)-2]
	if last >= 'a' && last <= 'z' && prev >= '0' && prev <= '9' {
		return az[:len(az)-1]
	}
	return az
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

type pageToken struct {
	NextToken string `xml:"nextToken"`
}

// ParseVolumes reads a DescribeVolumes response.
func ParseVolumes(body []byte, region string) ([]inventory.Volume, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				VolumeID   string  `xml:"volumeId"`
				Size       float64 `xml:"size"`
				Status     string  `xml:"status"`
				AZ         string  `xml:"availabilityZone"`
				VolumeType string  `xml:"volumeType"`
				Attach     struct {
					Items []struct {
						VolumeID string `xml:"volumeId"`
					} `xml:"item"`
				} `xml:"attachmentSet"`
				Tags xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"volumeSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeVolumes: %w", err)
	}
	out := make([]inventory.Volume, 0, len(resp.Set.Items))
	for _, item := range resp.Set.Items {
		reg := region
		if item.AZ != "" {
			reg = RegionFromAZ(item.AZ)
		}
		out = append(out, inventory.Volume{
			ID:              item.VolumeID,
			Name:            item.Tags.name(),
			Region:          reg,
			VolumeType:      item.VolumeType,
			State:           item.Status,
			CreatedBy:       item.Tags.createdBy(),
			SizeGB:          item.Size,
			AttachmentCount: len(item.Attach.Items),
		})
	}
	return out, resp.NextToken, nil
}

// ParseAddresses reads a DescribeAddresses response.
func ParseAddresses(body []byte, region string) ([]inventory.Address, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				PublicIP     string  `xml:"publicIp"`
				AllocationID string  `xml:"allocationId"`
				Association  string  `xml:"associationId"`
				InstanceID   string  `xml:"instanceId"`
				ENI          string  `xml:"networkInterfaceId"`
				Tags         xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"addressesSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeAddresses: %w", err)
	}
	out := make([]inventory.Address, 0, len(resp.Set.Items))
	for _, item := range resp.Set.Items {
		id := item.AllocationID
		if id == "" {
			id = item.PublicIP
		}
		out = append(out, inventory.Address{
			AllocationID: id,
			PublicIP:     item.PublicIP,
			Region:       region,
			CreatedBy:    item.Tags.createdBy(),
			Associated:   item.Association != "" || item.InstanceID != "" || item.ENI != "",
		})
	}
	return out, resp.NextToken, nil
}

// ParseENIPublicIPs reads public IPv4 addresses from DescribeNetworkInterfaces.
func ParseENIPublicIPs(body []byte, region string) ([]inventory.ENIPublic, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				ENIID string `xml:"networkInterfaceId"`
				AZ    string `xml:"availabilityZone"`
				Assoc struct {
					PublicIP     string `xml:"publicIp"`
					AllocationID string `xml:"allocationId"`
				} `xml:"association"`
				Tags xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"networkInterfaceSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeNetworkInterfaces: %w", err)
	}
	var out []inventory.ENIPublic
	for _, item := range resp.Set.Items {
		if item.Assoc.PublicIP == "" {
			continue
		}
		reg := region
		if item.AZ != "" {
			reg = RegionFromAZ(item.AZ)
		}
		out = append(out, inventory.ENIPublic{
			ENIID:        item.ENIID,
			AllocationID: item.Assoc.AllocationID,
			PublicIP:     item.Assoc.PublicIP,
			Region:       reg,
			CreatedBy:    item.Tags.createdBy(),
		})
	}
	return out, resp.NextToken, nil
}

// ParseNATGateways reads a DescribeNatGateways response. BytesOut is filled by the caller.
func ParseNATGateways(body []byte, region string) ([]inventory.NATGateway, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				ID         string  `xml:"natGatewayId"`
				State      string  `xml:"state"`
				CreateTime string  `xml:"createTime"`
				Tags       xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"natGatewaySet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeNatGateways: %w", err)
	}
	out := make([]inventory.NATGateway, 0, len(resp.Set.Items))
	for _, item := range resp.Set.Items {
		out = append(out, inventory.NATGateway{
			ID:        item.ID,
			Region:    region,
			State:     item.State,
			CreatedBy: item.Tags.createdBy(),
			CreatedAt: parseTime(item.CreateTime),
		})
	}
	return out, resp.NextToken, nil
}

// ParseMetricSum reads a GetMetricStatistics response and sums the Sum of each datapoint.
func ParseMetricSum(body []byte) (float64, error) {
	var resp struct {
		Result struct {
			Datapoints struct {
				Members []struct {
					Sum float64 `xml:"Sum"`
				} `xml:"member"`
			} `xml:"Datapoints"`
		} `xml:"GetMetricStatisticsResult"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return 0, fmt.Errorf("parse GetMetricStatistics: %w", err)
	}
	var sum float64
	for _, m := range resp.Result.Datapoints.Members {
		sum += m.Sum
	}
	return sum, nil
}

// ParseVPCEndpoints reads a DescribeVpcEndpoints response.
func ParseVPCEndpoints(body []byte, region string) ([]inventory.VPCEndpoint, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				ID      string `xml:"vpcEndpointId"`
				Type    string `xml:"vpcEndpointType"`
				Svc     string `xml:"serviceName"`
				State   string `xml:"state"`
				Subnets struct {
					Items []string `xml:"item"`
				} `xml:"subnetIds"`
				Tags xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"vpcEndpointSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeVpcEndpoints: %w", err)
	}
	out := make([]inventory.VPCEndpoint, 0, len(resp.Set.Items))
	for _, item := range resp.Set.Items {
		out = append(out, inventory.VPCEndpoint{
			ID:           item.ID,
			Region:       region,
			CreatedBy:    item.Tags.createdBy(),
			EndpointType: item.Type,
			ServiceName:  item.Svc,
			State:        item.State,
			AZCount:      len(item.Subnets.Items),
		})
	}
	return out, resp.NextToken, nil
}

// ParseInstances reads a DescribeInstances response. Volume sizes are filled later.
func ParseInstances(body []byte, region string) ([]inventory.Instance, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				Instances struct {
					Items []struct {
						ID    string `xml:"instanceId"`
						Type  string `xml:"instanceType"`
						State struct {
							Name string `xml:"name"`
						} `xml:"instanceState"`
						Placement struct {
							AZ string `xml:"availabilityZone"`
						} `xml:"placement"`
						Block struct {
							Items []struct {
								EBS struct {
									VolumeID string `xml:"volumeId"`
									Status   string `xml:"status"`
								} `xml:"ebs"`
							} `xml:"item"`
						} `xml:"blockDeviceMapping"`
						Tags xmlTags `xml:"tagSet"`
					} `xml:"item"`
				} `xml:"instancesSet"`
			} `xml:"item"`
		} `xml:"reservationSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeInstances: %w", err)
	}
	var out []inventory.Instance
	for _, res := range resp.Set.Items {
		for _, item := range res.Instances.Items {
			reg := region
			if item.Placement.AZ != "" {
				reg = RegionFromAZ(item.Placement.AZ)
			}
			inst := inventory.Instance{
				ID:           item.ID,
				Name:         item.Tags.name(),
				Region:       reg,
				InstanceType: item.Type,
				State:        item.State.Name,
				CreatedBy:    item.Tags.createdBy(),
			}
			for _, b := range item.Block.Items {
				if b.EBS.VolumeID == "" {
					continue
				}
				inst.Volumes = append(inst.Volumes, inventory.AttachedVolume{ID: b.EBS.VolumeID})
			}
			out = append(out, inst)
		}
	}
	return out, resp.NextToken, nil
}

// ParseLoadBalancers reads an ELBv2 DescribeLoadBalancers response.
func ParseLoadBalancers(body []byte) ([]inventory.LoadBalancer, string, error) {
	var resp struct {
		Result struct {
			NextToken string `xml:"NextMarker"`
			Set       struct {
				Members []struct {
					ARN  string `xml:"LoadBalancerArn"`
					Name string `xml:"LoadBalancerName"`
					Type string `xml:"Type"`
				} `xml:"member"`
			} `xml:"LoadBalancers"`
		} `xml:"DescribeLoadBalancersResult"`
		NextMarker string `xml:"NextMarker"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeLoadBalancers: %w", err)
	}
	out := make([]inventory.LoadBalancer, 0, len(resp.Result.Set.Members))
	for _, item := range resp.Result.Set.Members {
		out = append(out, inventory.LoadBalancer{
			ARN:    item.ARN,
			Name:   item.Name,
			Region: regionFromARN(item.ARN),
			Type:   item.Type,
		})
	}
	next := resp.Result.NextToken
	if next == "" {
		next = resp.NextMarker
	}
	return out, next, nil
}

// TargetGroup is a target group and the load balancers it is registered with.
type TargetGroup struct {
	ARN              string
	LoadBalancerARNs []string
}

// ParseTargetGroups reads an ELBv2 DescribeTargetGroups response.
func ParseTargetGroups(body []byte) ([]TargetGroup, string, error) {
	var resp struct {
		Result struct {
			NextMarker string `xml:"NextMarker"`
			Set        struct {
				Members []struct {
					ARN string `xml:"TargetGroupArn"`
					LBs struct {
						Members []string `xml:"member"`
					} `xml:"LoadBalancerArns"`
				} `xml:"member"`
			} `xml:"TargetGroups"`
		} `xml:"DescribeTargetGroupsResult"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeTargetGroups: %w", err)
	}
	out := make([]TargetGroup, 0, len(resp.Result.Set.Members))
	for _, item := range resp.Result.Set.Members {
		out = append(out, TargetGroup{ARN: item.ARN, LoadBalancerARNs: item.LBs.Members})
	}
	return out, resp.Result.NextMarker, nil
}

// ParseTargetHealth reads a DescribeTargetHealth response and returns healthy and total counts.
func ParseTargetHealth(body []byte) (healthy, total int, err error) {
	var resp struct {
		Result struct {
			Set struct {
				Members []struct {
					Health struct {
						State string `xml:"State"`
					} `xml:"TargetHealth"`
				} `xml:"member"`
			} `xml:"TargetHealthDescriptions"`
		} `xml:"DescribeTargetHealthResult"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return 0, 0, fmt.Errorf("parse DescribeTargetHealth: %w", err)
	}
	for _, m := range resp.Result.Set.Members {
		total++
		if strings.EqualFold(m.Health.State, "healthy") {
			healthy++
		}
	}
	return healthy, total, nil
}

// ApplyTargetHealth adds target health onto load balancers.
// health maps a target group ARN to healthy and total counts.
func ApplyTargetHealth(lbs []inventory.LoadBalancer, groups []TargetGroup, health map[string][2]int) []inventory.LoadBalancer {
	type counts struct{ healthy, total int }
	byLB := map[string]*counts{}
	for _, g := range groups {
		h := health[g.ARN]
		for _, arn := range g.LoadBalancerARNs {
			c := byLB[arn]
			if c == nil {
				c = &counts{}
				byLB[arn] = c
			}
			c.healthy += h[0]
			c.total += h[1]
		}
	}
	out := make([]inventory.LoadBalancer, len(lbs))
	copy(out, lbs)
	for i := range out {
		if c := byLB[out[i].ARN]; c != nil {
			out[i].Healthy = c.healthy
			out[i].Total = c.total
		}
	}
	return out
}

// ParseSnapshots reads a DescribeSnapshots response.
func ParseSnapshots(body []byte, region string) ([]inventory.Snapshot, string, error) {
	var resp struct {
		pageToken
		Set struct {
			Items []struct {
				ID        string  `xml:"snapshotId"`
				Size      float64 `xml:"volumeSize"`
				StartTime string  `xml:"startTime"`
				State     string  `xml:"status"`
				Tags      xmlTags `xml:"tagSet"`
			} `xml:"item"`
		} `xml:"snapshotSet"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse DescribeSnapshots: %w", err)
	}
	out := make([]inventory.Snapshot, 0, len(resp.Set.Items))
	for _, item := range resp.Set.Items {
		if item.State != "" && !strings.EqualFold(item.State, "completed") {
			continue
		}
		out = append(out, inventory.Snapshot{
			ID:        item.ID,
			Name:      item.Tags.name(),
			Region:    region,
			CreatedBy: item.Tags.createdBy(),
			SizeGB:    item.Size,
			CreatedAt: parseTime(item.StartTime),
		})
	}
	return out, resp.NextToken, nil
}

// ParseRegions reads a DescribeRegions response and returns opted-in region names.
func ParseRegions(body []byte) ([]string, error) {
	var resp struct {
		Set struct {
			Items []struct {
				Name   string `xml:"regionName"`
				Status string `xml:"optInStatus"`
			} `xml:"item"`
		} `xml:"regionInfo"`
	}
	if err := unmarshalXML(body, &resp); err != nil {
		return nil, fmt.Errorf("parse DescribeRegions: %w", err)
	}
	var out []string
	for _, item := range resp.Set.Items {
		if item.Status == "not-opted-in" {
			continue
		}
		out = append(out, item.Name)
	}
	return out, nil
}

func regionFromARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}
