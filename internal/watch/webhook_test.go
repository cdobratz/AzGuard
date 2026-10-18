package watch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookPayloadAndPost(t *testing.T) {
	alerts := []Alert{
		{
			Provider:            "azure",
			Operation:           "Microsoft.Network/publicIPAddresses/write",
			ResourceID:          "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/publicIPAddresses/pip1",
			Principal:           "11111111-2222-3333-4444-555555555555",
			Automation:          true,
			AutomationReason:    "service principal (idtyp=app)",
			EstimatedMonthlyUSD: 3.65,
		},
	}
	payload := BuildPayload(alerts)
	if payload.Text == "" || payload.Text != payload.Content {
		t.Fatalf("text and content = %q / %q", payload.Text, payload.Content)
	}
	if !strings.Contains(payload.Text, "billable resource created") {
		t.Fatalf("text = %q", payload.Text)
	}
	if payload.Username != "azguard" {
		t.Fatalf("username = %s", payload.Username)
	}
	if len(payload.Attachments) != 1 || payload.Attachments[0].Color != "#e67e22" {
		t.Fatalf("attachments = %+v", payload.Attachments)
	}
	if !strings.Contains(WebhookSchema, `"text"`) || !strings.Contains(WebhookSchema, `"content"`) {
		t.Fatal("webhook schema should document text and content")
	}

	var got Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := Post(context.Background(), srv.URL, payload, srv.Client()); err != nil {
		t.Fatal(err)
	}
	if got.Text != payload.Text || got.Content != payload.Content {
		t.Fatalf("posted = %+v", got)
	}
}

func TestWebhookNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	err := Post(context.Background(), srv.URL, Payload{Text: "hi", Content: "hi"}, srv.Client())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
}

func TestWebhookHumanColor(t *testing.T) {
	payload := BuildPayload([]Alert{{Provider: "aws", Operation: "RunInstances", Principal: "casey"}})
	if payload.Attachments[0].Color != "#3498db" {
		t.Fatalf("color = %s", payload.Attachments[0].Color)
	}
}
