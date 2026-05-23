package chatagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
)

type fakeDefinitions struct {
	resp daemon.WorkflowDefinitionsResponse
	def  daemon.WorkflowDefinitionResponse
	err  error
}

func (f *fakeDefinitions) ListWorkflowDefinitions(_ context.Context) (daemon.WorkflowDefinitionsResponse, error) {
	return f.resp, f.err
}

func (f *fakeDefinitions) WorkflowDefinition(_ context.Context, _ string) (daemon.WorkflowDefinitionResponse, error) {
	return f.def, f.err
}

type fakeRuns struct {
	runReq         daemon.RunWorkflowRequest
	runResp        daemon.RunWorkflowResponse
	runErr         error
	listResp       daemon.ListWorkflowsResponse
	inspectResp    daemon.WorkflowInspectResponse
	timelineResp   daemon.WorkflowTimelineResponse
	nodesResp      daemon.WorkflowNodesResponse
	summaryResp    daemon.WorkflowSummaryResponse
	artifactsResp  daemon.WorkflowArtifactsResponse
	recordedRunIDs []string
}

func (f *fakeRuns) RunWorkflow(_ context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
	f.runReq = req
	return f.runResp, f.runErr
}

func (f *fakeRuns) ListWorkflows(_ context.Context) (daemon.ListWorkflowsResponse, error) {
	return f.listResp, nil
}

func (f *fakeRuns) WorkflowStatus(_ context.Context, runID string) (daemon.RunWorkflowResponse, error) {
	f.recordedRunIDs = append(f.recordedRunIDs, runID)
	return f.runResp, nil
}

func (f *fakeRuns) WorkflowInspect(_ context.Context, runID string) (daemon.WorkflowInspectResponse, error) {
	f.recordedRunIDs = append(f.recordedRunIDs, runID)
	return f.inspectResp, nil
}

func (f *fakeRuns) WorkflowTimeline(_ context.Context, runID string, _ int, _ int) (daemon.WorkflowTimelineResponse, error) {
	f.recordedRunIDs = append(f.recordedRunIDs, runID)
	return f.timelineResp, nil
}

func (f *fakeRuns) WorkflowNodes(_ context.Context, runID string) (daemon.WorkflowNodesResponse, error) {
	return f.nodesResp, nil
}

func (f *fakeRuns) WorkflowSummary(_ context.Context, runID string) (daemon.WorkflowSummaryResponse, error) {
	return f.summaryResp, nil
}

func (f *fakeRuns) WorkflowArtifacts(_ context.Context, runID string) (daemon.WorkflowArtifactsResponse, error) {
	return f.artifactsResp, nil
}

type fakeProjects struct {
	projects []app.Project
	err      error
}

func (f *fakeProjects) List() ([]app.Project, error) {
	return f.projects, f.err
}

type fakeProvider struct {
	received ports.AgentRequest
	result   ports.AgentResult
	err      error
}

func (f *fakeProvider) Run(_ context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	f.received = req
	return f.result, f.err
}

type recordedCall struct {
	name     string
	status   ToolCallStatus
	err      string
	response any
}

type fakeRecorder struct {
	calls []recordedCall
}

func (f *fakeRecorder) Start(_ context.Context, name string, _ any) (string, error) {
	id := "tc-" + name
	f.calls = append(f.calls, recordedCall{name: name, status: ToolCallRunning})
	return id, nil
}

func (f *fakeRecorder) Finish(_ context.Context, _ string, status ToolCallStatus, response any, errMsg string) error {
	if len(f.calls) == 0 {
		return errors.New("finish without start")
	}
	last := &f.calls[len(f.calls)-1]
	last.status = status
	last.err = errMsg
	last.response = response
	return nil
}

func findTool(t *testing.T, tools []Tool, name string) Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return Tool{}
}

func TestBuildToolsListsFiveTools(t *testing.T) {
	tools := BuildTools(&ToolEnvironment{})
	if len(tools) != 8 {
		t.Fatalf("expected 8 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	want := []string{"agentflow.list_projects", "agentflow.list_workflows", "agentflow.list_runs", "agentflow.describe_workflow", "agentflow.run_workflow", "agentflow.inspect_workflow", "agentflow.read_project", "agentflow.ask_environment"}
	for _, n := range want {
		if !names[n] {
			t.Fatalf("missing tool %q", n)
		}
	}
}

func TestListProjectsReturnsConfiguredProjects(t *testing.T) {
	env := &ToolEnvironment{Projects: &fakeProjects{projects: []app.Project{{Name: "agentflow", Path: "/repo"}}}}
	tool := findTool(t, BuildTools(env), "agentflow.list_projects")
	result, err := tool.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	out := result.(listProjectsOutput)
	if len(out.Projects) != 1 || out.Projects[0].Name != "agentflow" {
		t.Fatalf("projects: %+v", out.Projects)
	}
}

func TestListWorkflowsDefaultsToDefinitionsOnly(t *testing.T) {
	defs := &fakeDefinitions{resp: daemon.WorkflowDefinitionsResponse{Definitions: []daemon.WorkflowDefinitionSummary{{ID: "wf1", Name: "Build"}}}}
	runs := &fakeRuns{listResp: daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{{ID: "r1"}}}}
	env := &ToolEnvironment{Definitions: defs, Runs: runs}
	tool := findTool(t, BuildTools(env), "agentflow.list_workflows")
	result, err := tool.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	out := result.(listWorkflowsOutput)
	if len(out.Definitions) != 1 || out.Definitions[0].ID != "wf1" {
		t.Fatalf("definitions: %+v", out.Definitions)
	}
}

func TestListWorkflowsRejectsRunParameters(t *testing.T) {
	defs := &fakeDefinitions{resp: daemon.WorkflowDefinitionsResponse{Definitions: []daemon.WorkflowDefinitionSummary{{ID: "wf1"}}}}
	runs := &fakeRuns{listResp: daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{{ID: "r1"}}}}
	env := &ToolEnvironment{Definitions: defs, Runs: runs}
	tool := findTool(t, BuildTools(env), "agentflow.list_workflows")
	result, err := tool.Invoke(context.Background(), json.RawMessage(`{"include_runs":true}`))
	if err == nil {
		t.Fatalf("expected include_runs to be rejected, got result %+v", result)
	}
}

func TestListRunsReturnsRuns(t *testing.T) {
	runs := &fakeRuns{listResp: daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{{ID: "r1"}}}}
	env := &ToolEnvironment{Runs: runs}
	tool := findTool(t, BuildTools(env), "agentflow.list_runs")
	result, err := tool.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	out := result.(listRunsOutput)
	if len(out.Runs) != 1 || out.Runs[0].ID != "r1" {
		t.Fatalf("runs: %+v", out.Runs)
	}
}

func TestToolDescriptionsSeparateProjectsDefinitionsAndRuns(t *testing.T) {
	tools := BuildTools(&ToolEnvironment{})
	listProjects := findTool(t, tools, "agentflow.list_projects")
	if !strings.Contains(listProjects.Description, "projects") || strings.Contains(listProjects.Description, "workflow runs") {
		t.Fatalf("project description is ambiguous: %q", listProjects.Description)
	}
	listWorkflows := findTool(t, tools, "agentflow.list_workflows")
	for _, want := range []string{"workflow definitions", "which workflows can I run", "never returns workflow runs"} {
		if !strings.Contains(listWorkflows.Description, want) {
			t.Fatalf("workflow description missing %q: %q", want, listWorkflows.Description)
		}
	}
	if properties := listWorkflows.Parameters["properties"].(map[string]any); len(properties) != 0 {
		t.Fatalf("list_workflows should not accept run parameters: %+v", properties)
	}
	listRuns := findTool(t, tools, "agentflow.list_runs")
	for _, want := range []string{"workflow runs", "execution history", "not for available workflow definitions"} {
		if !strings.Contains(listRuns.Description, want) {
			t.Fatalf("run description missing %q: %q", want, listRuns.Description)
		}
	}
}

func TestDescribeWorkflowReturnsInputsOutputsAndGraph(t *testing.T) {
	defs := &fakeDefinitions{def: daemon.WorkflowDefinitionResponse{WorkflowDefinition: daemon.WorkflowDefinition{
		ID:    "wf1",
		Name:  "build",
		Graph: "graph TD\n  start\n",
		Inputs: map[string]workflow.InputSpec{
			"query": {Type: "string", Required: true},
		},
		Outputs: map[string]workflow.OutputSpec{
			"summary": {Type: "string"},
		},
	}}}
	tool := findTool(t, BuildTools(&ToolEnvironment{Definitions: defs}), "agentflow.describe_workflow")
	result, err := tool.Invoke(context.Background(), json.RawMessage(`{"workflow":"build"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	out := result.(describeWorkflowOutput)
	if out.Definition.Inputs["query"].Type != "string" || out.Definition.Outputs["summary"].Type != "string" {
		t.Fatalf("missing definition metadata: %+v", out.Definition)
	}
	if !strings.Contains(out.Definition.Graph, "graph TD") {
		t.Fatalf("missing graph: %q", out.Definition.Graph)
	}
}

func TestRunWorkflowPassesWorkingDirFromProjectPath(t *testing.T) {
	runs := &fakeRuns{runResp: daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{ID: "r99"}}}
	env := &ToolEnvironment{Runs: runs, ProjectPath: "/abs/project"}
	tool := findTool(t, BuildTools(env), "agentflow.run_workflow")
	input := json.RawMessage(`{"workflow_ref":"build","inputs":{"name":"x"}}`)
	result, err := tool.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if runs.runReq.WorkflowRef != "build" {
		t.Fatalf("workflow_ref: %q", runs.runReq.WorkflowRef)
	}
	if runs.runReq.WorkingDir != "/abs/project" {
		t.Fatalf("working_dir: %q", runs.runReq.WorkingDir)
	}
	if runs.runReq.Inputs["name"] != "x" {
		t.Fatalf("inputs: %+v", runs.runReq.Inputs)
	}
	if result.(runWorkflowOutput).Run.ID != "r99" {
		t.Fatalf("unexpected run id")
	}
}

func TestRunWorkflowRequiresRef(t *testing.T) {
	tool := findTool(t, BuildTools(&ToolEnvironment{Runs: &fakeRuns{}}), "agentflow.run_workflow")
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "workflow_ref is required") {
		t.Fatalf("expected workflow_ref error, got %v", err)
	}
}

func TestInspectWorkflowAggregatesIncludes(t *testing.T) {
	runs := &fakeRuns{
		inspectResp:   daemon.WorkflowInspectResponse{RunID: "r1", Workflow: "build"},
		timelineResp:  daemon.WorkflowTimelineResponse{RunID: "r1"},
		nodesResp:     daemon.WorkflowNodesResponse{RunID: "r1"},
		summaryResp:   daemon.WorkflowSummaryResponse{RunID: "r1"},
		artifactsResp: daemon.WorkflowArtifactsResponse{RunID: "r1"},
	}
	tool := findTool(t, BuildTools(&ToolEnvironment{Runs: runs}), "agentflow.inspect_workflow")
	input := json.RawMessage(`{"run_id":"r1","include_timeline":true,"include_nodes":true,"include_summary":true,"include_artifacts":true}`)
	result, err := tool.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	out := result.(inspectWorkflowOutput)
	if out.Inspect.RunID != "r1" || out.Timeline == nil || out.Nodes == nil || out.Summary == nil || out.Artifacts == nil {
		t.Fatalf("missing aggregated parts: %+v", out)
	}
}

func TestReadProjectListReadSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// hello world\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agentflow", "runs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentflow", "runs", "secret"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	env := &ToolEnvironment{ProjectReader: NewProjectReader(dir)}
	tool := findTool(t, BuildTools(env), "agentflow.read_project")

	listResult, err := tool.Invoke(context.Background(), json.RawMessage(`{"operation":"list"}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	list := listResult.(readProjectOutput)
	for _, e := range list.Entries {
		if strings.Contains(e.Path, ".agentflow/runs") {
			t.Fatalf("denied path returned: %s", e.Path)
		}
	}

	readResult, err := tool.Invoke(context.Background(), json.RawMessage(`{"operation":"read","path":"main.go"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(readResult.(readProjectOutput).File.Content, "hello world") {
		t.Fatalf("file content missing hello world")
	}

	searchResult, err := tool.Invoke(context.Background(), json.RawMessage(`{"operation":"search","path":"","query":"hello"}`))
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(searchResult.(readProjectOutput).Matches) == 0 {
		t.Fatalf("expected search matches")
	}
}

func TestReadProjectRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	env := &ToolEnvironment{ProjectReader: NewProjectReader(dir)}
	tool := findTool(t, BuildTools(env), "agentflow.read_project")
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"operation":"read","path":"../../etc/passwd"}`))
	if err == nil {
		t.Fatal("expected error for traversal")
	}
}

func TestReadProjectDeniesAgentflowRuns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agentflow", "runs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentflow", "runs", "data"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	env := &ToolEnvironment{ProjectReader: NewProjectReader(dir)}
	tool := findTool(t, BuildTools(env), "agentflow.read_project")
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"operation":"read","path":".agentflow/runs/data"}`))
	if !errors.Is(err, ErrPathDenied) {
		t.Fatalf("expected ErrPathDenied, got %v", err)
	}
}

func TestAskEnvironmentSendsReadOnlySandboxAndWorkingDir(t *testing.T) {
	provider := &fakeProvider{result: ports.AgentResult{Text: "answer"}}
	registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": provider})
	env := &ToolEnvironment{
		Providers:        registry,
		EnabledProviders: []string{"codex"},
		ProjectPath:      "/abs/project",
	}
	tool := findTool(t, BuildTools(env), "agentflow.ask_environment")
	result, err := tool.Invoke(context.Background(), json.RawMessage(`{"provider":"codex","prompt":"what is this"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result.(askEnvironmentOutput).Text != "answer" {
		t.Fatalf("unexpected text")
	}
	if provider.received.WorkingDir != "/abs/project" {
		t.Fatalf("working_dir: %q", provider.received.WorkingDir)
	}
	if provider.received.Sandbox != (workflow.SandboxSpec{Mode: "read-only"}) {
		t.Fatalf("sandbox not read-only: %+v", provider.received.Sandbox)
	}
}

func TestAskEnvironmentRejectsDisabledProvider(t *testing.T) {
	provider := &fakeProvider{}
	registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": provider, "claude": provider})
	env := &ToolEnvironment{
		Providers:        registry,
		EnabledProviders: []string{"codex"},
	}
	tool := findTool(t, BuildTools(env), "agentflow.ask_environment")
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"provider":"claude","prompt":"hi"}`))
	if err == nil || !strings.Contains(err.Error(), "is not enabled") {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestAskEnvironmentRejectsUnknownProvider(t *testing.T) {
	registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{})
	env := &ToolEnvironment{Providers: registry}
	tool := findTool(t, BuildTools(env), "agentflow.ask_environment")
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"provider":"codex","prompt":"hi"}`))
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected not-registered error, got %v", err)
	}
}

func TestWithRecorderRecordsSuccessAndFailure(t *testing.T) {
	rec := &fakeRecorder{}
	env := &ToolEnvironment{
		Runs:     &fakeRuns{runErr: errors.New("boom")},
		Recorder: rec,
	}
	tool := WithRecorder(env, findTool(t, BuildTools(env), "agentflow.run_workflow"))
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{"workflow_ref":"x"}`))
	if err == nil {
		t.Fatal("expected boom")
	}
	if len(rec.calls) != 1 || rec.calls[0].status != ToolCallFailed {
		t.Fatalf("expected one failed call, got %+v", rec.calls)
	}

	rec2 := &fakeRecorder{}
	env2 := &ToolEnvironment{
		Runs:     &fakeRuns{runResp: daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{ID: "ok"}}},
		Recorder: rec2,
	}
	tool2 := WithRecorder(env2, findTool(t, BuildTools(env2), "agentflow.run_workflow"))
	_, err = tool2.Invoke(context.Background(), json.RawMessage(`{"workflow_ref":"y"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec2.calls) != 1 || rec2.calls[0].status != ToolCallSucceeded {
		t.Fatalf("expected one succeeded call, got %+v", rec2.calls)
	}
}
