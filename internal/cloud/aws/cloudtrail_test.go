package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type redirectTransport struct {
	target *url.URL
}

func (t redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL = t.target
	clone.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestLookupEventsRequestsBillableNames(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != "com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101.LookupEvents" {
			t.Errorf("target = %s", r.Header.Get("X-Amz-Target"))
		}
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"Events":[]}`))
	}))
	defer srv.Close()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	reader := &Reader{
		AccessKey: "AKIATEST",
		SecretKey: "secret",
		Region:    "us-east-1",
		HTTP:      &http.Client{Transport: redirectTransport{target: target}},
		Now:       func() time.Time { return time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC) },
	}
	since := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if _, err := reader.LookupEvents(context.Background(), "us-east-1", since, since.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != len(BillableEventNames) {
		t.Fatalf("calls = %d, want %d", len(bodies), len(BillableEventNames))
	}
	joined := strings.Join(bodies, "\n")
	for _, name := range []string{"RunInstances", "CreateNatGateway", "AllocateAddress", "CreateVpcEndpoint", "CreateStack", "CreateDBInstance", "CreateLoadBalancer", "CreateVolume"} {
		if !strings.Contains(joined, name) {
			t.Fatalf("missing LookupEvents attribute %s", name)
		}
	}
}
