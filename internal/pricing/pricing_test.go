package pricing

import (
	"math"
	"os"
	"testing"
)

// TestSampleSKUListPrices pins recorded Azure Retail Prices and AWS Price List
// responses. The estimate must stay within 10% of the published list price.
func TestSampleSKUListPrices(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		region   string
		key      string
		quantity float64
		list     float64
		parse    func([]byte) (*Book, error)
	}{
		{
			name: "azure standard hdd s10", file: "testdata/azure_s10.json", region: "eastus",
			key: "azure.disk.hdd.s10", quantity: 1, list: 5.89, parse: bookFromAzure,
		},
		{
			name: "azure standard public ipv4", file: "testdata/azure_public_ip.json", region: "eastus",
			key: KeyAzurePublicIPStandard, quantity: 1, list: 3.65, parse: bookFromAzure,
		},
		{
			name: "aws ebs gp3 100gb", file: "testdata/aws_gp3.json", region: "us-east-1",
			key: "aws.ebs.gp3", quantity: 100, list: 8.00, parse: bookFromAWS,
		},
		{
			name: "aws public ipv4", file: "testdata/aws_ipv4.json", region: "us-east-1",
			key: KeyAWSPublicIPv4, quantity: 1, list: 3.65, parse: bookFromAWS,
		},
		{
			name: "aws nat gateway", file: "testdata/aws_nat.json", region: "us-east-1",
			key: KeyAWSNATGateway, quantity: 1, list: 32.85, parse: bookFromAWS,
		},
	}
	if len(tests) < 5 {
		t.Fatalf("need 5 sample SKUs, got %d", len(tests))
	}
	bundled := Bundled()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			book, err := tt.parse(body)
			if err != nil {
				t.Fatal(err)
			}
			got, source, ok := book.Monthly(tt.key, tt.region, tt.quantity)
			if !ok {
				t.Fatalf("no price for %s", tt.key)
			}
			if source != SourceAzureRetail && source != SourceAWSPriceList {
				t.Fatalf("source = %s, want a live price-list source", source)
			}
			within10(t, got, tt.list)
			bGot, _, bOK := bundled.Monthly(tt.key, tt.region, tt.quantity)
			if !bOK {
				t.Fatalf("bundled table missing %s", tt.key)
			}
			within10(t, bGot, tt.list)
		})
	}
}

func bookFromAzure(body []byte) (*Book, error) {
	items, err := ItemsFromAzureRetail(body)
	if err != nil {
		return nil, err
	}
	return NewBook(items), nil
}

func bookFromAWS(body []byte) (*Book, error) {
	items, err := ItemsFromAWSPriceList(body, "us-east-1")
	if err != nil {
		return nil, err
	}
	return NewBook(items), nil
}

func within10(t *testing.T, got, list float64) {
	t.Helper()
	if list == 0 {
		t.Fatal("list price is 0")
	}
	delta := math.Abs(got-list) / list
	if delta > 0.10 {
		t.Fatalf("got %.4f, list %.4f, delta %.2f%% (limit 10%%)", got, list, delta*100)
	}
}
