package guard

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecideBlocksRunInstancesOverBudget(t *testing.T) {
	d := Decide("aws ec2 run-instances --instance-type t3.micro", 0, 5)
	if d.Allow {
		t.Fatal(d.Message)
	}
	if !strings.Contains(d.Message, "session budget") || !strings.Contains(d.Message, "7.59") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestDecideAllowsAtExactBudget(t *testing.T) {
	d := Decide("aws ec2 run-instances --instance-type t3.micro", 0, 7.59)
	if !d.Allow {
		t.Fatal(d.Message)
	}
}

func TestDecidePassesReadOnly(t *testing.T) {
	for _, command := range []string{
		"aws ec2 describe-instances",
		"aws s3 ls",
		"az group list",
		"az vm show -g rg -n vm1",
		"terraform plan",
		"pulumi preview",
		"cdk diff",
	} {
		d := Decide(command, 100, 1)
		if !d.Allow || d.Billable {
			t.Fatalf("%s: %+v", command, d)
		}
	}
}

func TestDecideBudgetZeroWarns(t *testing.T) {
	d := Decide("aws ec2 run-instances", 0, 0)
	if !d.Allow || !strings.Contains(d.Message, "7.59") || !strings.Contains(d.Message, "--budget") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestDecidePricesKnownCreates(t *testing.T) {
	cases := []struct {
		command string
		usd     float64
	}{
		{"aws ec2 allocate-address", 3.65},
		{"aws ec2 create-nat-gateway", 32.85},
		{"aws ec2 create-volume --volume-type gp3 --size 100", 8},
		{"aws elbv2 create-load-balancer", 16.43},
		{"az network public-ip create -g rg -n pip", 3.65},
		{"az network nat gateway create -g rg -n nat", 32.85},
		{"az vm create -g rg -n vm --size Standard_B1s", 7.59},
		{`bash -lc "aws ec2 run-instances --instance-type t3.micro"`, 7.59},
	}
	for _, tt := range cases {
		d := Decide(tt.command, 0, 1000)
		if !d.Allow || !d.Priced || d.USD != tt.usd {
			t.Fatalf("%s: %+v, want %.2f", tt.command, d, tt.usd)
		}
	}
	free := Decide("aws ec2 create-vpc-endpoint --vpc-endpoint-type Gateway", 0, 1)
	if free.Billable {
		t.Fatalf("gateway endpoint: %+v", free)
	}
	unknown := Decide("terraform apply", 0, 1)
	if !unknown.Allow || !unknown.Billable || unknown.Priced {
		t.Fatalf("terraform apply: %+v", unknown)
	}
}

func TestRunCommandDoesNotExecWhenBlocked(t *testing.T) {
	called := false
	err := RunCommand([]string{"aws", "ec2", "run-instances"}, Options{
		BudgetUSD: 1,
		Path:      filepath.Join(t.TempDir(), "session.json"),
		Exec: func(argv []string) error {
			called = true
			return nil
		},
	})
	var blocked *BlockedError
	if !errors.As(err, &blocked) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
	if blocked.ExitCode() != 2 || !strings.Contains(blocked.Message, "session budget") {
		t.Fatalf("blocked = %+v", blocked)
	}
}

func TestRunCommandExecsReadOnly(t *testing.T) {
	called := false
	err := RunCommand([]string{"aws", "ec2", "describe-instances"}, Options{
		BudgetUSD: 1,
		Exec:      func(argv []string) error { called = true; return nil },
	})
	if err != nil || !called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestSessionBudgetAccumulates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	opts := Options{BudgetUSD: 10, Path: path, Exec: func(argv []string) error { return nil }}
	if err := RunCommand([]string{"aws", "ec2", "run-instances"}, opts); err != nil {
		t.Fatal(err)
	}
	err := RunCommand([]string{"aws", "ec2", "allocate-address"}, opts)
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("second command err = %v", err)
	}
}

func TestHookDeniesOverBudget(t *testing.T) {
	in := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"aws ec2 run-instances"}}`
	var buf bytes.Buffer
	err := RunHook(strings.NewReader(in), &buf, Options{
		BudgetUSD: 1,
		Path:      filepath.Join(t.TempDir(), "session.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"permissionDecision": "deny"`) || !strings.Contains(out, "session budget") {
		t.Fatalf("hook output = %s", out)
	}
	if !strings.Contains(out, `"hookEventName": "PreToolUse"`) {
		t.Fatalf("hook output = %s", out)
	}
}

func TestHookAllowsReadOnly(t *testing.T) {
	in := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"az group list"}}`
	var buf bytes.Buffer
	if err := RunHook(strings.NewReader(in), &buf, Options{BudgetUSD: 1}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"permissionDecision": "allow"`) {
		t.Fatalf("hook output = %s", buf.String())
	}
}

func TestReportFiltersFixture(t *testing.T) {
	body, err := os.ReadFile("../watch/testdata/activity.json")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := EventsFromFixture(body)
	if err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 10, 17, 12, 4, 0, 0, time.UTC)
	alerts := CreatedDuring(ev, since, "casey@contoso.com", nil)
	if len(alerts) != 1 || alerts[0].ID != "evt-vm-user" {
		t.Fatalf("alerts = %+v", alerts)
	}
	trail, err := os.ReadFile("../watch/testdata/cloudtrail.json")
	if err != nil {
		t.Fatal(err)
	}
	awsEvents, err := EventsFromFixture(trail)
	if err != nil {
		t.Fatal(err)
	}
	awsAlerts := CreatedDuring(awsEvents, time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC), "ci-bot", nil)
	ids := map[string]bool{}
	for _, a := range awsAlerts {
		ids[a.ID] = true
	}
	if !ids["evt-eip"] || !ids["evt-nat"] || ids["evt-sso"] || ids["evt-gw"] || ids["evt-denied"] {
		t.Fatalf("aws alerts = %+v", awsAlerts)
	}
	var buf bytes.Buffer
	WriteReport(&buf, alerts)
	if !strings.Contains(buf.String(), "vm1") || !strings.Contains(buf.String(), "casey@contoso.com") {
		t.Fatalf("report = %s", buf.String())
	}
}
