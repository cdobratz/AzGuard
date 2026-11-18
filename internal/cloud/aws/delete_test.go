package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeleteVolumeRequest(t *testing.T) {
	const secret = "super-secret-key"
	var body, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		method = r.Method
		if strings.Contains(body, secret) || strings.Contains(r.Header.Get("Authorization"), secret) {
			t.Errorf("secret leaked in the request")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<DeleteVolumeResponse></DeleteVolumeResponse>`)
	}))
	t.Cleanup(srv.Close)

	reader := &Reader{
		AccessKey:   "AKIAEXAMPLE",
		SecretKey:   secret,
		Region:      "us-east-1",
		HTTP:        srv.Client(),
		EC2Endpoint: srv.URL,
		Now:         func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
	if err := reader.DeleteVolume(context.Background(), "us-east-1", "vol-123"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost {
		t.Fatalf("method %s", method)
	}
	if !strings.Contains(body, "Action=DeleteVolume") || !strings.Contains(body, "VolumeId=vol-123") {
		t.Fatalf("body %s", body)
	}
}

func TestDeleteVolumeNotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `<Response><Errors><Error><Code>InvalidVolume.NotFound</Code><Message>gone</Message></Error></Errors></Response>`)
	}))
	t.Cleanup(srv.Close)
	reader := testReader(srv)
	if err := reader.DeleteVolume(context.Background(), "us-east-1", "vol-gone"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteVolumeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Response><Errors><Error><Code>UnauthorizedOperation</Code></Error></Errors></Response>`)
	}))
	t.Cleanup(srv.Close)
	reader := testReader(srv)
	err := reader.DeleteVolume(context.Background(), "us-east-1", "vol-123")
	if err == nil || !strings.Contains(err.Error(), "UnauthorizedOperation") {
		t.Fatalf("err = %v", err)
	}
}

func TestReleaseAddressUsesAllocationID(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<ReleaseAddressResponse></ReleaseAddressResponse>`)
	}))
	t.Cleanup(srv.Close)
	reader := testReader(srv)
	if err := reader.ReleaseAddress(context.Background(), "us-east-1", "eipalloc-abc"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Action=ReleaseAddress") || !strings.Contains(body, "AllocationId=eipalloc-abc") {
		t.Fatalf("body %s", body)
	}
}

func TestLoadBalancerTags(t *testing.T) {
	const arn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/web/abc"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "Action=DescribeTags") || !strings.Contains(string(raw), "ResourceArns.member.1=") {
			t.Fatalf("body %s", raw)
		}
		_, _ = io.WriteString(w, `<DescribeTagsResponse>
		  <DescribeTagsResult>
		    <TagDescriptions>
		      <member>
		        <ResourceArn>`+arn+`</ResourceArn>
		        <Tags><member><Key>azguard:keep</Key><Value>true</Value></member></Tags>
		      </member>
		    </TagDescriptions>
		  </DescribeTagsResult>
		</DescribeTagsResponse>`)
	}))
	t.Cleanup(srv.Close)
	reader := testReader(srv)
	reader.ELBEndpoint = srv.URL
	got, err := reader.LoadBalancerTags(context.Background(), "us-east-1", []string{arn, arn})
	if err != nil {
		t.Fatal(err)
	}
	if got[arn]["azguard:keep"] != "true" {
		t.Fatalf("tags %+v", got)
	}
}

func testReader(srv *httptest.Server) *Reader {
	return &Reader{
		AccessKey:   "AKIAEXAMPLE",
		SecretKey:   "secret",
		Region:      "us-east-1",
		HTTP:        srv.Client(),
		EC2Endpoint: srv.URL,
		Now:         func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}
