package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/web/settings"
)

func TestWebCommandExists(t *testing.T) {
	cmd := NewRootCommand()
	if findSubcommand(cmd, "web") == nil {
		t.Fatal("expected web command to be registered")
	}
}

func TestWebCommandHelpListsFlags(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"web", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"--host", "--port", "--no-open", "--dev-assets",
		"--daemon", "--root", "--token",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in help output:\n%s", want, got)
		}
	}
}

func TestWebCommandFlagsCaptured(t *testing.T) {
	cmd := NewRootCommand()
	web := findSubcommand(cmd, "web")
	if web == nil {
		t.Fatal("expected web subcommand")
	}
	if err := web.ParseFlags([]string{
		"--host", "127.0.0.2",
		"--port", "12345",
		"--no-open",
		"--daemon", "off",
		"--token", "abc",
	}); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, want := range []string{"host", "port", "no-open", "daemon", "token"} {
		if !web.Flags().Changed(want) {
			t.Fatalf("expected flag %q to be marked changed", want)
		}
	}
}

func TestWebFlagsOverridesIncludeChangedValues(t *testing.T) {
	flags := webFlags{
		host:         "127.0.0.5",
		port:         1234,
		noOpen:       true,
		devAssets:    "/tmp/dev",
		daemon:       string(settings.DaemonRequirementRequired),
		root:         "/srv/root",
		token:        "tok",
		hostSet:      true,
		portSet:      true,
		devAssetsSet: true,
		daemonSet:    true,
		rootSet:      true,
		tokenSet:     true,
	}
	ov := flags.overrides()
	if ov.Host == nil || *ov.Host != "127.0.0.5" {
		t.Fatalf("host=%v", ov.Host)
	}
	if ov.Port == nil || *ov.Port != 1234 {
		t.Fatalf("port=%v", ov.Port)
	}
	if ov.NoOpen == nil || !*ov.NoOpen {
		t.Fatalf("no-open not set")
	}
	if ov.DevAssets == nil || *ov.DevAssets != "/tmp/dev" {
		t.Fatalf("dev_assets=%v", ov.DevAssets)
	}
	if ov.Daemon == nil || *ov.Daemon != settings.DaemonRequirementRequired {
		t.Fatalf("daemon=%v", ov.Daemon)
	}
	if ov.Root == nil || *ov.Root != "/srv/root" {
		t.Fatalf("root=%v", ov.Root)
	}
	if ov.Token == nil || *ov.Token != "tok" {
		t.Fatalf("token=%v", ov.Token)
	}
}

func TestWebFlagsOverridesSkipsUntouchedFlags(t *testing.T) {
	flags := webFlags{}
	ov := flags.overrides()
	if ov.Host != nil || ov.Port != nil || ov.DevAssets != nil ||
		ov.Daemon != nil || ov.Root != nil || ov.Token != nil {
		t.Fatalf("expected nils, got %#v", ov)
	}
	if ov.NoOpen != nil {
		t.Fatalf("expected NoOpen nil when flag unchanged")
	}
}
