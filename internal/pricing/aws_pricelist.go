package pricing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// priceListResponse is the AWS Price List GetProducts payload.
// Each PriceList entry is a JSON document encoded as a string.
type priceListResponse struct {
	PriceList []string `json:"PriceList"`
}

type priceDocument struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				Unit         string            `json:"unit"`
				Description  string            `json:"description"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// ItemsFromAWSPriceList maps a GetProducts body to price items.
// region is the region code the request was made for (the API speaks location names).
func ItemsFromAWSPriceList(body []byte, region string) ([]Item, error) {
	var resp priceListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse aws price list: %w", err)
	}
	var items []Item
	for _, raw := range resp.PriceList {
		var doc priceDocument
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, fmt.Errorf("parse aws price list item: %w", err)
		}
		if it, ok := mapAWSPrice(doc, region); ok {
			items = append(items, it)
		}
	}
	return items, nil
}

func mapAWSPrice(doc priceDocument, region string) (Item, bool) {
	usage := doc.Product.Attributes["usagetype"]
	group := strings.ToLower(doc.Product.Attributes["group"] + " " + doc.Product.Attributes["productFamily"] + " " + doc.Product.Attributes["volumeType"])
	usd, _, ok := firstOnDemandUSD(doc)
	if !ok {
		return Item{}, false
	}
	item := Item{
		Region: region,
		USD:    usd,
		Source: SourceAWSPriceList,
	}
	usageLower := strings.ToLower(usage)
	switch {
	case strings.Contains(usage, "EBS:VolumeUsage."):
		vol := strings.ToLower(usage[strings.LastIndex(usage, ".")+1:])
		item.Key = AWSEBSKey(vol)
		item.Unit = UnitGBMonth
		return item, item.Key != ""
	case strings.Contains(usage, "EBS:SnapshotUsage") || strings.Contains(usageLower, "snapshotusage"):
		item.Key = KeyAWSSnapshotGB
		item.Unit = UnitGBMonth
		return item, true
	case strings.Contains(usage, "NatGateway-Hours"):
		item.Key = KeyAWSNATGateway
		item.Unit = UnitHour
		return item, true
	case strings.Contains(usage, "PublicIPv4:") || strings.Contains(usage, "ElasticIP:"):
		item.Key = KeyAWSPublicIPv4
		item.Unit = UnitHour
		return item, true
	case strings.Contains(usage, "VpcEndpoint-Hours"):
		item.Key = KeyAWSVPCEndpointHour
		item.Unit = UnitHour
		return item, true
	case strings.Contains(usage, "LoadBalancerUsage"):
		if strings.Contains(group, "network") {
			item.Key = KeyAWSNLBHour
		} else {
			item.Key = KeyAWSALBHour
		}
		item.Unit = UnitHour
		return item, true
	case strings.Contains(usage, "BoxUsage:"):
		inst := strings.ToLower(usage[strings.LastIndex(usage, ":")+1:])
		item.Key = AWSEC2Key(inst)
		item.Unit = UnitHour
		return item, item.Key != ""
	default:
		return Item{}, false
	}
}

func firstOnDemandUSD(doc priceDocument) (float64, string, bool) {
	for _, term := range doc.Terms.OnDemand {
		for _, dim := range term.PriceDimensions {
			raw := dim.PricePerUnit["USD"]
			if raw == "" {
				continue
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			return v, dim.Unit, true
		}
	}
	return 0, "", false
}

// AWSPriceQuery describes one GetProducts call.
type AWSPriceQuery struct {
	ServiceCode string
	UsageType   string
	Region      string
	Location    string
}

// AWSLocationName maps a commercial region code to the Price List location attribute.
func AWSLocationName(region string) (string, bool) {
	loc, ok := awsLocations[region]
	return loc, ok
}

var awsLocations = map[string]string{
	"us-east-1":      "US East (N. Virginia)",
	"us-east-2":      "US East (Ohio)",
	"us-west-1":      "US West (N. California)",
	"us-west-2":      "US West (Oregon)",
	"eu-west-1":      "EU (Ireland)",
	"eu-west-2":      "EU (London)",
	"eu-central-1":   "EU (Frankfurt)",
	"ap-southeast-1": "Asia Pacific (Singapore)",
	"ap-southeast-2": "Asia Pacific (Sydney)",
	"ap-northeast-1": "Asia Pacific (Tokyo)",
}

// AWSPriceQueries returns the GetProducts queries that can override key.
func AWSPriceQueries(key, region string) []AWSPriceQuery {
	loc, ok := AWSLocationName(region)
	if !ok || key == "" {
		return nil
	}
	q := func(service, usage string) AWSPriceQuery {
		return AWSPriceQuery{ServiceCode: service, UsageType: usage, Region: region, Location: loc}
	}
	switch {
	case key == "aws.ebs.gp3":
		return []AWSPriceQuery{q("AmazonEC2", "EBS:VolumeUsage.gp3")}
	case key == "aws.ebs.gp2":
		return []AWSPriceQuery{q("AmazonEC2", "EBS:VolumeUsage.gp2")}
	case key == KeyAWSSnapshotGB:
		return []AWSPriceQuery{q("AmazonEC2", "EBS:SnapshotUsage")}
	case key == KeyAWSNATGateway:
		return []AWSPriceQuery{q("AmazonEC2", "NatGateway-Hours")}
	case key == KeyAWSPublicIPv4:
		return []AWSPriceQuery{q("AmazonEC2", "PublicIPv4:InUseAddress")}
	case key == KeyAWSVPCEndpointHour:
		return []AWSPriceQuery{q("AmazonEC2", "VpcEndpoint-Hours")}
	case key == KeyAWSALBHour:
		return []AWSPriceQuery{q("AWSELB", "LoadBalancerUsage")}
	case key == KeyAWSNLBHour:
		return []AWSPriceQuery{q("AWSELB", "LoadBalancerUsage")}
	case strings.HasPrefix(key, "aws.ec2."):
		inst := strings.TrimPrefix(key, "aws.ec2.")
		return []AWSPriceQuery{q("AmazonEC2", "BoxUsage:"+inst)}
	default:
		return nil
	}
}
