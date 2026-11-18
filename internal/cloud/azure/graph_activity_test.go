package azure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGraphQueryAndWorkspaceUsage(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("User-Agent") != "azguard" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if strings.Contains(r.URL.Path, "/usages") {
			_, _ = io.WriteString(w, `{"value":[
				{"name":{"value":"DataIngestion"},"unit":"Bytes","currentValue":10},
				{"name":{"value":"DataAnalyzed"},"unit":"GB","currentValue":1},
				{"name":{"value":"IngestionOther"},"unit":"Count","currentValue":2},
				{"name":{"value":"Search"},"unit":"requests","currentValue":9}
			]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		pages++
		if pages == 1 {
			if !strings.Contains(string(raw), "Resources") {
				t.Errorf("query %s", raw)
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"a"}],"$skipToken":"next"}`)
			return
		}
		if !strings.Contains(string(raw), "next") {
			t.Errorf("missing skip token %s", raw)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"b"}],"skipToken":""}`)
	}))
	t.Cleanup(srv.Close)

	reader := NewReader("11111111-1111-1111-1111-111111111111", func() (string, error) { return "token", nil })
	reader.BaseURL = srv.URL
	reader.HTTP = srv.Client()
	rows, err := reader.Query(context.Background(), "Resources")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %+v err=%v", rows, err)
	}
	bytes, err := reader.WorkspaceIngestionBytes(context.Background(), "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws")
	if err != nil || bytes != 10+1024*1024*1024+2 {
		t.Fatalf("bytes %v err=%v", bytes, err)
	}
	if _, err := reader.WorkspaceIngestionBytes(context.Background(), "ws"); err == nil {
		t.Fatal("expected resource id guard")
	}
}

func TestGraphAndActivityFailures(t *testing.T) {
	reader := NewReader("not-a-guid", func() (string, error) { return "t", nil })
	if _, err := reader.Query(context.Background(), "Resources"); err == nil {
		t.Fatal("expected invalid subscription")
	}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	if _, err := reader.ActivityEvents(context.Background(), since, until); err == nil {
		t.Fatal("expected invalid subscription")
	}
	if !strings.Contains(reader.ActivityPath(since, until), "Microsoft.Insights") {
		t.Fatal(reader.ActivityPath(since, until))
	}
	if _, _, err := ParseGraphPage([]byte("{")); err == nil {
		t.Fatal("expected bad graph json")
	}
	if _, err := ParseWorkspaceIngestion([]byte("{")); err == nil {
		t.Fatal("expected bad usage json")
	}
	if _, _, err := ParseActivity([]byte("{")); err == nil {
		t.Fatal("expected bad activity json")
	}

	events, next, err := ParseActivity([]byte(`{"value":[
		{"eventDataId":"e1","eventTimestamp":"2026-11-01T00:00:00.1Z","operationName":{"value":"Microsoft.Compute/virtualMachines/write"},"status":"Succeeded","caller":"app","resourceId":"/subs/vm","claims":{"idtyp":"app","appid":"app-1","http://schemas.xmlsoap.org/ws/2005/05/identity/claims/upn":"casey@contoso.com","name":"Casey"}},
		{"eventTimestamp":"not-a-time","operationName":"Microsoft.Storage/storageAccounts/write","caller":"","resourceId":"/subs/sa","correlationId":"c1"}
	],"nextLink":"https://example.invalid/next"}`))
	if err != nil || next == "" || len(events) != 2 {
		t.Fatalf("events %+v next %s err=%v", events, next, err)
	}
	if events[0].Principal.UPN != "casey@contoso.com" || events[0].Principal.AppID != "app-1" || events[0].Name == "" {
		t.Fatalf("principal %+v name %s", events[0].Principal, events[0].Name)
	}
	if events[1].ID == "" || events[1].Name != "Microsoft.Storage/storageAccounts/write" {
		t.Fatalf("fallback event %+v", events[1])
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("no", 400), http.StatusForbidden)
	}))
	t.Cleanup(denied.Close)
	reader = NewReader("11111111-1111-1111-1111-111111111111", func() (string, error) { return "token", nil })
	reader.BaseURL = denied.URL
	reader.HTTP = denied.Client()
	if _, err := reader.Query(context.Background(), "Resources"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("forbidden = %v", err)
	}
	plain := NewReader("11111111-1111-1111-1111-111111111111", nil)
	if _, err := plain.Query(context.Background(), "Resources"); err == nil {
		t.Fatal("expected missing token")
	}
	plain.Token = func() (string, error) { return "", io.EOF }
	if _, err := plain.Query(context.Background(), "Resources"); err == nil {
		t.Fatal("expected token error")
	}
}

func TestActivityEventsFollowsNextLink(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "page=2") {
			_, _ = io.WriteString(w, `{"value":[{"eventDataId":"e2","eventTimestamp":"2026-11-01T01:00:00Z","operationName":{"value":"write"},"status":{"value":"Failed"}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"value":[{"eventDataId":"e1","eventTimestamp":"2026-11-01T00:00:00Z","operationName":{"value":"write"},"status":{"value":"Succeeded"}}],"nextLink":"`+srv.URL+`/next?page=2"}`)
	}))
	t.Cleanup(srv.Close)
	reader := NewReader("11111111-1111-1111-1111-111111111111", func() (string, error) { return "token", nil })
	reader.BaseURL = srv.URL
	reader.HTTP = srv.Client()
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	events, err := reader.ActivityEvents(context.Background(), since, since.Add(time.Hour))
	if err != nil || len(events) != 2 {
		t.Fatalf("events %+v err=%v", events, err)
	}
}
