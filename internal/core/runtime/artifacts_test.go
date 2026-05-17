package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eventmemory "github.com/diasYuri/agentflow/internal/adapters/events/memory"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	"github.com/diasYuri/agentflow/internal/core/run"
)

func TestRunWorkflowBashCopiesDeclaredArtifacts(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: bash-artifacts
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: producer
    kind: bash
    command: "mkdir -p reports && echo '# Security Report' > reports/security.md"
    artifacts:
      - name: report
        path: reports/security.md
        media_type: text/markdown
`)
	uc := newTestRunWorkflowUseCase(dir, shell.NewRunner(), eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}

	producer := result.Summary.Nodes["producer"]
	if len(producer.Artifacts) == 0 {
		t.Fatal("expected artifacts in node result")
	}

	var reportRef run.ArtifactRef
	for _, art := range producer.Artifacts {
		if art.Name == "report" {
			reportRef = art
		}
	}
	if reportRef.ID == "" {
		t.Fatalf("expected report artifact ref, got %+v", producer.Artifacts)
	}

	artifactPath := filepath.Join(result.RunDir, "artifacts", filepath.FromSlash(reportRef.ID))
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("expected artifact file to exist: %v", err)
	}
	if !strings.Contains(string(data), "# Security Report") {
		t.Fatalf("expected artifact content, got %q", string(data))
	}

	// Verify artifact index
	indexData, err := os.ReadFile(filepath.Join(result.RunDir, "artifacts", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]run.Artifact
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}
	if _, ok := index[reportRef.ID]; !ok {
		t.Fatalf("expected artifact in index: %s", reportRef.ID)
	}
}

func TestRunWorkflowIndexesStdoutStderrResultAsArtifacts(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: native-artifacts
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo hello && echo error >&2"
`)
	uc := newTestRunWorkflowUseCase(dir, shell.NewRunner(), eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}

	shell := result.Summary.Nodes["shell"]
	artifactNames := map[string]bool{}
	for _, art := range shell.Artifacts {
		artifactNames[art.Name] = true
	}
	for _, name := range []string{"result.json", "stdout.txt", "stderr.txt"} {
		if !artifactNames[name] {
			t.Fatalf("expected artifact %s in node result, got %+v", name, shell.Artifacts)
		}
	}

	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "nodes", "shell", "stdout.txt"))
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "nodes", "shell", "stderr.txt"))
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "nodes", "shell", "result.json"))
}

func TestRunWorkflowFanOutArtifactsHaveDistinctIDs(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: fanout-artifacts
inputs:
  items:
    type: array
    required: true
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: each
    kind: bash
    for_each: "${inputs.items}"
    concurrency: 2
    command: "echo ${item}"
`)
	uc := newTestRunWorkflowUseCase(dir, shell.NewRunner(), eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		Inputs:      map[string]any{"items": []any{"a", "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}

	indexData, err := os.ReadFile(filepath.Join(result.RunDir, "artifacts", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]run.Artifact
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatal(err)
	}

	instanceIDs := map[string]int{}
	for id, art := range index {
		if art.NodeID == "each" && art.Name == "stdout.txt" && art.InstanceID != "" {
			instanceIDs[art.InstanceID]++
			if !strings.HasPrefix(id, "nodes/each/") {
				t.Fatalf("expected artifact id under nodes/each/, got %s", id)
			}
		}
	}
	if len(instanceIDs) != 2 {
		t.Fatalf("expected 2 distinct instance stdout artifacts, got %+v", instanceIDs)
	}
}

func TestRunWorkflowArtifactContextExposesID(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: artifact-context
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: producer
    kind: bash
    command: "echo ok > report.txt"
    artifacts:
      - name: report
        path: report.txt
  - id: consumer
    kind: bash
    depends_on: [producer]
    command: "echo ${nodes.producer.artifacts.report.id}"
`)
	uc := newTestRunWorkflowUseCase(dir, shell.NewRunner(), eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}

	consumer := result.Summary.Nodes["consumer"]
	if consumer.Stdout == "" {
		t.Fatalf("expected consumer to output artifact id, got %#v", consumer)
	}
	if !strings.Contains(consumer.Stdout, "nodes/producer/artifacts/report") {
		t.Fatalf("expected artifact id in stdout, got %q", consumer.Stdout)
	}
}
