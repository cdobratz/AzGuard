package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const azureRetailEndpoint = "https://prices.azure.com/api/retail/prices"

// retailResponse is the Azure Retail Prices API payload.
type retailResponse struct {
	Items        []retailItem `json:"Items"`
	NextPageLink string       `json:"NextPageLink"`
}

type retailItem struct {
	CurrencyCode  string  `json:"currencyCode"`
	RetailPrice   float64 `json:"retailPrice"`
	UnitPrice     float64 `json:"unitPrice"`
	UnitOfMeasure string  `json:"unitOfMeasure"`
	ArmRegionName string  `json:"armRegionName"`
	MeterName     string  `json:"meterName"`
	ProductName   string  `json:"productName"`
	SkuName       string  `json:"skuName"`
	ServiceName   string  `json:"serviceName"`
	ArmSkuName    string  `json:"armSkuName"`
	Type          string  `json:"type"`
}

var diskMeterRE = regexp.MustCompile(`(?i)\b([spe])(\d+)\b`)

// ItemsFromAzureRetail maps a Retail Prices API body to price items.
// Reservation and DevTestConsumption rows are ignored. Only USD rows are kept.
func ItemsFromAzureRetail(body []byte) ([]Item, error) {
	var resp retailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse azure retail prices: %w", err)
	}
	var items []Item
	for _, row := range resp.Items {
		if it, ok := mapAzureRetail(row); ok {
			items = append(items, it)
		}
	}
	return items, nil
}

func mapAzureRetail(row retailItem) (Item, bool) {
	if row.Type != "" && !strings.EqualFold(row.Type, "Consumption") {
		return Item{}, false
	}
	if row.CurrencyCode != "" && !strings.EqualFold(row.CurrencyCode, "USD") {
		return Item{}, false
	}
	unit := unitFromMeasure(row.UnitOfMeasure)
	if unit == "" {
		return Item{}, false
	}
	price := row.RetailPrice
	if price == 0 {
		price = row.UnitPrice
	}
	blob := strings.ToLower(row.ProductName + " " + row.MeterName + " " + row.SkuName + " " + row.ServiceName)
	if strings.Contains(blob, "spot") || strings.Contains(blob, "low priority") || strings.Contains(blob, "windows") {
		return Item{}, false
	}

	item := Item{
		Region: row.ArmRegionName,
		Unit:   unit,
		USD:    price,
		Source: SourceAzureRetail,
	}

	switch {
	case strings.Contains(blob, "managed disk"):
		m := diskMeterRE.FindStringSubmatch(row.MeterName + " " + row.SkuName)
		if m == nil {
			return Item{}, false
		}
		letter := strings.ToLower(m[1])
		num := m[2]
		family := "hdd"
		switch letter {
		case "p":
			family = "premium"
		case "e":
			family = "ssd"
		case "s":
			if strings.Contains(blob, "premium") {
				family = "premium"
				letter = "p"
			} else if strings.Contains(blob, "ssd") {
				family = "ssd"
				letter = "e"
			}
		}
		if strings.Contains(blob, "premium") {
			family = "premium"
			letter = "p"
		} else if strings.Contains(blob, "ssd") && !strings.Contains(blob, "hdd") {
			family = "ssd"
			letter = "e"
		}
		item.Key = "azure.disk." + family + "." + letter + num
		return item, true
	case strings.Contains(blob, "snapshot"):
		item.Key = KeyAzureSnapshotGB
		item.Unit = UnitGBMonth
		return item, true
	case strings.Contains(blob, "public ip") || strings.Contains(blob, "ipv4"):
		if strings.Contains(blob, "prefix") {
			return Item{}, false
		}
		if strings.Contains(blob, "basic") {
			item.Key = KeyAzurePublicIPBasic
		} else {
			item.Key = KeyAzurePublicIPStandard
		}
		return item, true
	case strings.Contains(blob, "nat gateway"):
		if strings.Contains(blob, "data processed") || strings.Contains(blob, "data transfer") {
			return Item{}, false
		}
		item.Key = KeyAzureNATGateway
		return item, true
	case strings.Contains(blob, "log analytics") || strings.Contains(blob, "analytics logs"):
		if strings.Contains(blob, "ingestion") || strings.Contains(blob, "data") {
			item.Key = KeyAzureLogAnalyticsGB
			item.Unit = UnitGBMonth
			return item, true
		}
		return Item{}, false
	case strings.Contains(blob, "app service") || strings.Contains(blob, "azure app service"):
		sku := strings.ToLower(strings.TrimSpace(row.SkuName))
		if sku == "" {
			sku = strings.ToLower(strings.TrimSpace(row.ArmSkuName))
		}
		if sku == "" {
			return Item{}, false
		}
		item.Key = AzurePlanKey(sku)
		return item, true
	case row.ArmSkuName != "" && (strings.Contains(blob, "virtual machines") || strings.EqualFold(row.ServiceName, "Virtual Machines")):
		if unit != UnitHour {
			return Item{}, false
		}
		item.Key = AzureVMKey(row.ArmSkuName)
		return item, true
	default:
		return Item{}, false
	}
}

func unitFromMeasure(measure string) string {
	m := strings.ToLower(strings.TrimSpace(measure))
	switch {
	case strings.Contains(m, "hour"):
		return UnitHour
	case strings.Contains(m, "gb"):
		return UnitGBMonth
	case strings.Contains(m, "month"):
		return UnitMonth
	default:
		return ""
	}
}

// AzureRetailFilter returns an OData filter that should include the meter for key.
func AzureRetailFilter(key, region string) (string, bool) {
	if region == "" {
		region = "eastus"
	}
	region = strings.ReplaceAll(region, "'", "")
	switch {
	case strings.HasPrefix(key, "azure.disk.hdd."):
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and productName eq 'Standard HDD Managed Disks'", region), true
	case strings.HasPrefix(key, "azure.disk.ssd."):
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and productName eq 'Standard SSD Managed Disks'", region), true
	case strings.HasPrefix(key, "azure.disk.premium."):
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and productName eq 'Premium SSD Managed Disks'", region), true
	case key == KeyAzurePublicIPStandard || key == KeyAzurePublicIPBasic:
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and serviceName eq 'Virtual Network' and productName eq 'IP Addresses'", region), true
	case key == KeyAzureNATGateway:
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and serviceName eq 'NAT Gateway'", region), true
	case key == KeyAzureSnapshotGB:
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and contains(productName, 'Snapshot')", region), false
	case key == KeyAzureLogAnalyticsGB:
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and serviceName eq 'Log Analytics'", region), true
	case strings.HasPrefix(key, "azure.plan."):
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and serviceName eq 'Azure App Service'", region), true
	case strings.HasPrefix(key, "azure.vm."):
		return fmt.Sprintf("armRegionName eq '%s' and priceType eq 'Consumption' and serviceName eq 'Virtual Machines' and armSkuName eq '%s'", region, armSKUFromKey(key)), true
	default:
		return "", false
	}
}

func armSKUFromKey(key string) string {
	s := strings.TrimPrefix(key, "azure.vm.")
	if s == "" {
		return s
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	// Standard_B1s is stored lower-case; Azure armSkuName keeps the original shape.
	// The retail API match is best-effort. Bundled prices cover the miss.
	return strings.Join(parts, "_")
}

// FetchAzureRetail queries the public Retail Prices API. It follows at most one next page.
func FetchAzureRetail(ctx context.Context, client *http.Client, filter string) ([]Item, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	u, err := url.Parse(azureRetailEndpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("currencyCode", "USD")
	q.Set("$filter", filter)
	u.RawQuery = q.Encode()

	var all []Item
	next := u.String()
	for page := 0; page < 2 && next != ""; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "azguard")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("azure retail prices status %d", resp.StatusCode)
		}
		items, err := ItemsFromAzureRetail(body)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		var parsed retailResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		next = parsed.NextPageLink
	}
	return all, nil
}
