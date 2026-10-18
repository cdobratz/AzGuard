package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/inventory"
	"github.com/cdobratz/azguard/internal/pricing"
)

// Reader calls read-only EC2, ELB, CloudWatch, and Price List APIs.
type Reader struct {
	AccessKey string
	SecretKey string
	Token     string
	Region    string
	HTTP      *http.Client
	Now       func() time.Time
}

// ReaderFromCost reuses credentials already resolved for the cost client.
func ReaderFromCost(c *CostClient) *Reader {
	r := &Reader{Region: "us-east-1", Now: time.Now}
	if c == nil {
		r.HTTP = &http.Client{Timeout: 60 * time.Second}
		return r
	}
	r.AccessKey = c.AccessKey
	r.SecretKey = c.SecretKey
	r.Token = c.Token
	if c.Region != "" {
		r.Region = c.Region
	}
	if c.HTTP != nil {
		r.HTTP = c.HTTP
	} else {
		r.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	return r
}

// Enabled reports whether signing credentials are present.
func (r *Reader) Enabled() bool {
	return r != nil && r.AccessKey != "" && r.SecretKey != ""
}

func (r *Reader) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Regions returns enabled commercial regions (DescribeRegions).
func (r *Reader) Regions(ctx context.Context) ([]string, error) {
	body, err := r.postForm(ctx, "https://ec2."+r.Region+".amazonaws.com/", r.Region, "ec2", "2016-11-15", "DescribeRegions", nil)
	if err != nil {
		return nil, err
	}
	return ParseRegions(body)
}

// Volumes returns EBS volumes in region.
func (r *Reader) Volumes(ctx context.Context, region string) ([]inventory.Volume, error) {
	var all []inventory.Volume
	err := r.ec2Pages(ctx, region, "DescribeVolumes", nil, func(body []byte) (string, error) {
		items, next, err := ParseVolumes(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// Addresses returns Elastic IP allocations in region.
func (r *Reader) Addresses(ctx context.Context, region string) ([]inventory.Address, error) {
	var all []inventory.Address
	err := r.ec2Pages(ctx, region, "DescribeAddresses", nil, func(body []byte) (string, error) {
		items, next, err := ParseAddresses(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// ENIPublicIPs returns public IPv4 addresses attached to network interfaces.
func (r *Reader) ENIPublicIPs(ctx context.Context, region string) ([]inventory.ENIPublic, error) {
	var all []inventory.ENIPublic
	err := r.ec2Pages(ctx, region, "DescribeNetworkInterfaces", nil, func(body []byte) (string, error) {
		items, next, err := ParseENIPublicIPs(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// NATGateways returns NAT gateways with 7-day BytesOutToDestination sums.
func (r *Reader) NATGateways(ctx context.Context, region string) ([]inventory.NATGateway, error) {
	var all []inventory.NATGateway
	err := r.ec2Pages(ctx, region, "DescribeNatGateways", nil, func(body []byte) (string, error) {
		items, next, err := ParseNATGateways(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	if err != nil {
		return nil, err
	}
	for i := range all {
		sum, mErr := r.natBytes(ctx, region, all[i].ID)
		if mErr != nil {
			return nil, mErr
		}
		all[i].BytesOut = sum
	}
	return all, nil
}

func (r *Reader) natBytes(ctx context.Context, region, natID string) (float64, error) {
	end := r.now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	params := url.Values{}
	params.Set("Namespace", "AWS/NATGateway")
	params.Set("MetricName", "BytesOutToDestination")
	params.Set("StartTime", start.Format(time.RFC3339))
	params.Set("EndTime", end.Format(time.RFC3339))
	params.Set("Period", "86400")
	params.Set("Statistics.member.1", "Sum")
	params.Set("Dimensions.member.1.Name", "NatGatewayId")
	params.Set("Dimensions.member.1.Value", natID)
	body, err := r.postForm(ctx, "https://monitoring."+region+".amazonaws.com/", region, "monitoring", "2010-08-01", "GetMetricStatistics", params)
	if err != nil {
		return 0, err
	}
	return ParseMetricSum(body)
}

// VPCEndpoints returns VPC endpoints in region.
func (r *Reader) VPCEndpoints(ctx context.Context, region string) ([]inventory.VPCEndpoint, error) {
	var all []inventory.VPCEndpoint
	err := r.ec2Pages(ctx, region, "DescribeVpcEndpoints", nil, func(body []byte) (string, error) {
		items, next, err := ParseVPCEndpoints(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// Instances returns EC2 instances in region.
func (r *Reader) Instances(ctx context.Context, region string) ([]inventory.Instance, error) {
	var all []inventory.Instance
	err := r.ec2Pages(ctx, region, "DescribeInstances", nil, func(body []byte) (string, error) {
		items, next, err := ParseInstances(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// Snapshots returns snapshots owned by this account.
func (r *Reader) Snapshots(ctx context.Context, region string) ([]inventory.Snapshot, error) {
	params := url.Values{}
	params.Set("Owner.1", "self")
	var all []inventory.Snapshot
	err := r.ec2Pages(ctx, region, "DescribeSnapshots", params, func(body []byte) (string, error) {
		items, next, err := ParseSnapshots(body, region)
		if err != nil {
			return "", err
		}
		all = append(all, items...)
		return next, nil
	})
	return all, err
}

// LoadBalancers returns load balancers with healthy and total target counts.
func (r *Reader) LoadBalancers(ctx context.Context, region string) ([]inventory.LoadBalancer, error) {
	var lbs []inventory.LoadBalancer
	err := r.elbPages(ctx, region, "DescribeLoadBalancers", nil, func(body []byte) (string, error) {
		items, next, err := ParseLoadBalancers(body)
		if err != nil {
			return "", err
		}
		for i := range items {
			if items[i].Region == "" {
				items[i].Region = region
			}
		}
		lbs = append(lbs, items...)
		return next, nil
	})
	if err != nil {
		return nil, err
	}
	var groups []TargetGroup
	err = r.elbPages(ctx, region, "DescribeTargetGroups", nil, func(body []byte) (string, error) {
		items, next, err := ParseTargetGroups(body)
		if err != nil {
			return "", err
		}
		groups = append(groups, items...)
		return next, nil
	})
	if err != nil {
		return nil, err
	}
	health := map[string][2]int{}
	for _, g := range groups {
		h, t, hErr := r.targetHealth(ctx, region, g.ARN)
		if hErr != nil {
			return nil, hErr
		}
		health[g.ARN] = [2]int{h, t}
	}
	return ApplyTargetHealth(lbs, groups, health), nil
}

func (r *Reader) targetHealth(ctx context.Context, region, tgARN string) (int, int, error) {
	params := url.Values{}
	params.Set("TargetGroupArn", tgARN)
	body, err := r.postForm(ctx, "https://elasticloadbalancing."+region+".amazonaws.com/", region, "elasticloadbalancing", "2015-12-01", "DescribeTargetHealth", params)
	if err != nil {
		return 0, 0, err
	}
	return ParseTargetHealth(body)
}

// GetProducts calls the AWS Price List API (always signed in us-east-1).
func (r *Reader) GetProducts(ctx context.Context, q pricing.AWSPriceQuery) ([]pricing.Item, error) {
	payload := map[string]any{
		"ServiceCode": q.ServiceCode,
		"Filters": []map[string]string{
			{"Type": "TERM_MATCH", "Field": "usagetype", "Value": q.UsageType},
			{"Type": "TERM_MATCH", "Field": "location", "Value": q.Location},
		},
		"MaxResults": 10,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, err := r.postJSON(ctx, "https://api.pricing.us-east-1.amazonaws.com/", "us-east-1", "pricing", "AWSPriceListService.GetProducts", raw)
	if err != nil {
		return nil, err
	}
	region := q.Region
	if region == "" {
		region = r.Region
	}
	return pricing.ItemsFromAWSPriceList(body, region)
}

func (r *Reader) ec2Pages(ctx context.Context, region, action string, base url.Values, handle func([]byte) (string, error)) error {
	token := ""
	for page := 0; page < 20; page++ {
		params := cloneValues(base)
		if token != "" {
			params.Set("NextToken", token)
		}
		body, err := r.postForm(ctx, "https://ec2."+region+".amazonaws.com/", region, "ec2", "2016-11-15", action, params)
		if err != nil {
			return err
		}
		next, err := handle(body)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		token = next
	}
	return fmt.Errorf("%s: stopped after 20 pages", action)
}

func (r *Reader) elbPages(ctx context.Context, region, action string, base url.Values, handle func([]byte) (string, error)) error {
	marker := ""
	for page := 0; page < 20; page++ {
		params := cloneValues(base)
		if marker != "" {
			params.Set("Marker", marker)
		}
		body, err := r.postForm(ctx, "https://elasticloadbalancing."+region+".amazonaws.com/", region, "elasticloadbalancing", "2015-12-01", action, params)
		if err != nil {
			return err
		}
		next, err := handle(body)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		marker = next
	}
	return fmt.Errorf("%s: stopped after 20 pages", action)
}

func (r *Reader) postForm(ctx context.Context, endpoint, region, service, version, action string, params url.Values) ([]byte, error) {
	if !r.Enabled() {
		return nil, fmt.Errorf("AWS credentials not configured")
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("Action", action)
	params.Set("Version", version)
	payload := params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	signV4(req, []byte(payload), r.AccessKey, r.SecretKey, r.Token, region, service, r.now())
	return r.do(req, service+" "+action)
}

func (r *Reader) postJSON(ctx context.Context, endpoint, region, service, target string, payload []byte) ([]byte, error) {
	if !r.Enabled() {
		return nil, fmt.Errorf("AWS credentials not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	signV4(req, payload, r.AccessKey, r.SecretKey, r.Token, region, service, r.now())
	return r.do(req, target)
}

func (r *Reader) do(req *http.Request, what string) ([]byte, error) {
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%s status %d (needs a read-only permission such as ReadOnlyAccess): %s", what, resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("%s status %d: %s", what, resp.StatusCode, msg)
	}
	return body, nil
}

func cloneValues(in url.Values) url.Values {
	out := url.Values{}
	for k, vs := range in {
		for _, v := range vs {
			out.Add(k, v)
		}
	}
	return out
}
