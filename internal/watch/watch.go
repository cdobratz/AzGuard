package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const DefaultInterval = 15 * time.Minute

// Source polls one provider for billable creates in a time window.
type Source interface {
	Poll(ctx context.Context, since, until time.Time) ([]Alert, error)
}

// Options controls one watch loop.
type Options struct {
	Interval  time.Duration
	Lookback  time.Duration
	Once      bool
	Webhook   string
	Desktop   bool
	StatePath string
	Sources   []Source
	Now       func() time.Time
	Notify    func(title, body string) error
	Post      func(ctx context.Context, payload Payload) error
	Out       io.Writer
	Err       io.Writer
}

type stateFile struct {
	Seen []string `json:"seen"`
}

// Run polls until ctx is cancelled. With Once, it polls a single time.
// The default interval is 15 minutes. Provider logs add more delay, so this is not a live stream.
func Run(ctx context.Context, opts Options) error {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Lookback <= 0 {
		opts.Lookback = opts.Interval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Notify == nil {
		opts.Notify = NotifyDesktop
	}
	if opts.Post == nil {
		opts.Post = func(ctx context.Context, payload Payload) error {
			return Post(ctx, opts.Webhook, payload, nil)
		}
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}
	window := opts.Lookback
	if opts.Interval > window {
		window = opts.Interval
	}
	fmt.Fprintf(opts.Out, "azguard watch is polling every %s (lookback %s).\n", opts.Interval, window)
	fmt.Fprintln(opts.Out, "Azure Activity Log and CloudTrail LookupEvents are delayed by the providers. This is not a live stream.")

	poll := func() error {
		until := opts.Now()
		since := until.Add(-window)
		seen := loadState(opts.StatePath)
		var fresh []Alert
		var errs []string
		for _, src := range opts.Sources {
			alerts, err := src.Poll(ctx, since, until)
			if err != nil {
				errs = append(errs, err.Error())
			}
			for _, a := range alerts {
				if a.ID == "" || seen.has(a.ID) {
					continue
				}
				fresh = append(fresh, a)
			}
		}
		for _, a := range fresh {
			fmt.Fprintln(opts.Out, FormatLine(a))
			if opts.Desktop {
				title := "azguard: billable resource"
				if a.Automation {
					title = "azguard: automation created a billable resource"
				}
				if err := opts.Notify(title, FormatLine(a)); err != nil {
					fmt.Fprintf(opts.Err, "desktop notification: %v\n", err)
				}
			}
		}
		if opts.Webhook != "" && len(fresh) > 0 {
			if err := opts.Post(ctx, BuildPayload(fresh)); err != nil {
				return err
			}
		}
		for _, a := range fresh {
			seen.add(a.ID)
		}
		if err := saveState(opts.StatePath, seen); err != nil {
			return err
		}
		if len(errs) > 0 && len(fresh) == 0 {
			return fmt.Errorf("%s", errs[0])
		}
		for _, e := range errs {
			fmt.Fprintf(opts.Err, "poll: %s\n", e)
		}
		return nil
	}

	if err := poll(); err != nil {
		return err
	}
	if opts.Once {
		return nil
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := poll(); err != nil {
				fmt.Fprintf(opts.Err, "poll: %v\n", err)
			}
		}
	}
}

type seenSet struct {
	ids []string
	set map[string]bool
}

func (s *seenSet) has(id string) bool {
	if s == nil || s.set == nil {
		return false
	}
	return s.set[id]
}

func (s *seenSet) add(id string) {
	if s.set == nil {
		s.set = map[string]bool{}
	}
	if s.set[id] {
		return
	}
	s.set[id] = true
	s.ids = append(s.ids, id)
	if len(s.ids) > 1000 {
		drop := s.ids[0]
		s.ids = s.ids[1:]
		delete(s.set, drop)
	}
}

func loadState(path string) *seenSet {
	s := &seenSet{set: map[string]bool{}}
	if path == "" {
		return s
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var file stateFile
	if err := json.Unmarshal(body, &file); err != nil {
		return s
	}
	for _, id := range file.Seen {
		s.add(id)
	}
	return s
}

func saveState(path string, s *seenSet) error {
	if path == "" {
		return nil
	}
	body, err := json.MarshalIndent(stateFile{Seen: s.ids}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
