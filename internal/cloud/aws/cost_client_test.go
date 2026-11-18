package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreeTierAndCostResponses(t *testing.T) {
	var sawFreeTier, sawCost bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") == "" || r.Header.Get("X-Amz-Date") == "" {
			t.Errorf("request was not signed")
		}
		switch r.Header.Get("X-Amz-Target") {
		case "AWSFreeTierService.GetFreeTierUsage":
			sawFreeTier = true
			_, _ = io.WriteString(w, `{
				"freeTierUsages": [
					{"service":"Amazon EC2","usageType":"BoxUsage","description":"hours","actualUsageAmount":100,"forecastedUsageAmount":120,"limit":{"amount":750,"unit":"hours"}},
					{"service":"Amazon S3","usageType":"TimedStorage","actualUsageAmount":1,"limit":{"amount":0,"unit":"GB"}}
				]
			}`)
		case "AWSInsightsIndexService.GetCostAndUsage":
			sawCost = true
			if !strings.Contains(string(raw), "2026-11-01") || !strings.Contains(string(raw), "SERVICE") {
				t.Errorf("cost payload %s", raw)
			}
			_, _ = io.WriteString(w, `{
				"ResultsByTime": [{
					"TimePeriod": {"Start":"2026-11-01","End":"2026-12-01"},
					"Groups": [
						{"Keys":["Amazon EC2"],"Metrics":{"UnblendedCost":{"Amount":"1.50","Unit":"USD"}}},
						{"Keys":["Amazon S3"],"Metrics":{"BlendedCost":{"Amount":"9","Unit":"USD"}}},
						{"Keys":[],"Metrics":{"UnblendedCost":{"Amount":"nope","Unit":"USD"}}}
					]
				}]
			}`)
		default:
			http.Error(w, "unknown target", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &CostClient{
		AccessKey: "AKIATEST",
		SecretKey: "secret",
		Region:    "us-east-1",
		HTTP:      &http.Client{Transport: redirectTransport{target: target}},
	}
	usages, err := client.GetFreeTierUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sawFreeTier || len(usages) != 2 || usages[0].PercentUsed != 100.0/750.0*100 || usages[0].FreeTierLimit != 750 {
		t.Fatalf("usages = %+v", usages)
	}
	if usages[1].PercentUsed != 0 {
		t.Fatalf("zero limit percent = %v", usages[1].PercentUsed)
	}

	costs, err := client.QueryCostsByService(context.Background(), "2026-11-01", "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	if !sawCost || costs.TotalCost != 1.5 || len(costs.Records) != 2 || costs.Records[0].ServiceName != "Amazon EC2" {
		t.Fatalf("costs = %+v", costs)
	}
}

func TestCostClientErrors(t *testing.T) {
	client := &CostClient{}
	if client.IsConfigured() {
		t.Fatal("empty client should not be configured")
	}
	if _, err := client.GetFreeTierUsage(context.Background()); err == nil {
		t.Fatal("expected missing credentials")
	}
	if _, err := client.QueryCostsByService(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected missing credentials")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Amz-Target") {
		case "AWSFreeTierService.GetFreeTierUsage":
			http.Error(w, "denied", http.StatusForbidden)
		default:
			_, _ = io.WriteString(w, "{")
		}
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client = &CostClient{
		AccessKey: "AKIATEST",
		SecretKey: "secret",
		Region:    "us-east-1",
		HTTP:      &http.Client{Transport: redirectTransport{target: target}},
	}
	if _, err := client.GetFreeTierUsage(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("status error = %v", err)
	}
	if _, err := client.QueryCostsByService(context.Background(), "2026-01-01", "2026-02-01"); err == nil {
		t.Fatal("expected bad json")
	}

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL, _ := url.Parse(closed.URL)
	closed.Close()
	client.HTTP = &http.Client{Transport: redirectTransport{target: closedURL}}
	if _, err := client.callAPI(context.Background(), "https://ce.us-east-1.amazonaws.com", "AWSInsightsIndexService.GetCostAndUsage", `{}`); err == nil {
		t.Fatal("expected transport error")
	}
}

func TestNewCostClientCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIATESTFROMENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret")
	t.Setenv("AWS_SESSION_TOKEN", "env-token")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	fromEnv := NewCostClient("", "", "", "")
	if fromEnv.AccessKey != "AKIATESTFROMENV" || fromEnv.SecretKey != "env-secret" || fromEnv.Token != "env-token" || fromEnv.Region != "eu-west-1" {
		t.Fatalf("env client = %+v", fromEnv)
	}
	explicit := NewCostClient("AKIATESTEXPLICIT", "explicit-secret", "explicit-token", "us-west-2")
	if explicit.AccessKey != "AKIATESTEXPLICIT" || explicit.Region != "us-west-2" || explicit.Token != "explicit-token" {
		t.Fatalf("explicit client = %+v", explicit)
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "aws")
	body := "#!/bin/sh\necho 'export AWS_ACCESS_KEY_ID=AKIAFAKECLI'\necho 'AWS_SECRET_ACCESS_KEY=cli-secret'\necho 'ignored'\necho 'export AWS_SESSION_TOKEN=cli-token'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	fromCLI := NewCostClient("", "", "", "")
	if fromCLI.AccessKey != "AKIAFAKECLI" || fromCLI.SecretKey != "cli-secret" || fromCLI.Token != "cli-token" || fromCLI.Region != "us-east-1" {
		t.Fatalf("cli client = %+v", fromCLI)
	}

	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := NewCostClient("", "", "", "ap-southeast-2")
	if missing.IsConfigured() || missing.Region != "ap-southeast-2" {
		t.Fatalf("failed cli client = %+v", missing)
	}
	if _, _, _, err := getCredentialsFromCLI(); err == nil {
		t.Fatal("expected cli failure")
	}
}
