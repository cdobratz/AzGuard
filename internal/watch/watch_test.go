package watch

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	alerts []Alert
	err    error
	calls  int
}

func (f *fakeSource) Poll(ctx context.Context, since, until time.Time) ([]Alert, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.alerts, nil
}

func TestRunOnceDedupesAndPosts(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "watch.json")
	src := &fakeSource{alerts: []Alert{{
		ID:                  "evt-1",
		Provider:            "aws",
		Operation:           "AllocateAddress",
		ResourceID:          "eipalloc-1",
		Principal:           "ci-bot",
		Automation:          true,
		AutomationReason:    "IAM user access key",
		EstimatedMonthlyUSD: 3.65,
		At:                  time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC),
	}}}
	var posts int
	var out, errBuf bytes.Buffer
	opts := Options{
		Interval:  time.Minute,
		Once:      true,
		Webhook:   "https://example.invalid/hook",
		Desktop:   true,
		StatePath: state,
		Sources:   []Source{src},
		Now:       func() time.Time { return time.Date(2026, 10, 17, 16, 0, 0, 0, time.UTC) },
		Notify:    func(title, body string) error { return errors.New("no display") },
		Post: func(ctx context.Context, payload Payload) error {
			posts++
			if payload.Text == "" || payload.Content != payload.Text {
				t.Fatalf("payload text/content = %q %q", payload.Text, payload.Content)
			}
			return nil
		},
		Out: &out,
		Err: &errBuf,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("posts = %d", posts)
	}
	if !strings.Contains(out.String(), "not a live stream") {
		t.Fatalf("output = %s", out.String())
	}
	if !strings.Contains(out.String(), "eipalloc-1") {
		t.Fatalf("output = %s", out.String())
	}
	if !strings.Contains(errBuf.String(), "desktop notification") {
		t.Fatalf("stderr = %s", errBuf.String())
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("second poll posted again: %d", posts)
	}
	body, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "evt-1") {
		t.Fatalf("state = %s", body)
	}
}

func TestRunDoesNotMarkSeenWhenWebhookFails(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "nested", "watch.json")
	src := &fakeSource{alerts: []Alert{{ID: "evt-2", Provider: "azure", Operation: "write", ResourceID: "rid"}}}
	opts := Options{
		Once:      true,
		Webhook:   "https://example.invalid/hook",
		StatePath: state,
		Sources:   []Source{src},
		Post: func(ctx context.Context, payload Payload) error {
			return errors.New("webhook status 500")
		},
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected webhook error")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state file err = %v", err)
	}
}

func TestDesktopCommands(t *testing.T) {
	name, args := desktopCommand("linux", "title", "body")
	if name != "notify-send" || len(args) != 2 {
		t.Fatalf("linux = %s %v", name, args)
	}
	name, args = desktopCommand("darwin", "title", "body")
	if name != "osascript" || len(args) != 2 {
		t.Fatalf("darwin = %s %v", name, args)
	}
	name, _ = desktopCommand("windows", "title", "body")
	if name != "powershell" {
		t.Fatalf("windows = %s", name)
	}
	name, args = desktopCommand("plan9", "title", "body")
	if name != "" || args != nil {
		t.Fatalf("plan9 = %s %v", name, args)
	}
}
