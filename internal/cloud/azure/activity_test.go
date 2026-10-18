package azure

import (
	"strings"
	"testing"
	"time"
)

func TestActivityPathIsReadOnly(t *testing.T) {
	r := NewReader("00000000-0000-0000-0000-000000000001", nil)
	since := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	until := since.Add(15 * time.Minute)
	path := r.ActivityPath(since, until)
	if !strings.Contains(path, "/providers/Microsoft.Insights/eventtypes/management/values") {
		t.Fatalf("path = %s", path)
	}
	if !strings.Contains(path, "api-version=2015-04-01") {
		t.Fatalf("path = %s", path)
	}
	if !strings.Contains(path, "eventTimestamp") {
		t.Fatalf("path = %s", path)
	}
}
