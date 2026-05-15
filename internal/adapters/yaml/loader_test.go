package yaml

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
