package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Result struct {
	Output    string
	ExitCode  int
	Error     error
}

type Executor interface {
	Execute(ctx context.Context, command string) (*Result, error)
	Name() string
}

type PowerShellExecutor struct {
	Command string
}

func NewPowerShellExecutor() *PowerShellExecutor {
	return &PowerShellExecutor{
		Command: "powershell",
	}
}

func (e *PowerShellExecutor) Name() string {
	return "powershell"
}

func (e *PowerShellExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	cmd := exec.CommandContext(ctx, e.Command, "-NoProfile", "-Command", command)
	output, err := cmd.CombinedOutput()

	result := &Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Error = err
		} else {
			result.Error = err
		}
	}

	return result, nil
}

type CmdExecutor struct{}

func NewCmdExecutor() *CmdExecutor {
	return &CmdExecutor{}
}

func (e *CmdExecutor) Name() string {
	return "cmd"
}

func (e *CmdExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	output, err := cmd.CombinedOutput()

	result := &Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Error = err
		} else {
			result.Error = err
		}
	}

	return result, nil
}

type BashExecutor struct{}

func NewBashExecutor() *BashExecutor {
	return &BashExecutor{}
}

func (e *BashExecutor) Name() string {
	return "bash"
}

func (e *BashExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	output, err := cmd.CombinedOutput()

	result := &Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Error = err
		} else {
			result.Error = err
		}
	}

	return result, nil
}

type AzureCLIExecutor struct{}

func NewAzureCLIExecutor() *AzureCLIExecutor {
	return &AzureCLIExecutor{}
}

func (e *AzureCLIExecutor) Name() string {
	return "azure-cli"
}

func (e *AzureCLIExecutor) Execute(ctx context.Context, command string) (*Result, error) {
	args := strings.Fields(command)
	args = append([]string{}, args...)

	cmd := exec.CommandContext(ctx, "az", args...)
	output, err := cmd.CombinedOutput()

	result := &Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Error = err
		} else {
			result.Error = err
		}
	}

	return result, nil
}

type ScriptExecutor struct{}

func NewScriptExecutor() *ScriptExecutor {
	return &ScriptExecutor{}
}

func (e *ScriptExecutor) ExecuteScript(ctx context.Context, scriptPath string, args ...string) (*Result, error) {
	_, err := os.Stat(scriptPath)
	if err != nil {
		return &Result{Error: err}, err
	}

	var cmd *exec.Cmd
	ext := strings.ToLower(filepath.Ext(scriptPath))

	switch ext {
	case ".ps1":
		cmdArgs := []string{"-NoProfile", "-File", scriptPath}
		cmdArgs = append(cmdArgs, args...)
		cmd = exec.CommandContext(ctx, "powershell", cmdArgs...)
	case ".sh":
		cmd = exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
	case ".bat", ".cmd":
		cmd = exec.CommandContext(ctx, "cmd", append([]string{"/c", scriptPath}, args...)...)
	default:
		cmd = exec.CommandContext(ctx, scriptPath, args...)
	}

	output, err := cmd.CombinedOutput()

	result := &Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			result.Error = err
		} else {
			result.Error = err
		}
	}

	return result, nil
}

func DetectShell() string {
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "powershell"
	}
	if os.Getenv("TERM_PROGRAM") == "vscode" || os.Getenv("WT_SESSION") != "" {
		return "powershell"
	}
	return "bash"
}

func AutoDetectExecutor() Executor {
	shell := DetectShell()
	switch shell {
	case "powershell":
		return NewPowerShellExecutor()
	default:
		return NewBashExecutor()
	}
}
