package azure

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAzureCreditLots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.Contains(r.URL.Path, "/lots"):
			_, _ = io.WriteString(w, `{
				"value": [
					{"properties":{"status":"Active","expirationDate":"2026-12-31T00:00:00Z","creditCurrency":"USD","closedBalance":{"currency":"USD","value":40}}},
					{"properties":{"status":"Active","expirationDate":"2026-12-01","closedBalance":20}},
					{"properties":{"status":"Expired","expirationDate":"2026-01-01T00:00:00Z","closedBalance":{"value":100}}},
					{"properties":{"status":"","closedBalance":0}}
				]
			}`)
		case strings.Contains(r.URL.Path, "/billingProfiles"):
			_, _ = io.WriteString(w, `{"value":[{"name":"profile-1"},{"name":""}]}`)
		default:
			_, _ = io.WriteString(w, `{"value":[{"name":"account-1"}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	client := NewCreditClient(func() (string, error) { return "token", nil })
	client.BaseURL = srv.URL
	client.HTTP = srv.Client()
	bal, err := client.Balance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bal.Remaining != 60 || bal.Lots != 2 || bal.Currency != "USD" || !bal.Soonest || bal.ExpiresText() != "2026-12-01" {
		t.Fatalf("balance %+v", bal)
	}
}

func TestAzureCreditFailures(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"value":[]}`)
	}))
	t.Cleanup(empty.Close)
	client := NewCreditClient(func() (string, error) { return "token", nil })
	client.BaseURL = empty.URL
	client.HTTP = empty.Client()
	if _, err := client.Balance(context.Background()); err == nil {
		t.Fatal("expected no billing account")
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	t.Cleanup(denied.Close)
	client.BaseURL = denied.URL
	client.HTTP = denied.Client()
	if _, err := client.Balance(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("denied %v", err)
	}

	if _, err := NewCreditClient(nil).Balance(context.Background()); err == nil {
		t.Fatal("expected missing token")
	}
	broken := NewCreditClient(func() (string, error) { return "", errors.New("no token") })
	if _, err := broken.Balance(context.Background()); err == nil {
		t.Fatal("expected token error")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(bad.Close)
	client.BaseURL = bad.URL
	client.HTTP = bad.Client()
	client.Token = func() (string, error) { return "token", nil }
	if _, err := client.Balance(context.Background()); err == nil {
		t.Fatal("expected bad json")
	}
}
