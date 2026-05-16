package adapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/desktop/runtime"
)

type mockFS struct {
	files map[string][]byte
	dirs  map[string][]os.DirEntry
}

func (m *mockFS) ReadFile(name string) ([]byte, error) {
	if data, ok := m.files[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if m.files == nil {
		m.files = make(map[string][]byte)
	}
	m.files[name] = data
	return nil
}

func (m *mockFS) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (m *mockFS) ReadDir(name string) ([]os.DirEntry, error) {
	if entries, ok := m.dirs[name]; ok {
		return entries, nil
	}
	return nil, os.ErrNotExist
}

type mockWorkflowRepo struct {
	spec *workflow.WorkflowSpec
	path string
	err  error
}

func (m *mockWorkflowRepo) Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error) {
	if m.err != nil {
		return nil, "", m.err
	}
	if m.spec != nil {
		return m.spec, m.path, nil
	}
	return nil, "", os.ErrNotExist
}

type mockSettingsStore struct {
	settings AppSettings
	err      error
}

func (m *mockSettingsStore) Load() (AppSettings, error) {
	if m.err != nil {
		return AppSettings{}, m.err
	}
	return m.settings, nil
}

func (m *mockSettingsStore) Save(settings AppSettings) error {
	m.settings = settings
	return m.err
}

func TestAdapter_LoadWorkflow(t *testing.T) {
	spec := &workflow.WorkflowSpec{
		Version:     "1",
		Name:        "test",
		Description: "desc",
		Inputs: map[string]workflow.InputSpec{
			"files": {Type: "array", Required: true},
		},
		Nodes: []workflow.NodeSpec{
			{ID: "step1", Kind: workflow.NodeKindBash, Command: "echo hi"},
		},
	}

	fs := &mockFS{
		files: map[string][]byte{
			"/tmp/test.yaml": []byte("version: \"1\"\nname: test"),
		},
	}

	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/test.yaml"}, nil, &mockSettingsStore{}, fs, nil)

	got, err := a.LoadWorkflow(context.Background(), "/tmp/test.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected name test, got %s", got.Name)
	}
	if got.Version != "1" {
		t.Errorf("expected version 1, got %s", got.Version)
	}
	if len(got.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(got.Nodes))
	}
	if len(got.Inputs) != 1 {
		t.Errorf("expected 1 input, got %d", len(got.Inputs))
	}
	if got.RawYAML == "" {
		t.Error("expected raw yaml")
	}
}

func TestAdapter_LoadWorkflow_NotFound(t *testing.T) {
	a := NewAdapter(&mockWorkflowRepo{}, nil, &mockSettingsStore{}, &mockFS{}, nil)

	_, err := a.LoadWorkflow(context.Background(), "/tmp/missing.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	de, ok := err.(DesktopError)
	if !ok {
		t.Fatalf("expected DesktopError, got %T", err)
	}
	if de.Code != ErrCodeWorkflowNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeWorkflowNotFound, de.Code)
	}
}

func TestAdapter_ValidateWorkflow(t *testing.T) {
	// Precisa de um workflow real para validar; usamos o sample do projeto.
	a := NewDefaultAdapter()

	// Caminho invalido deve retornar erro normalizado.
	got := a.ValidateWorkflow(context.Background(), "non-existent.yaml")
	if got.Valid {
		t.Error("expected invalid")
	}
	if len(got.Errors) == 0 {
		t.Error("expected errors")
	}
}

func TestAdapter_GenerateGraph(t *testing.T) {
	a := NewDefaultAdapter()

	got := a.GenerateGraph(context.Background(), "non-existent.yaml")
	if got.Valid {
		t.Error("expected invalid")
	}
	if len(got.Errors) == 0 {
		t.Error("expected errors")
	}
}

func TestAdapter_ResolveInput(t *testing.T) {
	spec := &workflow.WorkflowSpec{
		Version: "1",
		Name:    "input-test",
		Inputs: map[string]workflow.InputSpec{
			"name": {Type: "string", Required: true},
			"age":  {Type: "integer", Default: 30},
		},
		Nodes: []workflow.NodeSpec{
			{ID: "step1", Kind: workflow.NodeKindNoop},
		},
	}

	fs := &mockFS{
		files: map[string][]byte{
			"/tmp/input-test.yaml": []byte("version: \"1\"\nname: input-test"),
		},
	}

	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/input-test.yaml"}, nil, &mockSettingsStore{}, fs, nil)

	resolved, err := a.ResolveInput(context.Background(), "/tmp/input-test.yaml", map[string]any{
		"name": "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", resolved["name"])
	}
	if resolved["age"] != 30 {
		t.Errorf("expected age 30, got %v", resolved["age"])
	}
}

func TestAdapter_DryRunWorkflow(t *testing.T) {
	a := NewDefaultAdapter()

	got := a.DryRunWorkflow(context.Background(), "non-existent.yaml", nil, nil, 0, ".")
	if got.Valid {
		t.Error("expected invalid")
	}
	if len(got.Errors) == 0 {
		t.Error("expected errors")
	}
}

func TestAdapter_SaveWorkflow(t *testing.T) {
	fs := &mockFS{}
	a := NewAdapter(nil, nil, &mockSettingsStore{}, fs, nil)

	err := a.SaveWorkflow("/tmp/workflows/test.yaml", "version: \"1\"\nname: test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := fs.files["/tmp/workflows/test.yaml"]; !ok {
		t.Error("expected file to be saved")
	}
}

func TestAdapter_SaveInput(t *testing.T) {
	fs := &mockFS{}
	a := NewAdapter(nil, nil, &mockSettingsStore{}, fs, nil)

	err := a.SaveInput("/tmp/inputs/test.json", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := fs.files["/tmp/inputs/test.json"]; !ok {
		t.Error("expected file to be saved")
	}
}

func TestAdapter_GetUpdateAppSettings(t *testing.T) {
	store := &mockSettingsStore{settings: defaultSettings()}
	a := NewAdapter(nil, nil, store, &mockFS{}, nil)

	settings, err := a.GetAppSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Theme != "system" {
		t.Errorf("expected default theme system, got %s", settings.Theme)
	}

	settings.Theme = "dark"
	if err := a.UpdateAppSettings(settings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := a.GetAppSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Theme != "dark" {
		t.Errorf("expected theme dark, got %s", got.Theme)
	}
}

func TestAdapter_ListWorkflows(t *testing.T) {
	fs := &mockFS{
		dirs: map[string][]os.DirEntry{
			"/cwd/.agentflow/workflows": {
				&mockDirEntry{name: "wf1.yaml", isDir: false},
				&mockDirEntry{name: "readme.md", isDir: false},
			},
		},
		files: map[string][]byte{
			"/cwd/.agentflow/workflows/wf1.yaml": []byte("name: wf1\ndescription: desc1"),
		},
	}

	oldGetwd := osGetwd
	oldUserHomeDir := osUserHomeDir
	osGetwd = func() (string, error) { return "/cwd", nil }
	osUserHomeDir = func() (string, error) { return "/home", nil }
	defer func() {
		osGetwd = oldGetwd
		osUserHomeDir = oldUserHomeDir
	}()

	a := NewAdapter(nil, nil, &mockSettingsStore{}, fs, nil)

	list, err := a.ListWorkflows()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(list))
	}
	if list[0].Name != "wf1" {
		t.Errorf("expected name wf1, got %s", list[0].Name)
	}
	if list[0].Description != "desc1" {
		t.Errorf("expected description desc1, got %s", list[0].Description)
	}
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode          { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

var (
	_ ports.WorkflowRepository = (*mockWorkflowRepo)(nil)
	_ runworkflow.RunOptions   = runworkflow.RunOptions{}
)

func writeTestWorkflow(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return path
}

func TestAdapter_ValidateWorkflow_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTestWorkflow(t, tmp, "wf.yaml", `version: "1"
name: test-wf
nodes:
  - id: a
    kind: noop
`)
	a := NewDefaultAdapter()
	got := a.ValidateWorkflow(context.Background(), path)
	if !got.Valid {
		t.Fatalf("expected valid, got errors: %+v", got.Errors)
	}
	if got.Name != "test-wf" {
		t.Errorf("expected name test-wf, got %s", got.Name)
	}
	if got.NodeCount != 1 {
		t.Errorf("expected 1 node, got %d", got.NodeCount)
	}
}

func TestAdapter_GenerateGraph_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTestWorkflow(t, tmp, "wf.yaml", `version: "1"
name: test-wf
nodes:
  - id: a
    kind: noop
`)
	a := NewDefaultAdapter()
	got := a.GenerateGraph(context.Background(), path)
	if !got.Valid {
		t.Fatalf("expected valid, got errors: %+v", got.Errors)
	}
	if got.Mermaid == "" {
		t.Error("expected non-empty mermaid")
	}
}

func TestAdapter_DryRunWorkflow_Valid(t *testing.T) {
	tmp := t.TempDir()
	path := writeTestWorkflow(t, tmp, "wf.yaml", `version: "1"
name: test-wf
nodes:
  - id: a
    kind: noop
`)
	a := NewDefaultAdapter()
	got := a.DryRunWorkflow(context.Background(), path, nil, nil, 0, ".")
	if !got.Valid {
		t.Fatalf("expected valid, got errors: %+v", got.Errors)
	}
	if got.Workflow != "test-wf" {
		t.Errorf("expected workflow test-wf, got %s", got.Workflow)
	}
	if len(got.Order) != 1 {
		t.Errorf("expected 1 node in order, got %d", len(got.Order))
	}
}

func TestAdapter_ResolveInput_Error(t *testing.T) {
	spec := &workflow.WorkflowSpec{
		Version: "1",
		Name:    "input-test",
		Inputs: map[string]workflow.InputSpec{
			"name": {Type: "string", Required: true},
		},
		Nodes: []workflow.NodeSpec{{ID: "step1", Kind: workflow.NodeKindNoop}},
	}
	fs := &mockFS{
		files: map[string][]byte{
			"/tmp/input-test.yaml": []byte("version: \"1\"\nname: input-test"),
		},
	}
	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/input-test.yaml"}, nil, &mockSettingsStore{}, fs, nil)
	_, err := a.ResolveInput(context.Background(), "/tmp/input-test.yaml", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required input")
	}
	de, ok := err.(DesktopError)
	if !ok {
		t.Fatalf("expected DesktopError, got %T", err)
	}
	if de.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, de.Code)
	}
}

func TestAdapter_RunWorkflow_Cancel_List_Get(t *testing.T) {
	tmp := t.TempDir()
	path := writeTestWorkflow(t, tmp, "wf.yaml", `version: "1"
name: test-wf
nodes:
  - id: a
    kind: noop
`)
	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := a.RunWorkflow(ctx, RunWorkflowRequest{WorkflowRef: path})
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	if summary.ID == "" {
		t.Fatal("expected run id")
	}

	// ListRuns deve retornar a run ativa
	list, err := a.ListRuns()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(list.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list.Runs))
	}

	// GetRun deve retornar a mesma run
	got, err := a.GetRun(summary.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ID != summary.ID {
		t.Errorf("expected run id %s, got %s", summary.ID, got.ID)
	}

	// CancelRun
	cancelled, err := a.CancelRun(summary.ID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Errorf("expected cancelled, got %s", cancelled.Status)
	}

	// Aguardar a goroutine da run terminar para nao vazar arquivos no TempDir
	for i := 0; i < 100; i++ {
		got, _ := a.GetRun(summary.ID)
		if got.Status == "cancelled" || got.Status == "success" || got.Status == "failed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Buffer extra para finalizacao de IO interna
	time.Sleep(200 * time.Millisecond)
}

func TestAdapter_GetRunArtifacts(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts", "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "file.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "subdir", "nested.txt"), []byte("world"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunArtifacts("run-1")
	if err != nil {
		t.Fatalf("get artifacts: %v", err)
	}
	if len(resp.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(resp.Artifacts))
	}
}

func TestAdapter_GetRunArtifact(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "file.txt"), []byte("hello"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunArtifact("run-1", "file.txt")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if resp.Name != "file.txt" {
		t.Errorf("expected name file.txt, got %s", resp.Name)
	}
	decoded, err := os.ReadFile(filepath.Join(runDir, "artifacts", "file.txt"))
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected encoded content")
	}
	// verify base64 roundtrip indirectly via size
	if resp.Size != int64(len(decoded)) {
		t.Errorf("expected size %d, got %d", len(decoded), resp.Size)
	}
}

func TestAdapter_GetRunArtifact_PathTraversal(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "file.txt"), []byte("hello"), 0o644)
	// arquivo fora do artifacts
	_ = os.WriteFile(filepath.Join(runDir, "secret.txt"), []byte("secret"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	_, err := a.GetRunArtifact("run-1", "../secret.txt")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	de, ok := err.(DesktopError)
	if !ok {
		t.Fatalf("expected DesktopError, got %T", err)
	}
	if de.Code != ErrCodeInvalidPath {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidPath, de.Code)
	}
}

func TestAdapter_GetRunNodes(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "nodes", "a"), 0o755)
	result := map[string]any{
		"node_id": "a", "status": "success", "output": "done",
	}
	data, _ := json.Marshal(result)
	_ = os.WriteFile(filepath.Join(runDir, "nodes", "a", "result.json"), data, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunNodes("run-1")
	if err != nil {
		t.Fatalf("get nodes: %v", err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	if resp.Nodes[0].NodeID != "a" {
		t.Errorf("expected node id a, got %s", resp.Nodes[0].NodeID)
	}
}

func TestAdapter_GetRunNode(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "nodes", "a"), 0o755)
	result := map[string]any{
		"node_id": "a", "status": "success", "output": "done",
	}
	data, _ := json.Marshal(result)
	_ = os.WriteFile(filepath.Join(runDir, "nodes", "a", "result.json"), data, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunNode("run-1", "a")
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if resp.NodeID != "a" {
		t.Errorf("expected node id a, got %s", resp.NodeID)
	}
	if len(resp.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(resp.Instances))
	}

	_, err = a.GetRunNode("run-1", "missing")
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestAdapter_GetRunPlan(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(runDir, 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "run.json"), []byte(`{"run_id":"run-1"}`), 0o644)
	_ = os.WriteFile(filepath.Join(runDir, "workflow.yaml"), []byte("name: wf"), 0o644)
	_ = os.WriteFile(filepath.Join(runDir, "normalized.json"), []byte(`{"a":1}`), 0o644)
	_ = os.WriteFile(filepath.Join(runDir, "plan.json"), []byte(`{"order":["a"]}`), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunPlan("run-1")
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if resp.RunID != "run-1" {
		t.Errorf("expected run id run-1, got %s", resp.RunID)
	}
	if resp.Workflow != "name: wf" {
		t.Errorf("expected workflow content, got %s", resp.Workflow)
	}
}

func TestAdapter_GetRunLogs(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(runDir, 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("line1\nline2\n"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunLogs("run-1")
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(resp.Lines))
	}
	if resp.Lines[0] != "line1" {
		t.Errorf("expected line1, got %s", resp.Lines[0])
	}
}

func TestAdapter_GetRunEvents(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(runDir, 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte(
		`{"ts":"2024-01-01T00:00:00Z","run_id":"run-1","type":"run.started"}`+"\n"+
			`{"ts":"2024-01-01T00:00:01Z","run_id":"run-1","type":"node.started","node_id":"a"}`+"\n"+
			`{"ts":"2024-01-01T00:00:02Z","run_id":"run-1","type":"node.completed","node_id":"a"}`+"\n",
	), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, &mockFS{}, m)

	resp, err := a.GetRunEvents("run-1", 0, 2)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}
	if !resp.HasMore {
		t.Error("expected has_more true")
	}
	if resp.NextCursor != 2 {
		t.Errorf("expected next cursor 2, got %d", resp.NextCursor)
	}

	// segunda pagina
	resp2, err := a.GetRunEvents("run-1", resp.NextCursor, 2)
	if err != nil {
		t.Fatalf("get events page 2: %v", err)
	}
	if len(resp2.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp2.Events))
	}
	if resp2.HasMore {
		t.Error("expected has_more false")
	}
}
