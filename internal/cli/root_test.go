package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphCommandPrintsMermaid(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowRef := "graph-test"
	workflowPath := filepath.Join(workflowDir, workflowRef+".yaml")
	err = os.WriteFile(workflowPath, []byte(`
version: "1"
name: graph-test
nodes:
  - id: plan
    kind: noop
  - id: implement
    kind: noop
    depends_on: [plan]
  - id: isolated
    kind: noop
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"graph", workflowRef, "--format", "mermaid"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	const want = "graph TD\n  plan --> implement\n  isolated\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected graph output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestWorkflowListReportsDaemonUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected daemon unavailable error")
	}
	if !strings.Contains(err.Error(), "agentflowd is not running") {
		t.Fatalf("expected daemon unavailable message, got %v", err)
	}
}

func TestRunCommandIgnoresOutputDirFlag(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})

	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "cli-run.yaml"), []byte(`
version: "1"
name: cli-run
nodes:
  - id: ok
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"run", "cli-run", "-it", "--output-dir", filepath.Join(dir, "ignored")})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), filepath.Join(home, ".agentflow", "runs")) {
		t.Fatalf("expected run dir under home, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), filepath.Join(dir, "ignored")) {
		t.Fatalf("output-dir flag affected storage:\n%s", out.String())
	}
}
