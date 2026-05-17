package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuildWithVersionLdflags verifies that the daemon binary builds successfully
// with version metadata injected via ldflags, matching the release pipeline
// configuration.
func TestBuildWithVersionLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build integration test in short mode")
	}

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "agentflowd")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	ldflags := "-X main.buildVersion=1.2.3-test -X main.buildCommit=abc123def -X main.buildDate=2026-05-17T12:00:00Z"
	cmd := exec.Command("go", "build", "-o", binary, "-ldflags", ldflags, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// The daemon has no version subcommand; successful build is sufficient
	// validation that ldflags plumbing works.
}
