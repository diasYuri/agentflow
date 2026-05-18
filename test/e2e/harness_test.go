//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHarness_BuildAndVersion(t *testing.T) {
	h := New(t)
	h.Build()

	res := h.Run("version")
	res.AssertSuccess(t)

	if !strings.Contains(res.Stdout, "agentflow") {
		t.Fatalf("expected version output to contain 'agentflow', got:\n%s", res.Stdout)
	}
}

func TestHarness_CommandCapture(t *testing.T) {
	h := New(t)
	h.Build()

	res := h.Run("version", "--json")
	res.AssertSuccess(t)

	if res.Stdout == "" {
		t.Fatal("expected stdout to be captured")
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestHarness_FailingCommand_Diagnostics(t *testing.T) {
	h := New(t)
	h.Build()

	res := h.Run("validate", "nonexistent-workflow.yaml")
	res.AssertExitCode(t, 1)

	if res.Stderr == "" && res.Stdout == "" {
		t.Fatal("expected some output on failure for diagnostics")
	}
	if !strings.Contains(res.Cmd, "nonexistent-workflow.yaml") {
		t.Fatalf("expected result to record command line, got: %s", res.Cmd)
	}
}

func TestHarness_FakeProviderRun(t *testing.T) {
	h := New(t)
	h.Build()

	root := repoRoot(t)
	workflowSrc := filepath.Join(root, "test", "e2e", "testdata", "hello.yaml")
	inputsSrc := filepath.Join(root, "test", "e2e", "testdata", "inputs.json")
	fakeSrc := filepath.Join(root, "test", "e2e", "testdata", "fake-responses.json")

	h.StageFile(t, workflowSrc, "hello.yaml")
	h.StageFile(t, inputsSrc, "inputs.json")
	h.StageFile(t, fakeSrc, "fake.json")

	res := h.Run(
		"run", "-it", "hello.yaml",
		"--input-json", filepath.Join(h.Workspace, "inputs.json"),
		"--fake-provider-path", filepath.Join(h.Workspace, "fake.json"),
	)
	res.AssertSuccess(t)

	if !strings.Contains(res.Stdout, "run_id:") {
		t.Fatalf("expected run output to contain run_id, got:\n%s", res.Stdout)
	}
}
