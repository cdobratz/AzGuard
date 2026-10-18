package main

import (
	"strings"
	"testing"
	"time"

	"github.com/azguard/azguard/internal/watch"
)

func TestWatchCommandDefaults(t *testing.T) {
	cmd := watchCmd()
	if cmd.Flags().Lookup("interval").DefValue != watch.DefaultInterval.String() {
		t.Fatalf("interval default = %s", cmd.Flags().Lookup("interval").DefValue)
	}
	if watch.DefaultInterval != 15*time.Minute {
		t.Fatalf("default interval = %s", watch.DefaultInterval)
	}
	lower := strings.ToLower(cmd.Long + cmd.Short)
	if strings.Contains(lower, "real-time") || strings.Contains(lower, "realtime") {
		t.Fatal("watch help promises a live stream")
	}
	if !strings.Contains(cmd.Long, "not a live stream") {
		t.Fatal("watch help should say this is not a live stream")
	}
	if !strings.Contains(cmd.Long, "cloudtrail:LookupEvents") {
		t.Fatal("watch help should list the CloudTrail read permission")
	}
}
