package e2e

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// StageFile copies src into the harness workspace as dstName and returns
// the absolute path of the staged file.
func (h *Harness) StageFile(t *testing.T, src, dstName string) string {
	t.Helper()

	dst := filepath.Join(h.Workspace, dstName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", dst, err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("stage file %q -> %q: %v", src, dst, err)
	}
	return dst
}

// StageFromSamples copies workflow and input files from the repository
// samples/ directory into the harness workspace.
func StageFromSamples(t *testing.T, h *Harness, workflowFile, inputFile string) (workflowPath, inputPath string) {
	t.Helper()

	root := repoRoot(t)
	if workflowFile != "" {
		src := filepath.Join(root, "samples", "workflows", workflowFile)
		workflowPath = h.StageFile(t, src, workflowFile)
	}
	if inputFile != "" {
		src := filepath.Join(root, "samples", "inputs", inputFile)
		inputPath = h.StageFile(t, src, inputFile)
	}
	return
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller")
	}
	// file is .../test/e2e/fixtures.go
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func extractRunID(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "run_id:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "run_id:"))
		}
	}
	t.Fatalf("run_id not found in output:\n%s", output)
	return ""
}

func extractRunDir(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "run_dir:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "run_dir:"))
		}
	}
	t.Fatalf("run_dir not found in output:\n%s", output)
	return ""
}

func pollWorkflowStatus(t *testing.T, h *Harness, runID, wantStatus string, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		res := h.Run("workflow", "status", runID, "--no-color")
		if res.ExitCode == 0 && strings.Contains(res.Stdout, "status: "+wantStatus) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("workflow %s did not reach status %s within %v", runID, wantStatus, maxWait)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
