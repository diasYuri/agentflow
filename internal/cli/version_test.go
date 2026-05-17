package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/version"
)

func TestVersionCommandPrintsHumanReadable(t *testing.T) {
	oldVersion := version.Version
	oldCommit := version.Commit
	oldDate := version.Date
	version.Version = "1.2.3"
	version.Commit = "abc123"
	version.Date = "2026-05-17"
	t.Cleanup(func() {
		version.Version = oldVersion
		version.Commit = oldCommit
		version.Date = oldDate
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "1.2.3") {
		t.Fatalf("expected version in output, got %q", got)
	}
	if !strings.Contains(got, "abc123") {
		t.Fatalf("expected commit in output, got %q", got)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	oldVersion := version.Version
	version.Version = "0.0.0-test"
	t.Cleanup(func() { version.Version = oldVersion })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"version"`) {
		t.Fatalf("expected version key in json, got %q", got)
	}
	if !strings.Contains(got, "0.0.0-test") {
		t.Fatalf("expected version value in json, got %q", got)
	}
}
