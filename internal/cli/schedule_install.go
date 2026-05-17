package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type scheduleDispatcherInstaller interface {
	Ensure(context.Context, string) error
	Remove(context.Context) error
}

var newScheduleDispatcherInstaller = func() scheduleDispatcherInstaller {
	switch runtime.GOOS {
	case "darwin":
		return &launchdScheduleInstaller{}
	default:
		return &crontabScheduleInstaller{}
	}
}

func ensureScheduleDispatcherInstalled(ctx context.Context) error {
	binaryPath, err := findAgentflowBinary()
	if err != nil {
		return err
	}
	return newScheduleDispatcherInstaller().Ensure(ctx, binaryPath)
}

func removeScheduleDispatcher(ctx context.Context) error {
	return newScheduleDispatcherInstaller().Remove(ctx)
}

type crontabScheduleInstaller struct{}

func (i *crontabScheduleInstaller) Ensure(ctx context.Context, binaryPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(home, ".agentflow", "schedule-dispatcher.log")
	entry := fmt.Sprintf("# BEGIN AGENTFLOW SCHEDULE DISPATCHER\n* * * * * %s workflow schedule tick >> %s 2>&1\n# END AGENTFLOW SCHEDULE DISPATCHER", shellQuote(binaryPath), shellQuote(logPath))
	updated, err := upsertCrontabEntry(ctx, entry)
	if err != nil {
		return err
	}
	return writeCrontab(ctx, updated)
}

func (i *crontabScheduleInstaller) Remove(ctx context.Context) error {
	updated, err := removeCrontabEntry(ctx)
	if err != nil {
		return err
	}
	return writeCrontab(ctx, updated)
}

type launchdScheduleInstaller struct{}

func (i *launchdScheduleInstaller) Ensure(ctx context.Context, binaryPath string) error {
	plistPath, label, err := launchdDispatcherPaths()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(home, ".agentflow", "schedule-dispatcher.log")
	content := launchdPlistContent(label, binaryPath, logPath)
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return err
	}
	if err := launchctlBootout(ctx, plistPath); err != nil && !isLaunchctlMissingEntry(err) {
		return err
	}
	if err := launchctlBootstrap(ctx, plistPath); err != nil {
		_ = os.Remove(plistPath)
		return err
	}
	return nil
}

func (i *launchdScheduleInstaller) Remove(ctx context.Context) error {
	plistPath, _, err := launchdDispatcherPaths()
	if err != nil {
		return err
	}
	if err := launchctlBootout(ctx, plistPath); err != nil && !isLaunchctlMissingEntry(err) {
		return err
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func launchdDispatcherPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", "", fmt.Errorf("home directory is required")
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.agentflow.schedule.dispatcher.plist")
	return plistPath, "com.agentflow.schedule.dispatcher", nil
}

func launchdPlistContent(label, binaryPath, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>workflow</string>
    <string>schedule</string>
    <string>tick</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>StartInterval</key>
  <integer>60</integer>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, xmlEscape(label), xmlEscape(binaryPath), xmlEscape(logPath), xmlEscape(logPath))
}

func launchctlBootstrap(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "launchctl", "bootstrap", "gui/"+launchdUID(), plistPath)
	return cmd.Run()
}

func launchctlBootout(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "launchctl", "bootout", "gui/"+launchdUID(), plistPath)
	return cmd.Run()
}

func launchdUID() string {
	return fmt.Sprint(os.Getuid())
}

func isLaunchctlMissingEntry(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not find specified service") || strings.Contains(msg, "not found")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func crontabMarkerStart() string {
	return "# BEGIN AGENTFLOW SCHEDULE DISPATCHER"
}

func crontabMarkerEnd() string {
	return "# END AGENTFLOW SCHEDULE DISPATCHER"
}

func upsertCrontabEntry(ctx context.Context, entry string) (string, error) {
	current, err := readCrontab(ctx)
	if err != nil {
		return "", err
	}
	lines := removeMarkedCrontabBlock(strings.Split(current, "\n"))
	lines = trimTrailingEmptyLines(lines)
	lines = append(lines, crontabMarkerStart())
	lines = append(lines, strings.Split(entry, "\n")...)
	lines = append(lines, crontabMarkerEnd())
	return strings.Join(lines, "\n"), nil
}

func removeCrontabEntry(ctx context.Context) (string, error) {
	current, err := readCrontab(ctx)
	if err != nil {
		return "", err
	}
	lines := removeMarkedCrontabBlock(strings.Split(current, "\n"))
	return strings.Join(trimTrailingEmptyLines(lines), "\n"), nil
}

func readCrontab(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "crontab", "-l")
	out, err := cmd.Output()
	if err == nil {
		return string(out), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return "", nil
	}
	return "", err
}

func writeCrontab(ctx context.Context, content string) error {
	cmd := exec.CommandContext(ctx, "crontab", "-")
	cmd.Stdin = strings.NewReader(ensureTrailingNewline(content))
	return cmd.Run()
}

func removeMarkedCrontabBlock(lines []string) []string {
	var (
		out      []string
		inBlock  bool
		skipping bool
	)
	for _, line := range lines {
		if strings.TrimSpace(line) == crontabMarkerStart() {
			skipping = true
			inBlock = true
			continue
		}
		if inBlock && strings.TrimSpace(line) == crontabMarkerEnd() {
			inBlock = false
			skipping = false
			continue
		}
		if skipping {
			continue
		}
		out = append(out, line)
	}
	return out
}

func trimTrailingEmptyLines(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return append([]string(nil), lines[:end]...)
}

func ensureTrailingNewline(content string) string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return ""
	}
	return content + "\n"
}

func findAgentflowBinary() (string, error) {
	if path := os.Getenv("AGENTFLOW_PATH"); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("agentflow"); err == nil {
		return path, nil
	}
	self, err := os.Executable()
	if err == nil {
		base := filepath.Base(self)
		if base == "agentflow" {
			return self, nil
		}
	}
	return "", fmt.Errorf("agentflow binary not found; build and install it or set AGENTFLOW_PATH")
}
