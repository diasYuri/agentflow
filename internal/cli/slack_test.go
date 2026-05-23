package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/web/settings"
)

func TestSlackCommandExists(t *testing.T) {
	cmd := NewRootCommand()
	if findSubcommand(cmd, "slack") == nil {
		t.Fatal("expected slack command to be registered")
	}
}

func TestSlackCommandHelpListsFlags(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"slack", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"--app-token", "--bot-token", "--project", "--root",
		"--daemon", "--daemon-socket", "-l", "--log",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in help output:\n%s", want, got)
		}
	}
}

func TestSlackFlagsOverridesIncludeChangedValues(t *testing.T) {
	flags := slackFlags{
		root:            "/srv/root",
		daemon:          string(settings.DaemonRequirementRequired),
		rootSet:         true,
		daemonSet:       true,
		daemonSocket:    "/tmp/daemon.sock",
		daemonSocketSet: true,
	}
	ov := flags.overrides()
	if ov.Root == nil || *ov.Root != "/srv/root" {
		t.Fatalf("root=%v", ov.Root)
	}
	if ov.Daemon == nil || *ov.Daemon != settings.DaemonRequirementRequired {
		t.Fatalf("daemon=%v", ov.Daemon)
	}
}
