//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntime_InteractiveRun(t *testing.T) {
	h := New(t)
	h.Build()

	root := repoRoot(t)
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "simple-run.yaml"), "simple.yaml")

	res := h.Run("run", "-it", "simple.yaml")
	res.AssertSuccess(t)

	runID := extractRunID(t, res.Stdout)
	runDir := extractRunDir(t, res.Stdout)

	if runID == "" {
		t.Fatal("expected non-empty run_id")
	}
	if runDir == "" {
		t.Fatal("expected non-empty run_dir")
	}

	// Assert run directory exists and contains expected artifacts
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("run directory not found: %v", err)
	}
	for _, file := range []string{"run.json", "summary.json", "events.jsonl", "workflow.yaml"} {
		path := filepath.Join(runDir, file)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	// Assert node stdout artifact exists and contains expected output
	stdoutPath := filepath.Join(runDir, "nodes", "hello", "stdout.txt")
	data, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout artifact: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("unexpected stdout content: %q", string(data))
	}

	// Assert summary shows success
	summaryPath := filepath.Join(runDir, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if !strings.Contains(string(summaryData), `"status": "success"`) {
		t.Fatalf("expected success in summary: %q", string(summaryData))
	}
}

func TestRuntime_SecretMasking(t *testing.T) {
	h := New(t)
	h.Build()

	secret := "e2e-super-secret-token-12345"
	h.Setenv("AGENTFLOW_E2E_SECRET", secret)

	root := repoRoot(t)
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "secret-test.yaml"), "secret.yaml")

	res := h.Run("run", "-it", "secret.yaml")
	res.AssertSuccess(t)

	runDir := extractRunDir(t, res.Stdout)
	if runDir == "" {
		t.Fatal("expected non-empty run_dir")
	}

	// stdout must be redacted
	stdoutPath := filepath.Join(runDir, "nodes", "leak", "stdout.txt")
	stdoutData, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout artifact: %v", err)
	}
	if strings.Contains(string(stdoutData), secret) {
		t.Fatalf("stdout contains unmasked secret: %q", string(stdoutData))
	}
	if !strings.Contains(string(stdoutData), "[REDACTED]") {
		t.Fatalf("stdout does not contain redaction marker: %q", string(stdoutData))
	}

	// summary must not contain the raw secret
	summaryPath := filepath.Join(runDir, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if strings.Contains(string(summaryData), secret) {
		t.Fatalf("summary contains unmasked secret:\n%s", string(summaryData))
	}

	// events must not contain the raw secret
	eventsPath := filepath.Join(runDir, "events.jsonl")
	eventsData, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if strings.Contains(string(eventsData), secret) {
		t.Fatalf("events contain unmasked secret:\n%s", string(eventsData))
	}
}
