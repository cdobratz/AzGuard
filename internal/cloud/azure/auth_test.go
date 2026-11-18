package azure

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSubscriptionID(t *testing.T) {
	if err := ValidateSubscriptionID(""); err == nil {
		t.Fatal("empty")
	}
	for _, placeholder := range []string{"providers", "YOUR_SUB_ID", "subscription-id"} {
		if err := ValidateSubscriptionID(placeholder); err == nil {
			t.Fatalf("placeholder %s", placeholder)
		}
	}
	if err := ValidateSubscriptionID("not-a-guid"); err == nil {
		t.Fatal("bad guid")
	}
	if err := ValidateSubscriptionID("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
}

func TestNewTokenProvider(t *testing.T) {
	cli, err := NewTokenProvider("cli", nil)
	if err != nil || cli == nil {
		t.Fatal(err)
	}
	mi, err := NewTokenProvider("managed_identity", nil)
	if err != nil || mi == nil {
		t.Fatal(err)
	}
	sp, err := NewTokenProvider("service_principal", map[string]string{
		"tenant_id":     "tenant",
		"client_id":     "client",
		"client_secret": "secret",
	})
	if err != nil || sp == nil {
		t.Fatal(err)
	}
	if _, err := NewTokenProvider("nope", nil); err == nil {
		t.Fatal("expected unknown method")
	}
}

func TestTokenHelpersUseFixtures(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "az")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"accessToken\":\"cli-token\",\"id\":\"22222222-2222-2222-2222-222222222222\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	token, err := GetCLIToken()
	if err != nil || token != "cli-token" {
		t.Fatalf("cli token %q err=%v", token, err)
	}
	sub, err := GetSubscriptionIDFromCLI()
	if err != nil || sub != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("sub %q err=%v", sub, err)
	}

	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := GetCLIToken(); err == nil {
		t.Fatal("expected bad cli json")
	}
	if _, err := GetSubscriptionIDFromCLI(); err == nil {
		t.Fatal("expected bad subscription json")
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"id\":\"\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSubscriptionIDFromCLI(); err == nil {
		t.Fatal("expected empty subscription")
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := GetCLIToken(); err == nil {
		t.Fatal("expected cli failure")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "client_credentials") {
			if r.Header.Get("Content-Type") == "" {
				t.Error("missing content type")
			}
			_, _ = io.WriteString(w, `{"access_token":"sp-token"}`)
			return
		}
		if r.Header.Get("Metadata") != "true" {
			t.Error("missing metadata header")
		}
		if !strings.Contains(r.URL.RawQuery, "client_id=app") {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"access_token":"mi-token"}`)
	}))
	t.Cleanup(srv.Close)
	orig := identityEndpoint
	identityEndpoint = srv.URL
	t.Cleanup(func() { identityEndpoint = orig })
	sp, err := GetSPToken("tenant", "client", "secret")
	if err != nil || sp != "sp-token" {
		t.Fatalf("sp %q err=%v", sp, err)
	}
	provider, err := NewTokenProvider("service_principal", map[string]string{
		"tenant_id": "tenant", "client_id": "client", "client_secret": "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := provider(); err != nil || tok != "sp-token" {
		t.Fatalf("provider %q err=%v", tok, err)
	}

	t.Setenv("MSI_ENDPOINT", srv.URL)
	t.Setenv("MSI_CLIENT_ID", "app")
	mi, err := GetMIToken()
	if err != nil || mi != "mi-token" {
		t.Fatalf("mi %q err=%v", mi, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(bad.Close)
	identityEndpoint = bad.URL
	if _, err := GetSPToken("tenant", "client", "secret"); err == nil {
		t.Fatal("expected sp status error")
	}
	t.Setenv("MSI_ENDPOINT", bad.URL)
	if _, err := GetMIToken(); err == nil {
		t.Fatal("expected mi status error")
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{")
	}))
	t.Cleanup(broken.Close)
	identityEndpoint = broken.URL
	if _, err := GetSPToken("tenant", "client", "secret"); err == nil {
		t.Fatal("expected sp json error")
	}
	t.Setenv("MSI_ENDPOINT", broken.URL)
	t.Setenv("MSI_CLIENT_ID", "")
	if _, err := GetMIToken(); err == nil {
		t.Fatal("expected mi json error")
	}
}
