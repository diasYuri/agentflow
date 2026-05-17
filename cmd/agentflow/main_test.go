package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBuildWithVersionLdflags performs an end-to-end build of the CLI binary
// with custom ldflags and verifies the version command reflects the injected
// metadata. This validates the release pipeline's version injection plumbing.
func TestBuildWithVersionLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build integration test in short mode")
	}

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "agentflow")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	ldflags := "-X main.buildVersion=1.2.3-test -X main.buildCommit=abc123def -X main.buildDate=2026-05-17T12:00:00Z"
	cmd := exec.Command("go", "build", "-o", binary, "-ldflags", ldflags, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	versionOut, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\n%s", err, versionOut)
	}

	got := string(versionOut)
	if !strings.Contains(got, "1.2.3-test") {
		t.Errorf("expected version 1.2.3-test in output, got %q", got)
	}
	if !strings.Contains(got, "abc123def") {
		t.Errorf("expected commit abc123def in output, got %q", got)
	}
}
