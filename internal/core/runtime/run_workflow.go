package runtime

import (
	"context"
	"fmt"
	"time"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/core/runtime/handlers"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type RunWorkflowUseCase struct {
	Workflows  coreports.WorkflowRepository
	Runs       coreports.RunRepository
	Events     coreports.EventSink
	Agents     coreports.AgentProviderRegistry
	Shell      coreports.ShellRunner
	Extensions coreports.ExtensionRunner
	Worktrees  coreports.WorktreeProviderRegistry
	Now        func() time.Time
}

type RunOptions = handlers.Options

type RunResult = handlers.Result

func NewRunID(workflowName string, now time.Time) string {
	return handlers.NewRunID(workflowName, now)
}

type workflowPreparation struct {
	plan           coreworkflow.ExecutionPlan
	resolvedInputs map[string]any
	sourcePath     string
}

func (uc *RunWorkflowUseCase) Validate(ctx context.Context, ref string) (coreworkflow.ExecutionPlan, error) {
	spec, _, err := uc.Workflows.Load(ctx, ref)
	if err != nil {
		return coreworkflow.ExecutionPlan{}, err
	}
	if err := coreworkflow.Validate(spec, uc.Agents, uc.Worktrees); err != nil {
		return coreworkflow.ExecutionPlan{}, err
	}
	return coreworkflow.BuildPlan(*spec)
}

func (uc *RunWorkflowUseCase) DryRun(ctx context.Context, opts RunOptions) (coreworkflow.ExecutionPlan, map[string]any, error) {
	prepared, err := uc.prepareRunWorkflow(ctx, opts)
	if err != nil {
		return coreworkflow.ExecutionPlan{}, nil, err
	}
	return prepared.plan, prepared.resolvedInputs, nil
}

func (uc *RunWorkflowUseCase) Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	if opts.ResumeRunID != "" {
		if opts.DryRun {
			return RunResult{}, fmt.Errorf("dry-run is not supported when resuming a run")
		}
		return handlers.Execute(ctx, uc.services(), handlers.ExecutionRequest{
			ResumeRunID: opts.ResumeRunID,
			WorkingDir:  opts.WorkingDir,
			Pause:       opts.Pause,
			Tag:         opts.Tag,
		})
	}
	prepared, err := uc.prepareRunWorkflow(ctx, opts)
	if err != nil {
		return RunResult{}, err
	}
	if opts.DryRun {
		return RunResult{Status: corerun.RunPlanned, Plan: prepared.plan}, nil
	}
	return handlers.Execute(ctx, uc.services(), handlers.ExecutionRequest{
		RunID:              opts.RunID,
		WorkflowSourcePath: prepared.sourcePath,
		Plan:               prepared.plan,
		Inputs:             prepared.resolvedInputs,
		WorkingDir:         opts.WorkingDir,
		Pause:              opts.Pause,
		Tag:                opts.Tag,
	})
}

func (uc *RunWorkflowUseCase) prepareRunWorkflow(ctx context.Context, opts RunOptions) (workflowPreparation, error) {
	spec, sourcePath, err := uc.Workflows.Load(ctx, opts.WorkflowRef)
	if err != nil {
		return workflowPreparation{}, err
	}
	resolvedInputs, err := handlers.ResolveInputs(*spec, opts.Inputs)
	if err != nil {
		return workflowPreparation{}, err
	}
	if err := coreworkflow.ValidateInputValues(*spec, resolvedInputs); err != nil {
		return workflowPreparation{}, err
	}
	handlers.ApplyWorkflowOverrides(spec, opts)
	if err := coreworkflow.Validate(spec, uc.Agents, uc.Worktrees); err != nil {
		return workflowPreparation{}, err
	}
	plan, err := coreworkflow.BuildPlan(*spec)
	if err != nil {
		return workflowPreparation{}, err
	}
	return workflowPreparation{plan: plan, resolvedInputs: resolvedInputs, sourcePath: sourcePath}, nil
}

func (uc *RunWorkflowUseCase) services() handlers.Services {
	return handlers.Services{
		Workflows:  uc.Workflows,
		Runs:       uc.Runs,
		Events:     uc.Events,
		Agents:     uc.Agents,
		Shell:      uc.Shell,
		Extensions: uc.Extensions,
		Worktrees:  uc.Worktrees,
		Now:        uc.Now,
	}
}
