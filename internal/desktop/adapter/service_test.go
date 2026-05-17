package adapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
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

type mockProjectStore struct {
	projects []app.Project
	err      error
}

func (m *mockProjectStore) Load() ([]app.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]app.Project, len(m.projects))
	copy(out, m.projects)
	return out, nil
}

func (m *mockProjectStore) Save(projects []app.Project) error {
	if m.err != nil {
		return m.err
	}
	m.projects = make([]app.Project, len(projects))
	copy(m.projects, projects)
	return nil
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

	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/test.yaml"}, nil, &mockSettingsStore{}, nil, fs, nil)

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
	a := NewAdapter(&mockWorkflowRepo{}, nil, &mockSettingsStore{}, nil, &mockFS{}, nil)

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

	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/input-test.yaml"}, nil, &mockSettingsStore{}, nil, fs, nil)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, fs, nil)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, fs, nil)

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
	a := NewAdapter(nil, nil, store, nil, &mockFS{}, nil)

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

func TestAdapter_ProjectCRUD(t *testing.T) {
	projectStore := &mockProjectStore{projects: []app.Project{{Name: "demo", Path: "/tmp/demo"}}}
	a := NewAdapter(nil, nil, &mockSettingsStore{}, app.NewProjectRegistry(projectStore), &mockFS{}, nil)

	projects, err := a.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	if err := a.AddProject("extra", "/tmp/extra"); err != nil {
		t.Fatalf("add project: %v", err)
	}
	if len(projectStore.projects) != 2 {
		t.Fatalf("expected 2 stored projects, got %d", len(projectStore.projects))
	}

	if err := a.RemoveProject("demo"); err != nil {
		t.Fatalf("remove project: %v", err)
	}
	if len(projectStore.projects) != 1 || projectStore.projects[0].Name != "extra" {
		t.Fatalf("unexpected projects after remove: %#v", projectStore.projects)
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

	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, fs, nil)

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
	a := NewAdapter(&mockWorkflowRepo{spec: spec, path: "/tmp/input-test.yaml"}, nil, &mockSettingsStore{}, nil, fs, nil)
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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifacts("run-1")
	if err != nil {
		t.Fatalf("get artifacts: %v", err)
	}
	if len(resp.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(resp.Artifacts))
	}
}

func TestAdapter_GetRunArtifacts_Index(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "hello.txt"), []byte("hello"), 0o644)

	index := map[string]corerun.Artifact{
		"nodes/a/stdout.txt": {
			ID:           "nodes/a/stdout.txt",
			Name:         "stdout.txt",
			RelativePath: "nodes/a/stdout.txt",
			MediaType:    "text/plain",
			SizeBytes:    5,
			Kind:         corerun.ArtifactKindStdout,
			NodeID:       "a",
		},
	}
	idxData, _ := json.Marshal(index)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), idxData, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifacts("run-1")
	if err != nil {
		t.Fatalf("get artifacts: %v", err)
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact from index, got %d", len(resp.Artifacts))
	}
	art := resp.Artifacts[0]
	if art.ID != "nodes/a/stdout.txt" {
		t.Errorf("expected id nodes/a/stdout.txt, got %s", art.ID)
	}
	if art.Kind != corerun.ArtifactKindStdout {
		t.Errorf("expected kind stdout, got %s", art.Kind)
	}
	if art.NodeID != "a" {
		t.Errorf("expected node_id a, got %s", art.NodeID)
	}
	if art.MediaType != "text/plain" {
		t.Errorf("expected media_type text/plain, got %s", art.MediaType)
	}
}

func TestAdapter_GetRunArtifacts_Fallback(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "legacy.txt"), []byte("legacy"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifacts("run-1")
	if err != nil {
		t.Fatalf("get artifacts: %v", err)
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact from fallback scan, got %d", len(resp.Artifacts))
	}
	if resp.Artifacts[0].Name != "legacy.txt" {
		t.Errorf("expected name legacy.txt, got %s", resp.Artifacts[0].Name)
	}
}

func TestAdapter_GetRunArtifact_Text(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts", "nodes", "a"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "nodes", "a", "stdout.txt"), []byte("hello"), 0o644)

	index := map[string]corerun.Artifact{
		"nodes/a/stdout.txt": {
			ID:           "nodes/a/stdout.txt",
			Name:         "stdout.txt",
			RelativePath: "nodes/a/stdout.txt",
			MediaType:    "text/plain",
			SizeBytes:    5,
			Kind:         corerun.ArtifactKindStdout,
			NodeID:       "a",
		},
	}
	idxData, _ := json.Marshal(index)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), idxData, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifact("run-1", "nodes/a/stdout.txt")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if resp.Name != "stdout.txt" {
		t.Errorf("expected name stdout.txt, got %s", resp.Name)
	}
	if !resp.IsText {
		t.Error("expected is_text true")
	}
	if resp.TextContent != "hello" {
		t.Errorf("expected text_content hello, got %s", resp.TextContent)
	}
	if resp.Encoding != "text" {
		t.Errorf("expected encoding text, got %s", resp.Encoding)
	}
}

func TestAdapter_GetRunArtifact_Binary(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "image.png"), []byte{0x89, 0x50, 0x4E, 0x47}, 0o644)

	index := map[string]corerun.Artifact{
		"image.png": {
			ID:           "image.png",
			Name:         "image.png",
			RelativePath: "image.png",
			MediaType:    "image/png",
			SizeBytes:    4,
			Kind:         corerun.ArtifactKindCustom,
		},
	}
	idxData, _ := json.Marshal(index)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), idxData, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifact("run-1", "image.png")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if resp.IsText {
		t.Error("expected is_text false for binary")
	}
	if resp.TextContent != "" {
		t.Error("expected empty text_content for binary")
	}
	if !resp.Truncated {
		t.Error("expected truncated true for binary")
	}
}

func TestAdapter_GetRunArtifact_Truncated(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	large := make([]byte, MaxArtifactInline+100)
	for i := range large {
		large[i] = 'a'
	}
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "large.txt"), large, 0o644)

	index := map[string]corerun.Artifact{
		"large.txt": {
			ID:           "large.txt",
			Name:         "large.txt",
			RelativePath: "large.txt",
			MediaType:    "text/plain",
			SizeBytes:    int64(len(large)),
			Kind:         corerun.ArtifactKindFile,
		},
	}
	idxData, _ := json.Marshal(index)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), idxData, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifact("run-1", "large.txt")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if !resp.IsText {
		t.Error("expected is_text true")
	}
	if !resp.Truncated {
		t.Error("expected truncated true")
	}
	if len(resp.TextContent) != MaxArtifactInline {
		t.Errorf("expected text_content length %d, got %d", MaxArtifactInline, len(resp.TextContent))
	}
}

func TestAdapter_GetRunArtifactPath(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts", "nodes", "a"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "nodes", "a", "stdout.txt"), []byte("hello"), 0o644)

	index := map[string]corerun.Artifact{
		"nodes/a/stdout.txt": {
			ID:           "nodes/a/stdout.txt",
			Name:         "stdout.txt",
			RelativePath: "nodes/a/stdout.txt",
			MediaType:    "text/plain",
			SizeBytes:    5,
			Kind:         corerun.ArtifactKindStdout,
			NodeID:       "a",
		},
	}
	idxData, _ := json.Marshal(index)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), idxData, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	path, err := a.GetRunArtifactPath("run-1", "nodes/a/stdout.txt")
	if err != nil {
		t.Fatalf("get artifact path: %v", err)
	}
	if !strings.Contains(path, "artifacts/nodes/a/stdout.txt") {
		t.Errorf("expected path to contain artifacts/nodes/a/stdout.txt, got %s", path)
	}
}

func TestAdapter_GetRunArtifactPath_NotIndexed(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "legacy.txt"), []byte("legacy"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	_, err := a.GetRunArtifactPath("run-1", "legacy.txt")
	if err == nil {
		t.Fatal("expected error for non-indexed run")
	}
	de, ok := err.(DesktopError)
	if !ok {
		t.Fatalf("expected DesktopError, got %T", err)
	}
	if de.Code != ErrCodeWorkflowNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeWorkflowNotFound, de.Code)
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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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

func TestAdapter_GetRunArtifact_Fallback(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "artifacts", "fallback.txt"), []byte("fallback content"), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunArtifact("run-1", "fallback.txt")
	if err != nil {
		t.Fatalf("get artifact fallback: %v", err)
	}
	if resp.Name != "fallback.txt" {
		t.Errorf("expected name fallback.txt, got %s", resp.Name)
	}
	if !resp.IsText {
		t.Error("expected is_text true for fallback text file")
	}
	if resp.TextContent != "fallback content" {
		t.Errorf("expected text_content fallback content, got %s", resp.TextContent)
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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

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

func TestAdapter_GetRunDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	// Sem runs deve retornar erro
	_, err := a.GetRunDiagnostics("missing")
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestAdapter_GetRunTimeline(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(runDir, 0o755)
	_ = os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte(
		`{"ts":"2024-01-01T00:00:00Z","run_id":"run-1","type":"run.started"}`+"\n",
	), 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	resp, err := a.GetRunTimeline("run-1", 0, 10)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Type != "run.started" {
		t.Errorf("expected type run.started, got %s", resp.Entries[0].Type)
	}
}

func TestAdapter_GetRunChartSeries(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(filepath.Join(runDir, "nodes", "a"), 0o755)
	result := map[string]any{"node_id": "a", "status": "success", "duration_ms": 150, "attempts": 2}
	data, _ := json.Marshal(result)
	_ = os.WriteFile(filepath.Join(runDir, "nodes", "a", "result.json"), data, 0o644)

	m := runtime.NewManager(tmp, "", "", "", "")
	a := NewAdapter(nil, nil, &mockSettingsStore{}, nil, &mockFS{}, m)

	series, err := a.GetRunChartSeries("run-1")
	if err != nil {
		t.Fatalf("get chart series: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(series))
	}
	if series[0].Name != "Duration (ms)" {
		t.Errorf("expected first series Duration (ms), got %s", series[0].Name)
	}
	if series[1].Name != "Retries" {
		t.Errorf("expected second series Retries, got %s", series[1].Name)
	}
}

func TestAdapter_toRunSummary_WithDiagnostics(t *testing.T) {
	rs := runtime.RunSummary{
		ID:            "r1",
		Workflow:      "wf1",
		Status:        "success",
		DurationMS:    1234,
		FailedNodes:   1,
		Retries:       2,
		AgentCalls:    3,
		BashCalls:     4,
		ArtifactCount: 5,
		NodeCount:     6,
		FirstError:    "oops",
		SlowestNodes:  []corerun.SlowestNode{{NodeID: "a", DurationMS: 100}},
		AgentUsage:    []corerun.AgentUsage{{Provider: "openai", TotalTokens: 42}},
	}
	summary := toRunSummary(rs)
	if summary.DiagnosticSummary == nil {
		t.Fatal("expected diagnostic summary")
	}
	d := summary.DiagnosticSummary
	if d.DurationMS != 1234 {
		t.Errorf("expected duration 1234, got %d", d.DurationMS)
	}
	if d.FailedNodes != 1 {
		t.Errorf("expected failed 1, got %d", d.FailedNodes)
	}
	if d.Retries != 2 {
		t.Errorf("expected retries 2, got %d", d.Retries)
	}
	if d.AgentCalls != 3 {
		t.Errorf("expected agent calls 3, got %d", d.AgentCalls)
	}
	if d.BashCalls != 4 {
		t.Errorf("expected bash calls 4, got %d", d.BashCalls)
	}
	if d.ArtifactCount != 5 {
		t.Errorf("expected artifacts 5, got %d", d.ArtifactCount)
	}
	if d.NodeCount != 6 {
		t.Errorf("expected nodes 6, got %d", d.NodeCount)
	}
	if d.FirstError != "oops" {
		t.Errorf("expected first error oops, got %s", d.FirstError)
	}
	if len(d.SlowestNodes) != 1 || d.SlowestNodes[0].NodeID != "a" {
		t.Errorf("unexpected slowest nodes: %+v", d.SlowestNodes)
	}
	if len(d.AgentUsage) != 1 || d.AgentUsage[0].Provider != "openai" {
		t.Errorf("unexpected agent usage: %+v", d.AgentUsage)
	}
}

func TestAdapter_toRunSummary_WithoutDiagnostics(t *testing.T) {
	rs := runtime.RunSummary{
		ID:       "r1",
		Workflow: "wf1",
		Status:   "success",
	}
	summary := toRunSummary(rs)
	if summary.DiagnosticSummary != nil {
		t.Fatal("expected nil diagnostic summary when no metrics")
	}
}
