package pricing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPriceKeyHelpers(t *testing.T) {
	if AzureDiskKey("Premium_LRS", 128) != "azure.disk.premium.p10" {
		t.Fatal(AzureDiskKey("Premium_LRS", 128))
	}
	if AzureDiskKey("StandardSSD_LRS", 64) != "azure.disk.ssd.e6" {
		t.Fatal(AzureDiskKey("StandardSSD_LRS", 64))
	}
	if AzureDiskKey("Standard_LRS", 4096) != "azure.disk.hdd.s50" {
		t.Fatal(AzureDiskKey("Standard_LRS", 4096))
	}
	if AzureVMKey("Standard_B1s") != "azure.vm.standard_b1s" || AzureVMKey("  ") != "" {
		t.Fatal("vm key")
	}
	if AzurePlanKey("B1") != "azure.plan.b1" || AzurePlanKey("") != "" {
		t.Fatal("plan key")
	}
	if AWSEBSKey("GP3") != "aws.ebs.gp3" || AWSEBSKey("") != "" {
		t.Fatal("ebs key")
	}
	if AWSEC2Key("T3.Micro") != "aws.ec2.t3.micro" || AWSEC2Key("") != "" {
		t.Fatal("ec2 key")
	}
	for _, size := range []float64{32, 64, 128, 256, 512, 1024, 2048, 4096} {
		if diskTierNumber(size) == "" {
			t.Fatal(size)
		}
	}
}

func TestRetailAndPriceListFilters(t *testing.T) {
	keys := []string{
		"azure.disk.hdd.s10", "azure.disk.ssd.e10", "azure.disk.premium.p10",
		KeyAzurePublicIPStandard, KeyAzurePublicIPBasic, KeyAzureNATGateway,
		KeyAzureSnapshotGB, KeyAzureLogAnalyticsGB, "azure.plan.b1", "azure.vm.standard_b1s",
		"unknown",
	}
	for _, key := range keys {
		filter, ok := AzureRetailFilter(key, "east'us")
		if key == "unknown" || key == KeyAzureSnapshotGB {
			if ok {
				t.Fatalf("%s should not be fetched: %s", key, filter)
			}
			continue
		}
		if !ok || !strings.Contains(filter, "eastus") {
			t.Fatalf("%s filter %s ok %v", key, filter, ok)
		}
	}
	if _, ok := AzureRetailFilter(KeyAzureNATGateway, ""); !ok {
		t.Fatal("empty region should default")
	}
	if armSKUFromKey("azure.vm.") != "" || armSKUFromKey("azure.vm.standard_b1s") == "" {
		t.Fatal("arm sku")
	}

	awsKeys := []string{
		"aws.ebs.gp3", "aws.ebs.gp2", KeyAWSSnapshotGB, KeyAWSNATGateway, KeyAWSPublicIPv4,
		KeyAWSVPCEndpointHour, KeyAWSALBHour, KeyAWSNLBHour, "aws.ec2.t3.micro", "nope",
	}
	for _, key := range awsKeys {
		queries := AWSPriceQueries(key, "us-east-1")
		if key == "nope" {
			if queries != nil {
				t.Fatalf("unexpected queries for %s", key)
			}
			continue
		}
		if len(queries) != 1 || queries[0].Location != "US East (N. Virginia)" {
			t.Fatalf("%s queries %+v", key, queries)
		}
	}
	if AWSPriceQueries("aws.ebs.gp3", "no-such-region") != nil {
		t.Fatal("unknown region")
	}
	if loc, ok := AWSLocationName("eu-central-1"); !ok || loc == "" {
		t.Fatal("location")
	}
}

func TestItemsFromPriceDocuments(t *testing.T) {
	docs := []string{
		priceDoc("EBS:VolumeUsage.gp3", "", "0.08", "GB-Mo"),
		priceDoc("EBS:SnapshotUsage", "", "0.05", "GB-Mo"),
		priceDoc("NatGateway-Hours", "", "0.045", "Hrs"),
		priceDoc("PublicIPv4:InUseAddress", "", "0.005", "Hrs"),
		priceDoc("VpcEndpoint-Hours", "", "0.01", "Hrs"),
		priceDoc("LoadBalancerUsage", "Load Balancer-Network", "0.0225", "Hrs"),
		priceDoc("LoadBalancerUsage", "Load Balancer-Application", "0.0225", "Hrs"),
		priceDoc("BoxUsage:t3.micro", "", "0.0104", "Hrs"),
		priceDoc("BoxUsage:", "", "0.01", "Hrs"),
		priceDoc("SomethingElse", "", "1", "Hrs"),
	}
	body, _ := json.Marshal(map[string]any{"PriceList": docs})
	items, err := ItemsFromAWSPriceList(body, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 7 {
		t.Fatalf("items %+v", items)
	}
	if _, err := ItemsFromAWSPriceList([]byte("{"), "us-east-1"); err == nil {
		t.Fatal("expected bad price list")
	}
	badItem, _ := json.Marshal(map[string]any{"PriceList": []string{"{"}})
	if _, err := ItemsFromAWSPriceList(badItem, "us-east-1"); err == nil {
		t.Fatal("expected bad item")
	}

	retail := `{
		"Items": [
			{"currencyCode":"USD","retailPrice":5.89,"unitOfMeasure":"1/Month","armRegionName":"eastus","meterName":"S10 LRS Disk","productName":"Standard HDD Managed Disks","skuName":"S10 LRS","type":"Consumption"},
			{"currencyCode":"USD","retailPrice":0,"unitPrice":0.005,"unitOfMeasure":"1 Hour","productName":"IP Addresses","meterName":"Standard IPv4","skuName":"Standard","type":"Consumption"},
			{"currencyCode":"EUR","retailPrice":1,"unitOfMeasure":"1 Hour","productName":"IP Addresses","meterName":"Basic","type":"Consumption"},
			{"currencyCode":"USD","retailPrice":9,"unitOfMeasure":"1 Hour","productName":"Virtual Machines","meterName":"B1s","armSkuName":"Standard_B1s","serviceName":"Virtual Machines","type":"Consumption"},
			{"currencyCode":"USD","retailPrice":1,"unitOfMeasure":"1 Hour","productName":"Windows Virtual Machines","meterName":"B1s","type":"Consumption"},
			{"type":"Reservation","currencyCode":"USD","retailPrice":1,"unitOfMeasure":"1 Hour","productName":"IP Addresses","meterName":"Standard"}
		]
	}`
	parsed, err := ItemsFromAzureRetail([]byte(retail))
	if err != nil || len(parsed) < 3 {
		t.Fatalf("retail %+v err=%v", parsed, err)
	}
	if _, err := ItemsFromAzureRetail([]byte("{")); err == nil {
		t.Fatal("expected bad retail json")
	}
}

func TestFetchAzureRetailFixture(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, `{"Items":[{"currencyCode":"USD","retailPrice":0.045,"unitOfMeasure":"1 Hour","productName":"NAT Gateway","meterName":"Standard Gateway","type":"Consumption"}]}`)
			return
		}
		if r.Header.Get("User-Agent") != "azguard" {
			t.Errorf("user agent %s", r.Header.Get("User-Agent"))
		}
		_, _ = io.WriteString(w, `{"Items":[{"currencyCode":"USD","retailPrice":0.005,"unitOfMeasure":"1 Hour","productName":"IP Addresses","meterName":"Standard IPv4","type":"Consumption"}],"NextPageLink":"`+srv.URL+`?page=2"}`)
	}))
	t.Cleanup(srv.Close)
	orig := azureRetailEndpoint
	azureRetailEndpoint = srv.URL
	t.Cleanup(func() { azureRetailEndpoint = orig })

	items, err := FetchAzureRetail(context.Background(), nil, "serviceName eq 'Virtual Network'")
	if err != nil || len(items) != 2 {
		t.Fatalf("items %+v err=%v", items, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusTeapot)
	}))
	t.Cleanup(bad.Close)
	azureRetailEndpoint = bad.URL
	if _, err := FetchAzureRetail(context.Background(), bad.Client(), "x"); err == nil {
		t.Fatal("expected status error")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(broken.Close)
	azureRetailEndpoint = broken.URL
	if _, err := FetchAzureRetail(context.Background(), broken.Client(), "x"); err == nil {
		t.Fatal("expected json error")
	}
}

func priceDoc(usage, group, usd, unit string) string {
	return `{
		"product":{"attributes":{"usagetype":"` + usage + `","group":"` + group + `","productFamily":"` + group + `"}},
		"terms":{"OnDemand":{"term":{"priceDimensions":{"dim":{"unit":"` + unit + `","pricePerUnit":{"USD":"` + usd + `"}}}}}}
	}`
}
