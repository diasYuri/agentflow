package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
)

const (
	daemonTickInterval = 5 * time.Second
	runsTickInterval   = 5 * time.Second
	runDetailInterval  = 3 * time.Second
	runEventsInterval  = 2 * time.Second
	runLogsInterval    = 3 * time.Second
	statusTimeout      = 2 * time.Second
	operationTimeout   = 5 * time.Second
	maxCachedEvents    = 5000
	maxCachedLogLines  = 5000
)

func refreshDaemonStatusCmd(c client.DaemonClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
		defer cancel()
		state, err := c.DaemonStatus(ctx)
		return DaemonStatusMsg{State: state, Err: err}
	}
}

func fetchRunsCmd(c client.RunClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		runs, err := c.ListRuns(ctx)
		return RunsListMsg{Runs: runs, Err: err}
	}
}

func fetchRunDetailCmd(c client.RunClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		run, err := c.GetRun(ctx, runID)
		return RunDetailMsg{RunID: runID, Run: run, Err: err}
	}
}

func fetchRunLogsCmd(c client.RunClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		lines, err := c.GetRunLogs(ctx, runID)
		return RunLogsMsg{RunID: runID, Lines: lines, Err: err}
	}
}

func fetchRunEventsCmd(c client.RunClient, runID string, cursor int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		batch, err := c.GetRunEvents(ctx, runID, cursor, 100)
		return RunEventsMsg{RunID: runID, Batch: batch, Err: err}
	}
}

func fetchArtifactCmd(c client.ArtifactClient, runID, artifactID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		artifact, err := c.GetArtifact(ctx, runID, artifactID)
		return ArtifactMsg{RunID: runID, Artifact: artifact, Err: err}
	}
}

func fetchRunArtifactsCmd(c client.ArtifactClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		artifacts, err := c.ListArtifacts(ctx, runID)
		return RunArtifactsMsg{RunID: runID, Artifacts: artifacts, Err: err}
	}
}

func fetchRunNodesCmd(c client.RunClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		nodes, err := c.GetRunNodes(ctx, runID)
		return RunNodesMsg{RunID: runID, Nodes: nodes, Err: err}
	}
}

func fetchRunPlanCmd(c client.RunClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		plan, err := c.GetRunPlan(ctx, runID)
		return RunPlanMsg{RunID: runID, Plan: plan, Err: err}
	}
}

func cancelRunCmd(c client.ControlClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		err := c.CancelRun(ctx, runID)
		return ControlMsg{RunID: runID, Action: "cancel", Err: err}
	}
}

func pauseRunCmd(c client.ControlClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		err := c.PauseRun(ctx, runID)
		return ControlMsg{RunID: runID, Action: "pause", Err: err}
	}
}

func resumeRunCmd(c client.ControlClient, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		err := c.ResumeRun(ctx, runID)
		return ControlMsg{RunID: runID, Action: "resume", Err: err}
	}
}

func listWorkflowsCmd(c client.WorkflowClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		workflows, err := c.ListLocalWorkflows(ctx)
		return WorkflowsListMsg{Workflows: workflows, Err: err}
	}
}

func validateWorkflowCmd(c client.WorkflowClient, ref string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		err := c.ValidateWorkflow(ctx, ref)
		return WorkflowValidateMsg{Ref: ref, Err: err}
	}
}

func graphWorkflowCmd(c client.WorkflowClient, ref string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		out, err := c.GraphWorkflow(ctx, ref)
		return WorkflowGraphMsg{Ref: ref, Output: out, Err: err}
	}
}

func dryRunWorkflowCmd(c client.WorkflowClient, ref string, inputs, vars map[string]any) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		out, err := c.DryRunWorkflow(ctx, ref, inputs, vars)
		return WorkflowDryRunMsg{Ref: ref, Output: out, Err: err}
	}
}
