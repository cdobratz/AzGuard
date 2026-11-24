package action

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionScriptBudgetAndComment(t *testing.T) {
	fake := writeFakeAzguard(t, "testdata/preflight.json", "testdata/resources.json", 0)
	plan := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(plan, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	over := runScript(t, map[string]string{
		"AZGUARD_BIN":       fake,
		"INPUT_PLAN":        plan,
		"INPUT_BUDGET":      "25",
		"INPUT_SCAN_IDLE":   "false",
		"INPUT_COMMENT":     "true",
		"GITHUB_EVENT_NAME": "pull_request",
		"PR_NUMBER":         "9",
		"GITHUB_REPOSITORY": "cdobratz/AzGuard",
	})
	if over.exit != 1 {
		t.Fatalf("exit %d stderr %s", over.exit, over.stderr)
	}
	if !strings.Contains(over.comment, "this will create a NAT Gateway at about $32.85/month") {
		t.Fatalf("comment = %s", over.comment)
	}
	if !strings.Contains(over.comment, "exceeds the budget") || !strings.Contains(over.comment, "https://www.infracost.io/") {
		t.Fatalf("comment = %s", over.comment)
	}
	if !strings.Contains(over.comment, "<!-- azguard-report -->") {
		t.Fatalf("comment = %s", over.comment)
	}

	under := runScript(t, map[string]string{
		"AZGUARD_BIN":     fake,
		"INPUT_PLAN":      plan,
		"INPUT_BUDGET":    "100",
		"INPUT_SCAN_IDLE": "false",
	})
	if under.exit != 0 || !strings.Contains(under.comment, "within the budget") {
		t.Fatalf("exit %d comment %s stderr %s", under.exit, under.comment, under.stderr)
	}

	missing := runScript(t, map[string]string{
		"AZGUARD_BIN":     fake,
		"INPUT_PLAN":      filepath.Join(t.TempDir(), "missing.json"),
		"INPUT_BUDGET":    "",
		"INPUT_SCAN_IDLE": "false",
	})
	if missing.exit != 0 || !strings.Contains(missing.comment, "was not found") {
		t.Fatalf("exit %d comment %s", missing.exit, missing.comment)
	}
}

func TestActionScriptIdleFixture(t *testing.T) {
	fake := writeFakeAzguard(t, "testdata/preflight.json", "testdata/resources.json", 0)
	out := runScript(t, map[string]string{
		"AZGUARD_BIN":     fake,
		"INPUT_PLAN":      filepath.Join(t.TempDir(), "missing.json"),
		"INPUT_BUDGET":    "1",
		"INPUT_SCAN_IDLE": "true",
	})
	if out.exit != 1 {
		t.Fatalf("exit %d stderr %s comment %s", out.exit, out.stderr, out.comment)
	}
	if !strings.Contains(out.comment, "orphan") || !strings.Contains(out.comment, "azure.disk.unattached") {
		t.Fatalf("comment = %s", out.comment)
	}

	unconfigured := filepath.Join(t.TempDir(), "unconfigured.json")
	body := []byte(`{"findings":[],"errors":["Azure subscription is not configured; skipped Azure rules"],"total_estimated_monthly_usd":0}`)
	if err := os.WriteFile(unconfigured, body, 0o644); err != nil {
		t.Fatal(err)
	}
	fake = writeFakeAzguard(t, "testdata/preflight.json", unconfigured, 1)
	skipped := runScript(t, map[string]string{
		"AZGUARD_BIN":     fake,
		"INPUT_PLAN":      filepath.Join(t.TempDir(), "missing.json"),
		"INPUT_BUDGET":    "25",
		"INPUT_SCAN_IDLE": "true",
	})
	if skipped.exit != 0 || !strings.Contains(skipped.comment, "not configured") {
		t.Fatalf("exit %d comment %s stderr %s", skipped.exit, skipped.comment, skipped.stderr)
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	name := "azguard_1.2.3_linux_amd64.tar.gz"
	body := []byte("azguard-fixture\n")
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	line := fmt.Sprintf("%x  %s\n", sum, name)
	sums := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(sums, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", "source ./run.sh && verify_checksum \"$1\" \"$2\"", "bash", path, sums)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("bash", "-c", "source ./run.sh && verify_checksum \"$1\" \"$2\"", "bash", path, sums)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected checksum failure")
	}
}

func TestExampleWorkflowIsNotActive(t *testing.T) {
	if _, err := os.Stat("../.github/workflows/azguard.yml"); !os.IsNotExist(err) {
		t.Fatal("examples/azguard.yml must not be copied into .github/workflows")
	}
	body, err := os.ReadFile("../examples/azguard.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, needle := range []string{
		"azure/login@a641126d1b8aa4d1fa005f4f92df94a3a4c4c906",
		"aws-actions/configure-aws-credentials@e1253824e5c10ff9df46874f81ed3ec929e19cfd",
		"actions/checkout@3d3c42e5aac5ba805da76410c181273ba90b1",
		"id-token: write",
		"infracost.io",
		"azguard preflight",
		"./  # this repository",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("example missing %s", needle)
		}
	}
	action, err := os.ReadFile("../action.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(action), "using: composite") {
		t.Fatal("action.yml should be a composite action")
	}
}

type scriptResult struct {
	comment string
	stderr  string
	exit    int
}

func runScript(t *testing.T, extra map[string]string) scriptResult {
	t.Helper()
	out := filepath.Join(t.TempDir(), "comment.md")
	githubOut := filepath.Join(t.TempDir(), "github_output")
	cmd := exec.Command("bash", "run.sh")
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"AZGUARD_COMMENT_OUT=" + out,
		"GITHUB_OUTPUT=" + githubOut,
		"TMPDIR=" + t.TempDir(),
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	raw, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errorsAs(err, &exitErr) {
			t.Fatal(err)
		}
		exit = exitErr.ExitCode()
	}
	comment, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return scriptResult{comment: string(comment), stderr: string(raw), exit: exit}
}

func errorsAs(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}

func writeFakeAzguard(t *testing.T, preflightPath, resourcesPath string, resourcesExit int) string {
	t.Helper()
	preflight, err := filepath.Abs(preflightPath)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := filepath.Abs(resourcesPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "azguard")
	script := fmt.Sprintf(`#!/bin/sh
cmd="$1"
case "$cmd" in
  preflight) cat %q ;;
  resources) cat %q; exit %d ;;
  *) echo "unexpected $cmd" >&2; exit 99 ;;
esac
`, preflight, resources, resourcesExit)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
