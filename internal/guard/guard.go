package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/iac"
	"github.com/cdobratz/azguard/internal/pricing"
)

// Options controls one guard decision.
type Options struct {
	BudgetUSD float64
	Path      string
	Now       func() time.Time
	// Exec runs the wrapped command. Tests replace it. Nil uses os/exec.
	Exec func(argv []string) error
	// Plan is a terraform show -json plan, Pulumi preview JSON, or CDK/CloudFormation template.
	Plan []byte
}

// Decision is what the hook or wrapper should do with one command.
type Decision struct {
	Allow    bool
	Billable bool
	Priced   bool
	USD      float64
	Message  string
}

// BlockedError means the wrapper must not run the command.
type BlockedError struct {
	Message string
}

func (e *BlockedError) Error() string { return e.Message }

// ExitCode is 2 so a wrapper can fail a script when the budget blocks a command.
func (e *BlockedError) ExitCode() int { return 2 }

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string { return e.err.Error() }
func (e *exitCodeError) ExitCode() int { return e.code }

// Decide reports whether command may run under a session budget.
// budget <= 0 warns and allows. A known estimate blocks only when spent+estimate is greater than budget.
func Decide(command string, spent, budget float64) Decision {
	d, _ := DecidePlan(command, spent, budget, nil)
	return d
}

// DecidePlan is Decide with an optional IaC plan. A terraform apply, pulumi up,
// or cdk deploy uses the plan's bundled monthly total when every resource in it has a price.
func DecidePlan(command string, spent, budget float64, plan []byte) (Decision, error) {
	c, err := classification(command, plan)
	if err != nil {
		return Decision{}, err
	}
	return decideClass(c, spent, budget), nil
}

func classification(command string, plan []byte) (Classification, error) {
	c := Classify(command)
	if len(bytes.TrimSpace(plan)) == 0 || !IsIaC(command) {
		return c, nil
	}
	est, err := iac.Parse(plan, pricing.Bundled())
	if err != nil {
		return Classification{}, err
	}
	notes := dropIaCNotes(c.Summary)
	onlyIaC := len(notes) == 0
	c.Summary = strings.Join(notes, "; ")
	if !est.Complete {
		c.Billable = true
		c.Priced = false
		c.Summary = joinNonEmpty(c.Summary, "plan has resources without a bundled price")
		return c, nil
	}
	c.Billable = true
	c.USD = pricing.RoundUSD(c.USD + est.MonthlyUSD)
	c.Summary = joinNonEmpty(c.Summary, est.Format+" plan")
	if onlyIaC {
		c.Priced = true
	}
	return c, nil
}

func dropIaCNotes(summary string) []string {
	if summary == "" {
		return nil
	}
	var notes []string
	for _, part := range strings.Split(summary, "; ") {
		switch part {
		case "terraform apply", "pulumi up", "cdk deploy":
			continue
		default:
			notes = append(notes, part)
		}
	}
	return notes
}

func joinNonEmpty(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "; " + b
}

func decideClass(c Classification, spent, budget float64) Decision {
	if !c.Billable {
		return Decision{Allow: true, Message: "read-only command"}
	}
	d := Decision{Allow: true, Billable: true, Priced: c.Priced, USD: c.USD}
	label := c.Summary
	if label == "" {
		label = "billable command"
	}
	if !c.Priced {
		d.Message = fmt.Sprintf("allowed: %s has no bundled monthly estimate, so the session budget was not applied", label)
		return d
	}
	if budget <= 0 {
		d.Message = fmt.Sprintf("estimated $%.2f/month for %s; no session budget is set (pass --budget to block)", c.USD, label)
		return d
	}
	if spent+c.USD > budget {
		d.Allow = false
		d.Message = fmt.Sprintf("blocked: %s is estimated at $%.2f/month, and this session is already at $%.2f of a $%.2f session budget", label, c.USD, spent, budget)
		return d
	}
	d.Message = fmt.Sprintf("allowed: %s is estimated at $%.2f/month; session would be $%.2f of $%.2f", label, c.USD, spent+c.USD, budget)
	return d
}

// RunCommand evaluates argv and execs it only when the decision allows it.
func RunCommand(argv []string, opts Options) error {
	if len(argv) == 0 {
		return fmt.Errorf("guard: missing command after --")
	}
	command := strings.Join(argv, " ")
	decision, err := record(command, opts)
	if err != nil {
		return err
	}
	if !decision.Allow {
		return &BlockedError{Message: decision.Message}
	}
	execFn := opts.Exec
	if execFn == nil {
		execFn = defaultExec
	}
	return execFn(argv)
}

func defaultExec(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &exitCodeError{code: exitErr.ExitCode(), err: err}
	}
	return err
}

type hookInput struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
}

// RunHook reads a Claude Code PreToolUse payload from r and writes the decision JSON to w.
// It exits through a nil error even when the decision is deny, so the host reads the JSON.
func RunHook(r io.Reader, w io.Writer, opts Options) error {
	var in hookInput
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return fmt.Errorf("guard hook: read stdin: %w", err)
	}
	if !isShellTool(in.ToolName) {
		return writeHook(w, Decision{Allow: true, Message: "tool is not a shell command"})
	}
	command, _ := in.ToolInput["command"].(string)
	decision, err := record(command, opts)
	if err != nil {
		return err
	}
	return writeHook(w, decision)
}

func isShellTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "bash", "shell", "powershell":
		return true
	default:
		return false
	}
}

func writeHook(w io.Writer, d Decision) error {
	decision := "allow"
	if !d.Allow {
		decision = "deny"
	}
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       decision,
			"permissionDecisionReason": d.Message,
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func record(command string, opts Options) (Decision, error) {
	now := time.Now()
	if opts.Now != nil {
		now = opts.Now()
	}
	session, err := loadSession(opts.Path, opts.BudgetUSD, now)
	if err != nil {
		return Decision{}, err
	}
	decision, err := DecidePlan(command, session.SpentUSD, opts.BudgetUSD, opts.Plan)
	if err != nil {
		return Decision{}, err
	}
	if decision.Billable {
		if decision.Allow && decision.Priced {
			session.SpentUSD = pricing.RoundUSD(session.SpentUSD + decision.USD)
		}
		session.Entries = append(session.Entries, Entry{
			At:                  now,
			Command:             command,
			EstimatedMonthlyUSD: decision.USD,
			Priced:              decision.Priced,
			Allowed:             decision.Allow,
		})
		if err := saveSession(opts.Path, session); err != nil {
			return Decision{}, err
		}
	}
	return decision, nil
}
