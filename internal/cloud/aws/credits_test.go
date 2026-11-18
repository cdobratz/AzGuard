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

func TestBillingCreditsFromFixture(t *testing.T) {
	var sawAccount, sawCredits bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("unsigned request")
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "GetCallerIdentity") {
			sawAccount = true
			_, _ = io.WriteString(w, `<GetCallerIdentityResponse><GetCallerIdentityResult><Account>123456789012</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`)
			return
		}
		if r.Header.Get("X-Amz-Target") != "AWSBilling.GetCredits" {
			t.Errorf("target %s", r.Header.Get("X-Amz-Target"))
		}
		if !strings.Contains(string(raw), "123456789012") {
			t.Errorf("payload %s", raw)
		}
		sawCredits = true
		_, _ = io.WriteString(w, `{
			"credits": [
				{"remainingAmount":{"currencyAmount":"40.50","currencyCode":"USD"},"endDate":1798675200},
				{"remainingAmount":{"currencyAmount":"9.50","currencyCode":"USD"},"endDate":1796083200},
				{"remainingAmount":{"currencyAmount":"0","currencyCode":"USD"},"endDate":1790000000},
				{"remainingAmount":{"currencyAmount":"","currencyCode":"USD"}}
			]
		}`)
	}))
	t.Cleanup(srv.Close)
	client := BillingClient{
		AccessKey:       "AKIATEST",
		SecretKey:       "secret",
		BillingEndpoint: srv.URL,
		STSEndpoint:     srv.URL,
		HTTP:            srv.Client(),
		Now:             func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) },
	}
	bal, err := client.RemainingCredit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sawAccount || !sawCredits || bal.Remaining != 50 || bal.Count != 2 || bal.Currency != "USD" || !bal.Soonest {
		t.Fatalf("balance %+v", bal)
	}
	if bal.ExpiresText() == "" {
		t.Fatal("missing expiry")
	}
}

func TestParseCreditsAndAccountErrors(t *testing.T) {
	if _, err := ParseAccountID([]byte(`<GetCallerIdentityResponse><GetCallerIdentityResult><Account>123</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`)); err == nil {
		t.Fatal("expected short account")
	}
	if _, err := ParseAccountID([]byte("<")); err == nil {
		t.Fatal("expected bad xml")
	}
	id, err := ParseAccountID([]byte(`<GetCallerIdentityResponse><GetCallerIdentityResult><Account>123456789012</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`))
	if err != nil || id != "123456789012" {
		t.Fatalf("id %s err=%v", id, err)
	}
	bal, err := ParseCredits([]byte(`{"credits":[{"remainingAmount":{"currencyAmount":"5","currencyCode":"USD"},"endDate":"1796083200"}]}`))
	if err != nil || bal.Remaining != 5 || bal.Expires.IsZero() {
		t.Fatalf("string epoch %+v err=%v", bal, err)
	}
	if _, err := ParseCredits([]byte("{")); err == nil {
		t.Fatal("expected bad json")
	}
	if _, err := ParseCredits([]byte(`{"credits":[{"remainingAmount":{"currencyAmount":"nope"}}]}`)); err == nil {
		t.Fatal("expected bad amount")
	}
	empty := BillingClient{}
	if _, err := empty.AccountID(context.Background()); err == nil {
		t.Fatal("expected missing credentials")
	}
	if _, err := empty.GetCredits(context.Background(), "123456789012"); err == nil {
		t.Fatal("expected missing credentials")
	}
	client := BillingClient{AccessKey: "AKIATEST", SecretKey: "secret"}
	if _, err := client.GetCredits(context.Background(), "12"); err == nil {
		t.Fatal("expected bad account id")
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	t.Cleanup(denied.Close)
	client.STSEndpoint = denied.URL
	client.HTTP = denied.Client()
	if _, err := client.RemainingCredit(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("denied %v", err)
	}
	fromNil := BillingFromCost(nil)
	if fromNil.HTTP == nil || fromNil.enabled() {
		t.Fatal("nil cost client")
	}
	from := BillingFromCost(&CostClient{AccessKey: "AKIATEST", SecretKey: "secret", Token: "tok"})
	if !from.enabled() || from.Token != "tok" {
		t.Fatalf("from cost %+v", from)
	}
}
