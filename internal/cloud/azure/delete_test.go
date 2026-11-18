package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeleteResource(t *testing.T) {
	const (
		sub   = "00000000-0000-0000-0000-000000000000"
		token = "token-not-logged"
	)
	id := "/subscriptions/" + sub + "/resourceGroups/rg/providers/Microsoft.Compute/disks/orphan"
	var method, path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.RequestURI()
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	reader := NewReader(sub, func() (string, error) { return token, nil })
	reader.BaseURL = srv.URL
	reader.HTTP = srv.Client()
	if err := reader.DeleteResource(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete {
		t.Fatalf("method %s", method)
	}
	if !strings.Contains(path, "/providers/Microsoft.Compute/disks/orphan") || !strings.Contains(path, "api-version=2024-03-02") {
		t.Fatalf("path %s", path)
	}
	if auth != "Bearer "+token {
		t.Fatalf("auth header was not the bearer token")
	}
}

func TestDeleteResourceGuards(t *testing.T) {
	const sub = "00000000-0000-0000-0000-000000000000"
	reader := NewReader(sub, func() (string, error) { return "token-not-logged", nil })
	reader.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("refused delete must not call Azure")
		return nil, nil
	})}
	outside := "/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/rg/providers/Microsoft.Compute/disks/x"
	if err := reader.DeleteResource(context.Background(), outside); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside: %v", err)
	}
	other := "/subscriptions/" + sub + "/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/x"
	if err := reader.DeleteResource(context.Background(), other); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("type: %v", err)
	}
}

func TestDeleteResourceNotFoundAndForbidden(t *testing.T) {
	const (
		sub   = "00000000-0000-0000-0000-000000000000"
		token = "token-not-logged"
	)
	id := "/subscriptions/" + sub + "/resourceGroups/rg/providers/Microsoft.Network/publicIPAddresses/pip"
	status := http.StatusNotFound
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":{"code":"ResourceNotFound"}}`)
	}))
	t.Cleanup(srv.Close)
	reader := NewReader(sub, func() (string, error) { return token, nil })
	reader.BaseURL = srv.URL
	reader.HTTP = srv.Client()
	if err := reader.DeleteResource(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	status = http.StatusForbidden
	err := reader.DeleteResource(context.Background(), id)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("err = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
