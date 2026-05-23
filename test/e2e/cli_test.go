//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_ValidateKnownGoodSample(t *testing.T) {
	h := New(t)
	h.Build()

	root := repoRoot(t)
	wf := h.StageFile(t, filepath.Join(root, "samples", "workflows", "v2-intro.yaml"), "v2-intro.yaml")

	res := h.Run("validate", wf)
	res.AssertSuccess(t)

	if !strings.Contains(res.Stdout, "valid: v2-intro") {
		t.Fatalf("expected validation success message, got:\n%s", res.Stdout)
	}
}

func TestCLI_GraphSample(t *testing.T) {
	h := New(t)
	h.Build()

	root := repoRoot(t)
	wf := h.StageFile(t, filepath.Join(root, "samples", "workflows", "local-health-check.yaml"), "local-health-check.yaml")

	res := h.Run("graph", wf, "--format", "mermaid")
	res.AssertSuccess(t)

	wantSubstrings := []string{
		"graph TD",
		"list_module --> run_tests",
		"run_tests --> tests_failed",
		"run_tests --> done",
		"tests_failed --> done",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(res.Stdout, want) {
			t.Fatalf("expected graph output to contain %q, got:\n%s", want, res.Stdout)
		}
	}
}

func TestCLI_DryRunResolvesInputs(t *testing.T) {
	h := New(t)
	h.Build()

	root := repoRoot(t)
	wf := h.StageFile(t, filepath.Join(root, "samples", "workflows", "v2-intro.yaml"), "v2-intro.yaml")

	res := h.Run("dry-run", wf, "--input", "name=tester")
	res.AssertSuccess(t)

	if !strings.Contains(res.Stdout, "\"inputs\"") {
		t.Fatalf("expected inputs in dry-run output, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "\"name\": \"tester\"") {
		t.Fatalf("expected resolved input value, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "\"workflow\": \"v2-intro\"") {
		t.Fatalf("expected workflow name, got:\n%s", res.Stdout)
	}
}

func TestCLI_RunDryRunDoesNotPersistDaemonState(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	start := h.Run("daemon", "start")
	start.AssertSuccess(t)
	t.Cleanup(func() {
		h.Run("daemon", "stop")
	})

	root := repoRoot(t)
	wf := h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "simple-run.yaml"), "simple-run.yaml")

	res := h.Run("run", wf, "--dry-run")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "\"workflow\": \"simple-run\"") {
		t.Fatalf("expected dry-run plan output, got:\n%s", res.Stdout)
	}

	list := h.Run("workflow", "runs", "--no-color")
	list.AssertSuccess(t)
	if !strings.Contains(list.Stdout, "No workflow runs") {
		t.Fatalf("expected daemon run list to remain empty, got:\n%s", list.Stdout)
	}

	runRoot := filepath.Join(h.Home, ".agentflow", "runs")
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		t.Fatalf("read run root %s: %v", runRoot, err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no run directories in %s, found %d", runRoot, len(entries))
	}
}
