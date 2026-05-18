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

func TestWorkflowRepositoryLoadAcceptsDirectJSONWorkflowPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "direct.json")
	content := []byte(`{"version":"1","name":"json-workflow","nodes":[{"id":"ok","kind":"noop"}]}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := NewWorkflowRepository(filepath.Join(root, "missing-local"), filepath.Join(root, "missing-global"))
	spec, sourcePath, err := repo.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "json-workflow" {
		t.Fatalf("expected json workflow, got %q", spec.Name)
	}
	if sourcePath != filepath.Clean(path) {
		t.Fatalf("expected direct json source path, got %q", sourcePath)
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

func TestGoalV2WorkflowValidates(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, ".agentflow", "workflows", "goal-v2.yaml")

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != workflow.WorkflowVersion2 {
		t.Fatalf("expected version %q, got %q", workflow.WorkflowVersion2, spec.Version)
	}
	if spec.Inputs == nil || spec.Inputs["goal"].Type != "string" || !spec.Inputs["goal"].Required {
		t.Fatalf("expected required string goal input, got %#v", spec.Inputs["goal"])
	}
	if len(spec.Outputs) == 0 {
		t.Fatal("expected workflow outputs to be decoded")
	}
	if _, ok := spec.Outputs["plan_matrix"]; !ok {
		t.Fatal("expected plan_matrix output")
	}
	if _, ok := spec.Outputs["verification"]; !ok {
		t.Fatal("expected verification output")
	}
	if len(spec.Nodes) != 5 {
		t.Fatalf("expected 5 expanded nodes, got %d", len(spec.Nodes))
	}
	verifyNode := spec.Nodes[len(spec.Nodes)-1]
	if verifyNode.ID != "verify_goal" {
		t.Fatalf("expected final node verify_goal, got %q", verifyNode.ID)
	}
	if verifyNode.GoToIf == nil || verifyNode.GoToIf.Target != "draft_goal_spec" {
		t.Fatalf("expected verify_goal to loop back to draft_goal_spec, got %#v", verifyNode.GoToIf)
	}

	if err := workflow.Validate(spec, workflow.DefaultProviders(), nil); err != nil {
		t.Fatalf("expected goal-v2 workflow to validate: %v", err)
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
	if spec.Worktree.Provider != "codex" {
		t.Fatalf("expected provider codex, got %q", spec.Worktree.Provider)
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

func TestDecodeV2FieldsDoesNotBreakKnownFields(t *testing.T) {
	root := t.TempDir()
	importPath := filepath.Join(root, "other.yaml")
	importContent := []byte(`version: "2"
name: other
nodes:
  - id: other-node
    kind: noop
`)
	if err := os.WriteFile(importPath, importContent, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "v2.yaml")
	content := []byte(`version: "2"
name: v2-fields
nodes:
  - id: ok
    kind: noop
imports:
  - path: other.yaml
outputs:
  result:
    value: "ok"
    type: string
hooks:
  - kind: pre
    command: echo hello
steps:
  myStep:
    parameters:
      - name
    nodes:
      - id: stepNode
        kind: noop
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatalf("expected v2 fields to decode, got %v", err)
	}
	if spec.Version != workflow.WorkflowVersion2 {
		t.Fatalf("expected version %q, got %q", workflow.WorkflowVersion2, spec.Version)
	}
	// Imports are consumed during load and no longer present in the final spec.
	if len(spec.Imports) != 0 {
		t.Fatalf("expected imports to be resolved and cleared, got %#v", spec.Imports)
	}
	if len(spec.Outputs) != 1 {
		t.Fatalf("expected outputs decoded, got %#v", spec.Outputs)
	}
	if len(spec.Hooks) != 1 || spec.Hooks[0].Kind != "pre" {
		t.Fatalf("expected hooks decoded, got %#v", spec.Hooks)
	}
	if len(spec.Steps) != 1 {
		t.Fatalf("expected steps decoded, got %#v", spec.Steps)
	}
}

func TestDecodeV2ImportScalarPath(t *testing.T) {
	root := t.TempDir()
	importPath := filepath.Join(root, "imported.yaml")
	importContent := []byte(`version: "2"
name: imported
nodes:
  - id: imported-node
    kind: noop
`)
	if err := os.WriteFile(importPath, importContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main
imports:
  - imported.yaml
nodes:
  - id: local-node
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(spec.Nodes))
	}
}

func TestDecodeV2ImportMixedScalarAndObjectPaths(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.yaml")
	firstContent := []byte(`version: "2"
name: first
nodes:
  - id: first-node
    kind: noop
`)
	if err := os.WriteFile(firstPath, firstContent, 0o644); err != nil {
		t.Fatal(err)
	}
	secondPath := filepath.Join(root, "second.yaml")
	secondContent := []byte(`version: "2"
name: second
nodes:
  - id: second-node
    kind: noop
`)
	if err := os.WriteFile(secondPath, secondContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main-mixed
imports:
  - first.yaml
  - path: second.yaml
nodes:
  - id: local-node
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(spec.Nodes))
	}
}

func TestDecodeV2ImportRejectsEmptyPath(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main-empty-import
imports:
  - ""
nodes:
  - id: local-node
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(mainPath)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "import path is required") {
		t.Fatalf("expected import path error, got %v", err)
	}
}

func TestDecodeV2InputSchemaDoesNotBreakKnownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "v2-input-schema.yaml")
	content := []byte(`version: "2"
name: v2-input-schema
inputs:
  count:
    type: integer
    schema:
      minimum: 0
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatalf("expected v2 input schema to decode, got %v", err)
	}
	input, ok := spec.Inputs["count"]
	if !ok {
		t.Fatal("expected input 'count' decoded")
	}
	if input.Schema == nil {
		t.Fatal("expected input schema decoded")
	}
}

func TestDecodeV2NodeRefDoesNotBreakKnownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "v2-node-ref.yaml")
	content := []byte(`version: "2"
name: v2-node-ref
steps:
  myStep:
    parameters:
      - name
    nodes:
      - id: stepNode
        kind: noop
nodes:
  - id: ok
    kind: noop
    ref: myStep
    params:
      name: world
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatalf("expected v2 node ref to decode, got %v", err)
	}
	if len(spec.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(spec.Nodes))
	}
	if spec.Nodes[0].ID != "ok" {
		t.Fatalf("expected node id ok after expansion, got %q", spec.Nodes[0].ID)
	}
	if spec.Nodes[0].Ref != "" {
		t.Fatalf("expected ref to be cleared after expansion, got %q", spec.Nodes[0].Ref)
	}
}

func TestDecodeV2ImportSimple(t *testing.T) {
	root := t.TempDir()
	importPath := filepath.Join(root, "imported.yaml")
	importContent := []byte(`version: "2"
name: imported
inputs:
  count:
    type: integer
nodes:
  - id: imported-node
    kind: noop
`)
	if err := os.WriteFile(importPath, importContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main
imports:
  - path: imported.yaml
nodes:
  - id: local-node
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(spec.Nodes))
	}
	if spec.Nodes[0].ID != "imported-node" {
		t.Fatalf("expected first node imported-node, got %q", spec.Nodes[0].ID)
	}
	if spec.Nodes[1].ID != "local-node" {
		t.Fatalf("expected second node local-node, got %q", spec.Nodes[1].ID)
	}
	if _, ok := spec.Inputs["count"]; !ok {
		t.Fatal("expected imported input 'count'")
	}
}

func TestDecodeV2ImportChain(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yaml")
	baseContent := []byte(`version: "2"
name: base
nodes:
  - id: base-node
    kind: noop
`)
	if err := os.WriteFile(basePath, baseContent, 0o644); err != nil {
		t.Fatal(err)
	}

	midPath := filepath.Join(root, "mid.yaml")
	midContent := []byte(`version: "2"
name: mid
imports:
  - path: base.yaml
nodes:
  - id: mid-node
    kind: noop
`)
	if err := os.WriteFile(midPath, midContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main
imports:
  - path: mid.yaml
nodes:
  - id: main-node
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(spec.Nodes))
	}
	ids := []string{spec.Nodes[0].ID, spec.Nodes[1].ID, spec.Nodes[2].ID}
	if ids[0] != "base-node" || ids[1] != "mid-node" || ids[2] != "main-node" {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestDecodeV2ImportCycle(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "a.yaml")
	aContent := []byte(`version: "2"
name: a
imports:
  - path: b.yaml
nodes:
  - id: a-node
    kind: noop
`)
	if err := os.WriteFile(aPath, aContent, 0o644); err != nil {
		t.Fatal(err)
	}

	bPath := filepath.Join(root, "b.yaml")
	bContent := []byte(`version: "2"
name: b
imports:
  - path: a.yaml
nodes:
  - id: b-node
    kind: noop
`)
	if err := os.WriteFile(bPath, bContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(aPath)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !strings.Contains(err.Error(), "import cycle:") {
		t.Fatalf("expected import cycle error, got %v", err)
	}
}

func TestDecodeV2ImportNodeConflict(t *testing.T) {
	root := t.TempDir()
	importPath := filepath.Join(root, "imported.yaml")
	importContent := []byte(`version: "2"
name: imported
nodes:
  - id: shared
    kind: noop
`)
	if err := os.WriteFile(importPath, importContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main
imports:
  - path: imported.yaml
nodes:
  - id: shared
    kind: bash
    command: echo ok
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(mainPath)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "node id conflict after merge") {
		t.Fatalf("expected node conflict error, got %v", err)
	}
}

func TestDecodeV2ImportStepConflict(t *testing.T) {
	root := t.TempDir()
	importPath := filepath.Join(root, "imported.yaml")
	importContent := []byte(`version: "2"
name: imported
steps:
  notify:
    parameters:
      - msg
    nodes:
      - id: n
        kind: noop
`)
	if err := os.WriteFile(importPath, importContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "main.yaml")
	mainContent := []byte(`version: "2"
name: main
imports:
  - path: imported.yaml
steps:
  notify:
    parameters:
      - msg
    nodes:
      - id: n
        kind: noop
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(mainPath, mainContent, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(mainPath)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "step name conflict") {
		t.Fatalf("expected step conflict error, got %v", err)
	}
}

func TestDecodeV2StepExpansionProducesValidPlan(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "macro.yaml")
	content := []byte(`version: "2"
name: macro
steps:
  greet:
    parameters:
      - name
    nodes:
      - id: say
        kind: bash
        command: "echo ${name}"
nodes:
  - id: hello
    ref: greet
    params:
      name: world
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Validate(spec, workflow.DefaultProviders(), nil); err != nil {
		t.Fatalf("expected validation to pass: %v", err)
	}
	plan, err := workflow.BuildPlan(*spec)
	if err != nil {
		t.Fatalf("expected plan to build: %v", err)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("expected 1 planned node, got %d", len(plan.Nodes))
	}
	if _, ok := plan.Nodes["hello"]; !ok {
		t.Fatalf("expected planned node hello")
	}
}

func TestDecodeV1RejectsImportsAtLoadTime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "v1-import.yaml")
	content := []byte(`version: "1"
name: v1-import
imports:
  - path: other.yaml
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := decodeWorkflow(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `imports are not supported in workflow version "1"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeArtifacts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifacts.yaml")
	content := []byte(`version: "1"
name: artifacts
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
    artifacts:
      - name: report
        path: reports/security.md
        media_type: text/markdown
        description: Security scan results
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	spec, err := decodeWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := spec.NodeByID("shell")
	if !ok {
		t.Fatal("expected shell node")
	}
	if len(node.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(node.Artifacts))
	}
	art := node.Artifacts[0]
	if art.Name != "report" {
		t.Fatalf("expected artifact name report, got %q", art.Name)
	}
	if art.Path != "reports/security.md" {
		t.Fatalf("expected artifact path reports/security.md, got %q", art.Path)
	}
	if art.MediaType != "text/markdown" {
		t.Fatalf("expected artifact media_type text/markdown, got %q", art.MediaType)
	}
	if art.Description != "Security scan results" {
		t.Fatalf("expected artifact description, got %q", art.Description)
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
