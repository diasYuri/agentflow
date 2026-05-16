package yaml

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestWorkflowRepositoryLoadPrefersLocalScope(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "agentflow", "workflows")
	globalRoot := filepath.Join(root, "home", ".agentflow", "workflows")
	writeWorkflowFile(t, localRoot, "local.yaml", "local-workflow", "local")
	writeWorkflowFile(t, globalRoot, "global.yaml", "local-workflow", "global")

	repo := NewWorkflowRepository(localRoot, globalRoot)
	spec, sourcePath, err := repo.Load(context.Background(), "local-workflow")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Description != "local" {
		t.Fatalf("expected local workflow, got %#v", spec.Description)
	}
	if !strings.HasPrefix(sourcePath, localRoot) {
		t.Fatalf("expected local source path, got %q", sourcePath)
	}
}

func TestWorkflowRepositoryLoadFallsBackToGlobalScope(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "agentflow", "workflows")
	globalRoot := filepath.Join(root, "home", ".agentflow", "workflows")
	writeWorkflowFile(t, globalRoot, "global.yaml", "global-workflow", "global")

	repo := NewWorkflowRepository(localRoot, globalRoot)
	spec, sourcePath, err := repo.Load(context.Background(), "global-workflow")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Description != "global" {
		t.Fatalf("expected global workflow, got %#v", spec.Description)
	}
	if !strings.HasPrefix(sourcePath, globalRoot) {
		t.Fatalf("expected global source path, got %q", sourcePath)
	}
}

func TestWorkflowRepositoryLoadErrorsOnDuplicateInScope(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "agentflow", "workflows")
	writeWorkflowFile(t, localRoot, "one.yaml", "dup", "first")
	writeWorkflowFile(t, localRoot, "two.yaml", "dup", "second")

	repo := NewWorkflowRepository(localRoot, filepath.Join(root, "home", ".agentflow", "workflows"))
	_, _, err := repo.Load(context.Background(), "dup")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate workflow name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowRepositoryLoadReturnsHelpfulNotFoundError(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "agentflow", "workflows")
	globalRoot := filepath.Join(root, "home", ".agentflow", "workflows")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	repo := NewWorkflowRepository(localRoot, globalRoot)
	_, _, err := repo.Load(context.Background(), "missing-workflow")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "missing-workflow") {
		t.Fatalf("expected workflow name in error, got %v", err)
	}
}

func TestWorkflowRepositoryLoadRejectsRemovedToolsField(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "agentflow", "workflows")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(localRoot, "tools.yaml")
	content := []byte(`version: "1"
name: removed-tools
nodes:
  - id: agent
    kind: agent
    prompt: test
    tools:
      - shell
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := NewWorkflowRepository(localRoot, filepath.Join(root, "home", ".agentflow", "workflows"))
	_, _, err := repo.Load(context.Background(), "removed-tools")
	if err == nil {
		t.Fatal("expected removed tools field to be rejected")
	}
	if !strings.Contains(err.Error(), "field tools not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowRepositoryLoadAcceptsDirectWorkflowPath(t *testing.T) {
	root := t.TempDir()
	path := writeWorkflowFile(t, root, "direct.yaml", "direct-workflow", "direct")

	repo := NewWorkflowRepository(filepath.Join(root, "missing-local"), filepath.Join(root, "missing-global"))
	spec, sourcePath, err := repo.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "direct-workflow" {
		t.Fatalf("expected direct workflow, got %q", spec.Name)
	}
	if sourcePath != filepath.Clean(path) {
		t.Fatalf("expected direct source path, got %q", sourcePath)
	}
}

func TestClaudeSampleWorkflowValidates(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, "samples", "workflows", "claude-code-review.yaml")

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Validate(spec, workflow.DefaultProviders(), nil); err != nil {
		t.Fatalf("expected claude sample to validate: %v", err)
	}
}

func TestDecodeWorktreeTrue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "worktree-true.yaml")
	content := []byte(`version: "1"
name: worktree-true
nodes:
  - id: ok
    kind: noop
worktree: true
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Worktree.Enabled {
		t.Fatal("expected worktree enabled")
	}
	if spec.Worktree.Provider != "pi" {
		t.Fatalf("expected provider pi, got %q", spec.Worktree.Provider)
	}
	if spec.Worktree.Base != "current" {
		t.Fatalf("expected base current, got %q", spec.Worktree.Base)
	}
	if spec.Worktree.Merge.Strategy != "deterministic" {
		t.Fatalf("expected merge strategy deterministic, got %q", spec.Worktree.Merge.Strategy)
	}
	if spec.Worktree.Merge.OnConflict != "agent" {
		t.Fatalf("expected merge on_conflict agent, got %q", spec.Worktree.Merge.OnConflict)
	}
	if spec.Worktree.Cleanup.OnSuccess == nil || !*spec.Worktree.Cleanup.OnSuccess {
		t.Fatal("expected cleanup on_success true")
	}
	if spec.Worktree.Cleanup.OnFailure != "keep" {
		t.Fatalf("expected cleanup on_failure keep, got %q", spec.Worktree.Cleanup.OnFailure)
	}
}

func TestDecodeWorktreeProviderShortcut(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "worktree-provider.yaml")
	content := []byte(`version: "1"
name: worktree-provider
nodes:
  - id: ok
    kind: noop
worktree-provider: pi
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Worktree.Enabled {
		t.Fatal("expected worktree enabled")
	}
	if spec.Worktree.Provider != "pi" {
		t.Fatalf("expected provider pi, got %q", spec.Worktree.Provider)
	}
}

func TestDecodeWorktreeStructured(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "worktree-structured.yaml")
	content := []byte(`version: "1"
name: worktree-structured
nodes:
  - id: ok
    kind: noop
worktree:
  enabled: true
  provider: pi
  base: current
  merge:
    strategy: deterministic
    on_conflict: agent
  cleanup:
    on_success: false
    on_failure: keep
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Worktree.Enabled {
		t.Fatal("expected worktree enabled")
	}
	if spec.Worktree.Provider != "pi" {
		t.Fatalf("expected provider pi, got %q", spec.Worktree.Provider)
	}
	if spec.Worktree.Cleanup.OnSuccess != nil && *spec.Worktree.Cleanup.OnSuccess {
		t.Fatal("expected cleanup on_success false")
	}
}

func TestDecodeWorktreeConflict(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "worktree-conflict.yaml")
	content := []byte(`version: "1"
name: worktree-conflict
nodes:
  - id: ok
    kind: noop
worktree:
  enabled: true
  provider: codex
worktree-provider: pi
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(path)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "conflicts with worktree-provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeWorkflowFile(t *testing.T, root string, filename string, name string, description string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filename)
	content := []byte("version: \"1\"\nname: " + name + "\ndescription: " + description + "\nnodes:\n  - id: ok\n    kind: noop\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
