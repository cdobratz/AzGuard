package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	awscloud "github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/findings"
	"github.com/azguard/azguard/internal/pricing"
)

// enrichPrices overlays Azure Retail Prices and AWS Price List responses onto book.
// Failures are warnings. The bundled list price stays in place for any key the APIs miss.
func enrichPrices(ctx context.Context, found []findings.Finding, book *pricing.Book, awsReader *awscloud.Reader) []string {
	if len(found) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	azureFilters := map[string]struct{}{}
	awsQueries := map[string]pricing.AWSPriceQuery{}
	for _, f := range found {
		if f.PriceKey == "" {
			continue
		}
		if filter, ok := pricing.AzureRetailFilter(f.PriceKey, f.Region); ok {
			azureFilters[filter] = struct{}{}
		}
		if awsReader != nil && awsReader.Enabled() {
			for _, q := range pricing.AWSPriceQueries(f.PriceKey, f.Region) {
				awsQueries[q.ServiceCode+"\n"+q.UsageType+"\n"+q.Region] = q
			}
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var warnings []string
	sem := make(chan struct{}, 4)
	client := &http.Client{Timeout: 8 * time.Second}
	add := func(items []pricing.Item) {
		mu.Lock()
		book.Add(items...)
		mu.Unlock()
	}
	warn := func(msg string) {
		mu.Lock()
		warnings = append(warnings, msg)
		mu.Unlock()
	}

	for filter := range azureFilters {
		filter := filter
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items, err := pricing.FetchAzureRetail(ctx, client, filter)
			if err != nil {
				warn("azure retail prices: " + err.Error())
				return
			}
			add(items)
		}()
	}
	for _, q := range awsQueries {
		q := q
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items, err := awsReader.GetProducts(ctx, q)
			if err != nil {
				warn(fmt.Sprintf("aws price list %s: %s", q.UsageType, err.Error()))
				return
			}
			add(items)
		}()
	}
	wg.Wait()
	if len(warnings) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("live price lookup failed for %d request(s); bundled list prices were kept where the API did not answer", len(warnings))}
}
