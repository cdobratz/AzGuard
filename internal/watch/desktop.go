package watch

import (
	"fmt"
	"os/exec"
	"runtime"
)

// NotifyDesktop shows a best-effort desktop notification.
// Linux uses notify-send, macOS uses osascript, Windows uses a PowerShell balloon.
// A missing notifier returns an error and does not fail the poll by itself.
func NotifyDesktop(title, body string) error {
	name, args := desktopCommand(runtime.GOOS, title, body)
	if name == "" {
		return fmt.Errorf("no desktop notifier for %s", runtime.GOOS)
	}
	return exec.Command(name, args...).Run()
}

func desktopCommand(goos, title, body string) (string, []string) {
	switch goos {
	case "linux":
		return "notify-send", []string{title, body}
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", body, title)
		return "osascript", []string{"-e", script}
	case "windows":
		script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; $n = New-Object System.Windows.Forms.NotifyIcon; $n.Icon = [System.Drawing.SystemIcons]::Information; $n.Visible = $true; $n.ShowBalloonTip(5000, %q, %q, 'Info')`, title, body)
		return "powershell", []string{"-NoProfile", "-Command", script}
	default:
		return "", nil
	}
}
