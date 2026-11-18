package aws

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func (r *Reader) ec2Action(ctx context.Context, region, action string, params url.Values) error {
	if strings.TrimSpace(region) == "" {
		return fmt.Errorf("missing region for %s", action)
	}
	_, err := r.postForm(ctx, r.ec2Endpoint(region), region, "ec2", "2016-11-15", action, params)
	return alreadyGone(err)
}

func (r *Reader) elbAction(ctx context.Context, region, action string, params url.Values) error {
	if strings.TrimSpace(region) == "" {
		return fmt.Errorf("missing region for %s", action)
	}
	_, err := r.postForm(ctx, r.elbEndpoint(region), region, "elasticloadbalancing", "2015-12-01", action, params)
	return alreadyGone(err)
}

// alreadyGone treats a missing resource as success so a rerun can finish.
func alreadyGone(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "NotFound") {
		return nil
	}
	return err
}

// DeleteVolume deletes an EBS volume.
func (r *Reader) DeleteVolume(ctx context.Context, region, volumeID string) error {
	params := url.Values{}
	params.Set("VolumeId", volumeID)
	return r.ec2Action(ctx, region, "DeleteVolume", params)
}

// ReleaseAddress releases an Elastic IP. id is an allocation id or a public IP.
func (r *Reader) ReleaseAddress(ctx context.Context, region, id string) error {
	params := url.Values{}
	if strings.HasPrefix(id, "eipalloc-") {
		params.Set("AllocationId", id)
	} else {
		params.Set("PublicIp", id)
	}
	return r.ec2Action(ctx, region, "ReleaseAddress", params)
}

// DeleteNATGateway deletes a NAT gateway.
func (r *Reader) DeleteNATGateway(ctx context.Context, region, natID string) error {
	params := url.Values{}
	params.Set("NatGatewayId", natID)
	return r.ec2Action(ctx, region, "DeleteNatGateway", params)
}

// DeleteVPCEndpoint deletes one interface VPC endpoint.
func (r *Reader) DeleteVPCEndpoint(ctx context.Context, region, endpointID string) error {
	params := url.Values{}
	params.Set("VpcEndpointId.1", endpointID)
	return r.ec2Action(ctx, region, "DeleteVpcEndpoints", params)
}

// TerminateInstance terminates one EC2 instance.
func (r *Reader) TerminateInstance(ctx context.Context, region, instanceID string) error {
	params := url.Values{}
	params.Set("InstanceId.1", instanceID)
	return r.ec2Action(ctx, region, "TerminateInstances", params)
}

// DeleteSnapshot deletes an EBS snapshot.
func (r *Reader) DeleteSnapshot(ctx context.Context, region, snapshotID string) error {
	params := url.Values{}
	params.Set("SnapshotId", snapshotID)
	return r.ec2Action(ctx, region, "DeleteSnapshot", params)
}

// DeleteLoadBalancer deletes an ELBv2 load balancer by ARN.
func (r *Reader) DeleteLoadBalancer(ctx context.Context, region, arn string) error {
	params := url.Values{}
	params.Set("LoadBalancerArn", arn)
	return r.elbAction(ctx, region, "DeleteLoadBalancer", params)
}

// LoadBalancerTags reads ELBv2 tags so cleanup can see azguard:keep.
// ARNs absent from a successful response are omitted. The caller must skip those.
func (r *Reader) LoadBalancerTags(ctx context.Context, region string, arns []string) (map[string]map[string]string, error) {
	uniq := uniqueNonEmpty(arns)
	out := map[string]map[string]string{}
	const batch = 20
	for start := 0; start < len(uniq); start += batch {
		end := start + batch
		if end > len(uniq) {
			end = len(uniq)
		}
		params := url.Values{}
		for i, arn := range uniq[start:end] {
			params.Set(fmt.Sprintf("ResourceArns.member.%d", i+1), arn)
		}
		body, err := r.postForm(ctx, r.elbEndpoint(region), region, "elasticloadbalancing", "2015-12-01", "DescribeTags", params)
		if err != nil {
			return nil, err
		}
		page, err := ParseELBTags(body)
		if err != nil {
			return nil, err
		}
		for arn, tags := range page {
			out[arn] = tags
		}
	}
	return out, nil
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
