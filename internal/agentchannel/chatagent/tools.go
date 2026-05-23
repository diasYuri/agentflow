package chatagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
)

type WorkflowDefinitionClient interface {
	ListWorkflowDefinitions(ctx context.Context) (daemon.WorkflowDefinitionsResponse, error)
	WorkflowDefinition(ctx context.Context, id string) (daemon.WorkflowDefinitionResponse, error)
}

type WorkflowRunClient interface {
	RunWorkflow(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error)
	ListWorkflows(ctx context.Context) (daemon.ListWorkflowsResponse, error)
	WorkflowStatus(ctx context.Context, runID string) (daemon.RunWorkflowResponse, error)
	WorkflowInspect(ctx context.Context, runID string) (daemon.WorkflowInspectResponse, error)
	WorkflowTimeline(ctx context.Context, runID string, cursor int, limit int) (daemon.WorkflowTimelineResponse, error)
	WorkflowNodes(ctx context.Context, runID string) (daemon.WorkflowNodesResponse, error)
	WorkflowSummary(ctx context.Context, runID string) (daemon.WorkflowSummaryResponse, error)
	WorkflowArtifacts(ctx context.Context, runID string) (daemon.WorkflowArtifactsResponse, error)
}

type ToolCallStatus string

const (
	ToolCallRunning   ToolCallStatus = "running"
	ToolCallSucceeded ToolCallStatus = "succeeded"
	ToolCallFailed    ToolCallStatus = "failed"
)

type ToolCallRecorder interface {
	Start(ctx context.Context, name string, request any) (string, error)
	Finish(ctx context.Context, id string, status ToolCallStatus, response any, errMsg string) error
}

type ToolEnvironment struct {
	SessionID        string
	ProjectPath      string
	ProjectName      string
	Definitions      WorkflowDefinitionClient
	Runs             WorkflowRunClient
	ProjectReader    *ProjectReader
	Providers        ports.AgentProviderRegistry
	EnabledProviders []string
	Recorder         ToolCallRecorder
}

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Invoke      toolInvoker
}

type toolInvoker = func(ctx context.Context, raw json.RawMessage) (any, error)

func BuildTools(env *ToolEnvironment) []Tool {
	if env == nil {
		env = &ToolEnvironment{}
	}
	return []Tool{
		newListWorkflowsTool(env),
		newDescribeWorkflowTool(env),
		newRunWorkflowTool(env),
		newInspectWorkflowTool(env),
		newReadProjectTool(env),
		newAskEnvironmentTool(env),
	}
}

func WithRecorder(env *ToolEnvironment, tool Tool) Tool {
	if env == nil || env.Recorder == nil {
		return tool
	}
	original := tool.Invoke
	tool.Invoke = wrapToolInvoke(env.Recorder, tool.Name, original)
	return tool
}

func wrapToolInvoke(rec ToolCallRecorder, name string, fn toolInvoker) toolInvoker {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		id, startErr := rec.Start(ctx, name, json.RawMessage(raw))
		if startErr != nil {
			return nil, fmt.Errorf("record tool start: %w", startErr)
		}
		result, err := fn(ctx, raw)
		if err != nil {
			_ = rec.Finish(ctx, id, ToolCallFailed, nil, err.Error())
			return nil, err
		}
		_ = rec.Finish(ctx, id, ToolCallSucceeded, result, "")
		return result, nil
	}
}

type listWorkflowsInput struct {
	IncludeRuns *bool `json:"include_runs,omitempty"`
}

type listWorkflowsOutput struct {
	Definitions []daemon.WorkflowDefinitionSummary `json:"definitions"`
	Runs        []daemon.WorkflowRun               `json:"runs,omitempty"`
}

func newListWorkflowsTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in listWorkflowsInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		if env.Definitions == nil && env.Runs == nil {
			return nil, errors.New("workflow clients are not configured")
		}
		out := listWorkflowsOutput{}
		if env.Definitions != nil {
			resp, err := env.Definitions.ListWorkflowDefinitions(ctx)
			if err != nil {
				return nil, fmt.Errorf("list workflow definitions: %w", err)
			}
			out.Definitions = resp.Definitions
		}
		includeRuns := in.IncludeRuns == nil || *in.IncludeRuns
		if includeRuns && env.Runs != nil {
			resp, err := env.Runs.ListWorkflows(ctx)
			if err != nil {
				return nil, fmt.Errorf("list workflow runs: %w", err)
			}
			out.Runs = resp.Runs
		}
		return out, nil
	}
	return Tool{
		Name:        "agentflow.list_workflows",
		Description: "List workflow definitions and (optionally) recent workflow runs.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_runs": map[string]any{"type": "boolean"},
			},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

type describeWorkflowInput struct {
	Workflow string `json:"workflow"`
}

type describeWorkflowOutput struct {
	Definition daemon.WorkflowDefinition `json:"definition"`
}

func newDescribeWorkflowTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Definitions == nil {
			return nil, errors.New("workflow definition client is not configured")
		}
		var in describeWorkflowInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		ref := strings.TrimSpace(in.Workflow)
		if ref == "" {
			return nil, errors.New("workflow is required")
		}
		resp, err := env.Definitions.WorkflowDefinition(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("describe workflow %s: %w", ref, err)
		}
		return describeWorkflowOutput{Definition: resp.WorkflowDefinition}, nil
	}
	return Tool{
		Name:        "agentflow.describe_workflow",
		Description: "Get a workflow definition by id or name, including declared inputs, outputs, graph, execution order, and raw spec.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workflow": map[string]any{"type": "string"},
			},
			"required":             []string{"workflow"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

type runWorkflowInput struct {
	WorkflowRef string         `json:"workflow_ref"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	Vars        map[string]any `json:"vars,omitempty"`
	Tag         string         `json:"tag,omitempty"`
	DryRun      bool           `json:"dry_run,omitempty"`
}

type runWorkflowOutput struct {
	Run daemon.WorkflowRun `json:"run"`
}

func newRunWorkflowTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Runs == nil {
			return nil, errors.New("workflow run client is not configured")
		}
		var in runWorkflowInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		ref := strings.TrimSpace(in.WorkflowRef)
		if ref == "" {
			return nil, errors.New("workflow_ref is required")
		}
		req := daemon.RunWorkflowRequest{
			WorkflowRef: ref,
			Inputs:      in.Inputs,
			Vars:        in.Vars,
			Tag:         in.Tag,
			DryRun:      in.DryRun,
			WorkingDir:  env.ProjectPath,
		}
		resp, err := env.Runs.RunWorkflow(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("run workflow %s: %w", ref, err)
		}
		return runWorkflowOutput{Run: resp.Run}, nil
	}
	return Tool{
		Name:        "agentflow.run_workflow",
		Description: "Run an AgentFlow workflow by reference using the active project as the working directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workflow_ref": map[string]any{"type": "string"},
				"inputs":       map[string]any{"type": "object"},
				"vars":         map[string]any{"type": "object"},
				"tag":          map[string]any{"type": "string"},
				"dry_run":      map[string]any{"type": "boolean"},
			},
			"required":             []string{"workflow_ref"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

type inspectWorkflowInput struct {
	RunID            string `json:"run_id"`
	IncludeTimeline  bool   `json:"include_timeline,omitempty"`
	IncludeNodes     bool   `json:"include_nodes,omitempty"`
	IncludeSummary   bool   `json:"include_summary,omitempty"`
	IncludeArtifacts bool   `json:"include_artifacts,omitempty"`
	TimelineLimit    int    `json:"timeline_limit,omitempty"`
}

type inspectWorkflowOutput struct {
	Inspect   daemon.WorkflowInspectResponse    `json:"inspect"`
	Timeline  *daemon.WorkflowTimelineResponse  `json:"timeline,omitempty"`
	Nodes     *daemon.WorkflowNodesResponse     `json:"nodes,omitempty"`
	Summary   *daemon.WorkflowSummaryResponse   `json:"summary,omitempty"`
	Artifacts *daemon.WorkflowArtifactsResponse `json:"artifacts,omitempty"`
}

func newInspectWorkflowTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Runs == nil {
			return nil, errors.New("workflow run client is not configured")
		}
		var in inspectWorkflowInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		runID := strings.TrimSpace(in.RunID)
		if runID == "" {
			return nil, errors.New("run_id is required")
		}
		inspect, err := env.Runs.WorkflowInspect(ctx, runID)
		if err != nil {
			return nil, fmt.Errorf("inspect run %s: %w", runID, err)
		}
		out := inspectWorkflowOutput{Inspect: inspect}
		if in.IncludeTimeline {
			limit := in.TimelineLimit
			if limit <= 0 {
				limit = 50
			}
			tl, err := env.Runs.WorkflowTimeline(ctx, runID, 0, limit)
			if err != nil {
				return nil, fmt.Errorf("timeline %s: %w", runID, err)
			}
			out.Timeline = &tl
		}
		if in.IncludeNodes {
			nodes, err := env.Runs.WorkflowNodes(ctx, runID)
			if err != nil {
				return nil, fmt.Errorf("nodes %s: %w", runID, err)
			}
			out.Nodes = &nodes
		}
		if in.IncludeSummary {
			summary, err := env.Runs.WorkflowSummary(ctx, runID)
			if err != nil {
				return nil, fmt.Errorf("summary %s: %w", runID, err)
			}
			out.Summary = &summary
		}
		if in.IncludeArtifacts {
			arts, err := env.Runs.WorkflowArtifacts(ctx, runID)
			if err != nil {
				return nil, fmt.Errorf("artifacts %s: %w", runID, err)
			}
			out.Artifacts = &arts
		}
		return out, nil
	}
	return Tool{
		Name:        "agentflow.inspect_workflow",
		Description: "Inspect a workflow run; optionally include timeline, nodes, summary, and artifacts.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id":            map[string]any{"type": "string"},
				"include_timeline":  map[string]any{"type": "boolean"},
				"include_nodes":     map[string]any{"type": "boolean"},
				"include_summary":   map[string]any{"type": "boolean"},
				"include_artifacts": map[string]any{"type": "boolean"},
				"timeline_limit":    map[string]any{"type": "integer"},
			},
			"required":             []string{"run_id"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

type readProjectInput struct {
	Operation string `json:"operation"`
	Path      string `json:"path,omitempty"`
	Query     string `json:"query,omitempty"`
}

type readProjectOutput struct {
	Operation string         `json:"operation"`
	Entries   []ProjectEntry `json:"entries,omitempty"`
	File      *ProjectFile   `json:"file,omitempty"`
	Matches   []SearchMatch  `json:"matches,omitempty"`
}

func newReadProjectTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.ProjectReader == nil {
			return nil, errors.New("project reader is not configured")
		}
		var in readProjectInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		op := strings.ToLower(strings.TrimSpace(in.Operation))
		switch op {
		case "list", "":
			entries, err := env.ProjectReader.List(in.Path)
			if err != nil {
				return nil, err
			}
			return readProjectOutput{Operation: "list", Entries: entries}, nil
		case "read":
			file, err := env.ProjectReader.Read(in.Path)
			if err != nil {
				return nil, err
			}
			return readProjectOutput{Operation: "read", File: &file}, nil
		case "search":
			matches, err := env.ProjectReader.Search(in.Path, in.Query)
			if err != nil {
				return nil, err
			}
			return readProjectOutput{Operation: "search", Matches: matches}, nil
		default:
			return nil, fmt.Errorf("unsupported operation %q", in.Operation)
		}
	}
	return Tool{
		Name:        "agentflow.read_project",
		Description: "Read-only access to the active project: list directories, read files, or search text. Hidden runtime paths are denied.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{"type": "string"},
				"path":      map[string]any{"type": "string"},
				"query":     map[string]any{"type": "string"},
			},
			"required":             []string{"operation"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

type askEnvironmentInput struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Prompt   string `json:"prompt"`
	System   string `json:"system,omitempty"`
	Context  string `json:"context,omitempty"`
}

type askEnvironmentOutput struct {
	Provider string `json:"provider"`
	Text     string `json:"text"`
}

func newAskEnvironmentTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Providers == nil {
			return nil, errors.New("agent provider registry is not configured")
		}
		var in askEnvironmentInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		provider := strings.TrimSpace(in.Provider)
		if provider == "" {
			return nil, errors.New("provider is required")
		}
		if !providerEnabled(env.EnabledProviders, provider) {
			return nil, fmt.Errorf("provider %q is not enabled", provider)
		}
		impl, ok := env.Providers.Get(provider)
		if !ok {
			return nil, fmt.Errorf("provider %q is not registered", provider)
		}
		if strings.TrimSpace(in.Prompt) == "" {
			return nil, errors.New("prompt is required")
		}
		prompt := in.Prompt
		if in.Context != "" {
			prompt = in.Context + "\n\n" + prompt
		}
		req := ports.AgentRequest{
			Provider:   provider,
			Model:      in.Model,
			System:     in.System,
			Prompt:     prompt,
			WorkingDir: env.ProjectPath,
			Sandbox:    workflow.SandboxSpec{Mode: "read-only"},
		}
		result, err := impl.Run(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("ask %s: %w", provider, err)
		}
		return askEnvironmentOutput{Provider: provider, Text: result.Text}, nil
	}
	return Tool{
		Name:        "agentflow.ask_environment",
		Description: "Ask an auxiliary agent (e.g. codex, claude, pi) for a read-only answer using the active project as working directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{"type": "string"},
				"model":    map[string]any{"type": "string"},
				"prompt":   map[string]any{"type": "string"},
				"system":   map[string]any{"type": "string"},
				"context":  map[string]any{"type": "string"},
			},
			"required":             []string{"provider", "prompt"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

func providerEnabled(enabled []string, name string) bool {
	if len(enabled) == 0 {
		return true
	}
	for _, p := range enabled {
		if strings.EqualFold(strings.TrimSpace(p), name) {
			return true
		}
	}
	return false
}

func decodeToolInput(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode tool input: %w", err)
	}
	return nil
}
