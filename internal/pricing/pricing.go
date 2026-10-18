// Package pricing estimates monthly list prices for idle-resource findings.
//
// Estimates prefer the Azure Retail Prices API and the AWS Price List API.
// When those calls are unavailable, Bundled supplies a small list-price table
// (730 hours per month). Tests pin recorded responses from both APIs.
package pricing

import "math"

const (
	// HoursPerMonth is the hour count used to turn an hourly list price into a
	// monthly estimate. 730 is the conventional cloud-billing average
	// (365 days / 12 * 24).
	HoursPerMonth = 730.0

	SourceBundled      = "bundled"
	SourceAzureRetail  = "azure-retail"
	SourceAWSPriceList = "aws-price-list"
	SourceUnpriced     = "unpriced"

	UnitMonth   = "month"
	UnitHour    = "hour"
	UnitGBMonth = "gb-month"
)

// Canonical price keys shared by rules, the bundled table, and API mappers.
const (
	KeyAzurePublicIPStandard = "azure.publicip.standard"
	KeyAzurePublicIPBasic    = "azure.publicip.basic"
	KeyAzureNATGateway       = "azure.natgateway"
	KeyAzureSnapshotGB       = "azure.snapshot.gb"
	KeyAzureLogAnalyticsGB   = "azure.loganalytics.gb"

	KeyAWSPublicIPv4      = "aws.ipv4"
	KeyAWSNATGateway      = "aws.natgateway"
	KeyAWSVPCEndpointHour = "aws.vpcendpoint.interface"
	KeyAWSALBHour         = "aws.elb.application"
	KeyAWSNLBHour         = "aws.elb.network"
	KeyAWSSnapshotGB      = "aws.snapshot.gb"
)

// Item is one list price. An empty Region means the price applies in every region.
type Item struct {
	Key    string
	Region string
	Unit   string
	USD    float64
	Source string
}

// Book is a set of list prices. Region-specific items override region-independent ones.
type Book struct {
	items map[string]Item
}

// NewBook returns a book containing items. Later items with the same key and region win.
func NewBook(items []Item) *Book {
	b := &Book{items: map[string]Item{}}
	b.Add(items...)
	return b
}

// Add overlays items onto the book.
func (b *Book) Add(items ...Item) {
	if b.items == nil {
		b.items = map[string]Item{}
	}
	for _, it := range items {
		if it.Key == "" || it.Source == "" {
			continue
		}
		b.items[it.Key+"\n"+it.Region] = it
	}
}

// Monthly returns the estimated monthly USD cost for quantity units of key in region.
// Quantity is a count for monthly and hourly prices (hourly prices are multiplied by
// HoursPerMonth) and a size in GB for gb-month prices.
func (b *Book) Monthly(key, region string, quantity float64) (usd float64, source string, ok bool) {
	if b == nil || key == "" {
		return 0, SourceUnpriced, false
	}
	it, found := b.lookup(key, region)
	if !found {
		return 0, SourceUnpriced, false
	}
	return MonthlyUSD(it.Unit, it.USD, quantity), it.Source, true
}

func (b *Book) lookup(key, region string) (Item, bool) {
	if region != "" {
		if it, ok := b.items[key+"\n"+region]; ok {
			return it, true
		}
	}
	it, ok := b.items[key+"\n"]
	return it, ok
}

// MonthlyUSD converts a unit price and quantity into a monthly USD amount rounded to cents.
func MonthlyUSD(unit string, unitUSD, quantity float64) float64 {
	var v float64
	switch unit {
	case UnitHour:
		v = unitUSD * HoursPerMonth * quantity
	default:
		v = unitUSD * quantity
	}
	return RoundUSD(v)
}

// RoundUSD rounds to cents, which is how the commands display money.
func RoundUSD(v float64) float64 {
	return math.Round(v*100) / 100
}
