package aws

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	exampleAccessKey = "AKIDEXAMPLE"
	exampleSecret    = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	// Signature for POST / payload {} to the free tier host at 2015-08-30T12:36:00Z.
	// Canonical headers are content-type, host, x-amz-date, and x-amz-target.
	exampleFreeTierSignature = "6349819c782bebe3a2e1de58190fcb2b3034ad7a7f53a1dc21cbecf20ae1e027"
)

func TestSignV4MatchesKnownVector(t *testing.T) {
	req := freeTierRequest(t)
	when := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	signV4(req, []byte("{}"), exampleAccessKey, exampleSecret, "", "us-east-1", "freetier", when)

	auth := req.Header.Get("Authorization")
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/freetier/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=" + exampleFreeTierSignature
	if auth != want {
		t.Fatalf("authorization\n got %s\nwant %s", auth, want)
	}
	if req.Header.Get("X-Amz-Date") != "20150830T123600Z" {
		t.Fatalf("date %s", req.Header.Get("X-Amz-Date"))
	}
	if req.Header.Get("Host") != "freetier.us-east-1.amazonaws.com" {
		t.Fatalf("host %s", req.Header.Get("Host"))
	}
}

func TestSignV4TokenAndEmptyPath(t *testing.T) {
	req := freeTierRequest(t)
	req.URL.Path = ""
	req.URL.RawQuery = "limit=1"
	when := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	signV4(req, []byte("{}"), exampleAccessKey, exampleSecret, "session-token", "us-east-1", "freetier", when)
	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token;x-amz-target") {
		t.Fatalf("signed headers: %s", auth)
	}
	if strings.Contains(auth, exampleFreeTierSignature) {
		t.Fatal("session token should change the signature")
	}
	if req.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Fatalf("token header %s", req.Header.Get("X-Amz-Security-Token"))
	}
}

func TestCostClientSignRequestMatchesSignV4(t *testing.T) {
	req := freeTierRequest(t)
	client := &CostClient{
		AccessKey: exampleAccessKey,
		SecretKey: exampleSecret,
		Region:    "us-east-1",
		Now:       func() time.Time { return time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC) },
	}
	client.signRequest(req, []byte("{}"))
	if !strings.Contains(req.Header.Get("Authorization"), "Signature="+exampleFreeTierSignature) {
		t.Fatalf("cost client signature: %s", req.Header.Get("Authorization"))
	}

	withToken := freeTierRequest(t)
	client.Token = "session-token"
	client.signRequest(withToken, []byte("{}"))
	if !strings.Contains(withToken.Header.Get("Authorization"), "x-amz-security-token") {
		t.Fatalf("token signature: %s", withToken.Header.Get("Authorization"))
	}

	ce := freeTierRequest(t)
	ce.URL.Host = "ce.us-east-1.amazonaws.com"
	client.Token = ""
	client.signRequest(ce, []byte("{}"))
	if !strings.Contains(ce.Header.Get("Authorization"), "/us-east-1/ce/aws4_request") {
		t.Fatalf("ce scope: %s", ce.Header.Get("Authorization"))
	}
}

func freeTierRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://freetier.us-east-1.amazonaws.com/", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSFreeTierService.GetFreeTierUsage")
	return req
}
