package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
)

// DaemonStatusMsg carries the result of a daemon status check.
type DaemonStatusMsg struct {
	State client.DaemonState
	Err   error
}

// RunsListMsg carries the result of listing runs.
type RunsListMsg struct {
	Runs []client.RunSummary
	Err  error
}

// RunDetailMsg carries the result of fetching a single run.
type RunDetailMsg struct {
	RunID string
	Run   client.RunSummary
	Err   error
}

// RunLogsMsg carries the result of fetching run logs.
type RunLogsMsg struct {
	RunID string
	Lines []string
	Err   error
}

// RunEventsMsg carries a batch of run events.
type RunEventsMsg struct {
	RunID string
	Batch client.EventBatch
	Err   error
}

// RunArtifactsMsg carries the result of fetching run artifacts.
type RunArtifactsMsg struct {
	RunID     string
	Artifacts []client.ArtifactSummary
	Err       error
}

// ArtifactMsg carries the result of fetching a single artifact.
type ArtifactMsg struct {
	RunID    string
	Artifact client.ArtifactSummary
	Err      error
}

// RunNodesMsg carries the result of fetching run nodes.
type RunNodesMsg struct {
	RunID string
	Nodes []client.NodeSummary
	Err   error
}

// RunPlanMsg carries the result of fetching a run plan.
type RunPlanMsg struct {
	RunID string
	Plan  client.PlanSummary
	Err   error
}

// ControlMsg carries the result of a control operation.
type ControlMsg struct {
	RunID  string
	Action string
	Err    error
}

// WorkflowsListMsg carries the result of listing local workflows.
type WorkflowsListMsg struct {
	Workflows []client.LocalWorkflow
	Err       error
}

// WorkflowValidateMsg carries the result of validating a workflow.
type WorkflowValidateMsg struct {
	Ref string
	Err error
}

// WorkflowGraphMsg carries the result of generating a workflow graph.
type WorkflowGraphMsg struct {
	Ref    string
	Output string
	Err    error
}

// WorkflowDryRunMsg carries the result of a workflow dry-run.
type WorkflowDryRunMsg struct {
	Ref    string
	Output string
	Err    error
}

// tickMsg triggers a polling tick.
type tickMsg struct {
	kind  tickKind
	runID string
}

type tickKind int

const (
	tickDaemon tickKind = iota
	tickRuns
	tickRunDetail
	tickRunEvents
	tickRunLogs
)

// tickCmd schedules the next polling tick.
func tickCmd(kind tickKind, runID string, interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{kind: kind, runID: runID}
	})
}
