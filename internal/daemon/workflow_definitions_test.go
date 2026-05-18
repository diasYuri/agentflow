package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"

	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestWorkflowDefinitionCRUDPersistsInSQLiteAndFilesystem(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	manager := NewManagerWithStore(cfg, suture.NewSimple("workflow-definitions"), nil, store)
	server := NewServer(cfg, manager, time.Now(), func() {}, nil)
	ts := httptest.NewServer(server.routes())
	t.Cleanup(ts.Close)

	spec := coreworkflow.WorkflowSpec{
		Version: "1",
		Name:    "internal-crud",
		Nodes: []coreworkflow.NodeSpec{
			{ID: "ok", Kind: coreworkflow.NodeKindNoop},
		},
	}

	created := createWorkflowDefinition(t, ts.URL, spec)
	if created.WorkflowDefinition.ID == "" {
		t.Fatal("expected generated workflow definition id")
	}
	if created.WorkflowDefinition.Spec.Name != spec.Name {
		t.Fatalf("expected name %q, got %q", spec.Name, created.WorkflowDefinition.Spec.Name)
	}

	mirrorPath := filepath.Join(home, ".agentflow", "workflows", "internal", created.WorkflowDefinition.ID+".json")
	if _, err := os.Stat(mirrorPath); err != nil {
		t.Fatalf("expected mirror file at %s: %v", mirrorPath, err)
	}

	record, err := store.GetWorkflowDefinition(context.Background(), created.WorkflowDefinition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != spec.Name {
		t.Fatalf("expected db name %q, got %q", spec.Name, record.Name)
	}

	listResp := doWorkflowDefinitionRequest(t, ts.URL, http.MethodGet, "/v1/workflow-definitions", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.StatusCode)
	}
	var list WorkflowDefinitionsResponse
	mustDecodeBody(t, listResp.Body, &list)
	if len(list.Definitions) != 1 || list.Definitions[0].ID != created.WorkflowDefinition.ID {
		t.Fatalf("unexpected list response: %#v", list)
	}

	getResp := doWorkflowDefinitionRequest(t, ts.URL, http.MethodGet, "/v1/workflow-definitions/"+created.WorkflowDefinition.ID, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected get status 200, got %d", getResp.StatusCode)
	}
	var got WorkflowDefinitionResponse
	mustDecodeBody(t, getResp.Body, &got)
	if got.WorkflowDefinition.ID != created.WorkflowDefinition.ID {
		t.Fatalf("expected id %q, got %q", created.WorkflowDefinition.ID, got.WorkflowDefinition.ID)
	}

	updatedSpec := coreworkflow.WorkflowSpec{
		Version:     "1",
		Name:        "internal-crud-updated",
		Description: "updated definition",
		Nodes: []coreworkflow.NodeSpec{
			{ID: "ok", Kind: coreworkflow.NodeKindNoop},
		},
	}
	updated := doWorkflowDefinitionRequest(t, ts.URL, http.MethodPut, "/v1/workflow-definitions/"+created.WorkflowDefinition.ID, updatedSpec)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("expected update status 200, got %d", updated.StatusCode)
	}
	var updatedResp WorkflowDefinitionResponse
	mustDecodeBody(t, updated.Body, &updatedResp)
	if updatedResp.WorkflowDefinition.Spec.Name != "internal-crud-updated" {
		t.Fatalf("expected updated name, got %#v", updatedResp.WorkflowDefinition.Spec.Name)
	}
	assertFileContains(t, mirrorPath, `"name": "internal-crud-updated"`)

	deleteResp := doWorkflowDefinitionRequest(t, ts.URL, http.MethodDelete, "/v1/workflow-definitions/"+created.WorkflowDefinition.ID, nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected delete status 204, got %d", deleteResp.StatusCode)
	}
	if _, err := store.GetWorkflowDefinition(context.Background(), created.WorkflowDefinition.ID); !os.IsNotExist(err) {
		t.Fatalf("expected db row to be removed, got %v", err)
	}
	if _, err := os.Stat(mirrorPath); !os.IsNotExist(err) {
		t.Fatalf("expected mirror file to be removed, got %v", err)
	}
}

func TestWorkflowDefinitionValidationRejectsInvalidPayload(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)

	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	manager := NewManagerWithStore(cfg, suture.NewSimple("workflow-definitions"), nil, store)
	server := NewServer(cfg, manager, time.Now(), func() {}, nil)
	ts := httptest.NewServer(server.routes())
	t.Cleanup(ts.Close)

	resp := doWorkflowDefinitionRequest(t, ts.URL, http.MethodPost, "/v1/workflow-definitions", map[string]any{
		"version": "1",
		"name":    "invalid",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
	runs, err := store.LoadWorkflowDefinitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no persisted workflow definitions, got %d", len(runs))
	}
	mirrorPath := filepath.Join(home, ".agentflow", "workflows", "internal")
	if entries, err := os.ReadDir(mirrorPath); err == nil && len(entries) > 0 {
		t.Fatalf("expected no mirror files, found %d", len(entries))
	}
}

func TestWorkflowDefinitionLookupPrefersInternalAndSupportsWorkflowPath(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(workflowDir, "legacy.yaml")
	if err := os.WriteFile(legacyPath, []byte(`
version: "1"
name: legacy
nodes:
  - id: ok
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "shadow.yaml"), []byte(`
version: "1"
name: shadow
nodes:
  - id: yaml
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("workflow-definitions")
	go func() {
		_ = runSupervisor.Serve(ctx)
	}()
	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)

	created, err := manager.CreateWorkflowDefinition(context.Background(), coreworkflow.WorkflowSpec{
		Version: "1",
		Name:    "shadow",
		Nodes: []coreworkflow.NodeSpec{
			{ID: "ok", Kind: coreworkflow.NodeKindNoop},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	internalRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "shadow", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForWorkflowRunStatus(t, manager, internalRun.ID, "success")
	assertFileContains(t, filepath.Join(internalRun.RunDir, "workflow.yaml"), `"name": "shadow"`)
	if data, err := os.ReadFile(filepath.Join(internalRun.RunDir, "workflow.yaml")); err == nil {
		if strings.Contains(string(data), "name: shadow") {
			t.Fatalf("expected internal definition to win over yaml shadow, got yaml content: %s", string(data))
		}
	}

	idRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: created.ID, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForWorkflowRunStatus(t, manager, idRun.ID, "success")
	assertFileContains(t, filepath.Join(idRun.RunDir, "workflow.yaml"), `"name": "shadow"`)

	pathRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: legacyPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForWorkflowRunStatus(t, manager, pathRun.ID, "success")
	assertFileContains(t, filepath.Join(pathRun.RunDir, "workflow.yaml"), "name: legacy")
}

func TestOpenSQLiteRunStoreCreatesWorkflowDefinitionsTable(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "agentflowd.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE workflow_runs (id TEXT PRIMARY KEY, started_at TEXT NOT NULL, status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenSQLiteRunStore(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	rows, err := store.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name='workflow_definitions'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected workflow_definitions table to exist")
	}
}

func createWorkflowDefinition(t *testing.T, baseURL string, spec coreworkflow.WorkflowSpec) WorkflowDefinitionResponse {
	t.Helper()
	resp := doWorkflowDefinitionRequest(t, baseURL, http.MethodPost, "/v1/workflow-definitions", spec)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}
	var out WorkflowDefinitionResponse
	mustDecodeBody(t, resp.Body, &out)
	return out
}

func doWorkflowDefinitionRequest(t *testing.T, baseURL string, method string, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})
	return resp
}

func mustDecodeBody(t *testing.T, body io.ReadCloser, out any) {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode response: %v: %s", err, string(data))
	}
}

func waitForWorkflowRunStatus(t *testing.T, manager *Manager, runID string, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := manager.WorkflowStatus(runID)
		if ok && string(got.Status) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := manager.WorkflowStatus(runID)
	t.Fatalf("workflow run %s did not reach %s: %#v", runID, want, got)
}
