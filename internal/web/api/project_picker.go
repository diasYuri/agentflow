package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type NativeFolderPicker struct{}

func (NativeFolderPicker) PickFolder(r *http.Request) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		return runFolderPicker(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Select an AgentFlow project folder")`)
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description = "Select an AgentFlow project folder"; if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.SelectedPath }`
		return runFolderPicker(ctx, "powershell", "-NoProfile", "-Command", script)
	default:
		if path, err := exec.LookPath("zenity"); err == nil {
			return runFolderPicker(ctx, path, "--file-selection", "--directory", "--title=Select an AgentFlow project folder")
		}
		if path, err := exec.LookPath("kdialog"); err == nil {
			return runFolderPicker(ctx, path, "--getexistingdirectory", ".")
		}
		return "", fmt.Errorf("native folder picker is not available on this platform")
	}
}

func runFolderPicker(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("select folder: %s", msg)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("folder selection was cancelled")
	}
	return path, nil
}
