package guard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one command the guard allowed or blocked during a session.
type Entry struct {
	At                  time.Time `json:"at"`
	Command             string    `json:"command"`
	EstimatedMonthlyUSD float64   `json:"estimated_monthly_usd"`
	Priced              bool      `json:"priced"`
	Allowed             bool      `json:"allowed"`
}

// Session is the on-disk budget for one guard session.
type Session struct {
	StartedAt time.Time `json:"started_at"`
	BudgetUSD float64   `json:"budget_usd"`
	SpentUSD  float64   `json:"spent_usd"`
	Entries   []Entry   `json:"entries"`
}

func loadSession(path string, budget float64, now time.Time) (Session, error) {
	if path == "" {
		return Session{StartedAt: now, BudgetUSD: budget}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Session{StartedAt: now, BudgetUSD: budget}, nil
		}
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(body, &s); err != nil {
		return Session{}, fmt.Errorf("read guard session: %w", err)
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = now
	}
	s.BudgetUSD = budget
	return s, nil
}

func saveSession(path string, s Session) error {
	if path == "" {
		return nil
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
