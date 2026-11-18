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

	"github.com/cdobratz/azguard/internal/pricing"
)

func TestReaderParsesRecordedResponses(t *testing.T) {
	pages := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(r.Header.Get("Content-Type"), "json") || r.Header.Get("X-Amz-Target") != "" {
			if !strings.Contains(r.Header.Get("X-Amz-Target"), "GetProducts") {
				t.Errorf("unexpected json target %s", r.Header.Get("X-Amz-Target"))
			}
			_, _ = io.WriteString(w, `{"PriceList":[]}`)
			return
		}
		vals, _ := url.ParseQuery(string(raw))
		action := vals.Get("Action")
		pages[action]++
		if pages[action] > 1 {
			switch action {
			case "DescribeVolumes":
				_, _ = io.WriteString(w, `<DescribeVolumesResponse><volumeSet><item><volumeId>vol-2</volumeId><status>in-use</status><size>4</size><attachmentSet><item><volumeId>vol-2</volumeId></item></attachmentSet></item></volumeSet></DescribeVolumesResponse>`)
			default:
				_, _ = io.WriteString(w, `<Response></Response>`)
			}
			return
		}
		body := map[string]string{
			"DescribeRegions":           regionXML,
			"DescribeVolumes":           volumeXML,
			"DescribeAddresses":         addressXML,
			"DescribeNetworkInterfaces": eniXML,
			"DescribeNatGateways":       natXML,
			"GetMetricStatistics":       metricXML,
			"DescribeVpcEndpoints":      endpointXML,
			"DescribeInstances":         instanceXML,
			"DescribeSnapshots":         snapshotXML,
			"DescribeLoadBalancers":     elbXML,
			"DescribeTargetGroups":      targetGroupXML,
			"DescribeTargetHealth":      healthXML,
		}[action]
		if body == "" {
			http.Error(w, "unknown "+action, http.StatusBadRequest)
			return
		}
		if action == "DescribeSnapshots" && !strings.Contains(string(raw), "Owner.1=self") {
			t.Errorf("snapshots missing owner filter: %s", raw)
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	reader := testReaderAt(t, srv)

	ctx := context.Background()
	regions, err := reader.Regions(ctx)
	if err != nil || len(regions) != 2 {
		t.Fatalf("regions %v err=%v", regions, err)
	}
	volumes, err := reader.Volumes(ctx, "us-east-1")
	if err != nil || len(volumes) != 2 || volumes[1].ID != "vol-2" {
		t.Fatalf("volumes %+v err=%v", volumes, err)
	}
	addrs, err := reader.Addresses(ctx, "us-east-1")
	if err != nil || len(addrs) != 2 {
		t.Fatalf("addrs %+v err=%v", addrs, err)
	}
	enis, err := reader.ENIPublicIPs(ctx, "us-east-1")
	if err != nil || len(enis) != 1 {
		t.Fatalf("enis %+v err=%v", enis, err)
	}
	nats, err := reader.NATGateways(ctx, "us-east-1")
	if err != nil || len(nats) != 1 || nats[0].BytesOut != 15 {
		t.Fatalf("nats %+v err=%v", nats, err)
	}
	eps, err := reader.VPCEndpoints(ctx, "us-east-1")
	if err != nil || len(eps) != 1 {
		t.Fatalf("endpoints %+v err=%v", eps, err)
	}
	instances, err := reader.Instances(ctx, "us-east-1")
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances %+v err=%v", instances, err)
	}
	snaps, err := reader.Snapshots(ctx, "us-east-1")
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snaps %+v err=%v", snaps, err)
	}
	lbs, err := reader.LoadBalancers(ctx, "us-east-1")
	if err != nil || len(lbs) != 1 || lbs[0].Healthy != 1 || lbs[0].Total != 2 {
		t.Fatalf("lbs %+v err=%v", lbs, err)
	}
	items, err := reader.GetProducts(ctx, pricing.AWSPriceQuery{
		ServiceCode: "AmazonEC2",
		UsageType:   "EBS:VolumeUsage.gp3",
		Location:    "US East (N. Virginia)",
	})
	if err != nil || len(items) != 0 {
		t.Fatalf("products %+v err=%v", items, err)
	}
}

func TestReaderErrors(t *testing.T) {
	if _, err := (&Reader{}).Volumes(context.Background(), "us-east-1"); err == nil {
		t.Fatal("expected missing credentials")
	}
	fromNil := ReaderFromCost(nil)
	if fromNil.Enabled() || fromNil.HTTP == nil {
		t.Fatal("nil cost client should still build a reader")
	}
	fromCost := ReaderFromCost(&CostClient{AccessKey: "AKIATEST", SecretKey: "secret", Region: "eu-west-1"})
	if !fromCost.Enabled() || fromCost.Region != "eu-west-1" {
		t.Fatalf("from cost %+v", fromCost)
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(denied.Close)
	reader := testReaderAt(t, denied)
	if _, err := reader.Regions(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("forbidden = %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<not")
	}))
	t.Cleanup(bad.Close)
	if _, err := testReaderAt(t, bad).Addresses(context.Background(), "us-east-1"); err == nil {
		t.Fatal("expected parse error")
	}

	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<DescribeVolumesResponse><nextToken>again</nextToken></DescribeVolumesResponse>`)
	}))
	t.Cleanup(loop.Close)
	if _, err := testReaderAt(t, loop).Volumes(context.Background(), "us-east-1"); err == nil || !strings.Contains(err.Error(), "20 pages") {
		t.Fatalf("page cap = %v", err)
	}

	elbLoop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<DescribeLoadBalancersResponse><DescribeLoadBalancersResult><NextMarker>again</NextMarker></DescribeLoadBalancersResult></DescribeLoadBalancersResponse>`)
	}))
	t.Cleanup(elbLoop.Close)
	if _, err := testReaderAt(t, elbLoop).LoadBalancers(context.Background(), "us-east-1"); err == nil || !strings.Contains(err.Error(), "20 pages") {
		t.Fatalf("elb page cap = %v", err)
	}

	long := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, strings.Repeat("x", 600))
	}))
	t.Cleanup(long.Close)
	if _, err := testReaderAt(t, long).Instances(context.Background(), "us-east-1"); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("long error = %v", err)
	}
}

func testReaderAt(t *testing.T, srv *httptest.Server) *Reader {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Reader{
		AccessKey: "AKIATEST",
		SecretKey: "secret",
		Token:     "session",
		Region:    "us-east-1",
		HTTP:      &http.Client{Transport: redirectTransport{target: target}},
		Now:       func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}
