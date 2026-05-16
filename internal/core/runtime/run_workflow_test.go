package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentfake "github.com/diasYuri/agentflow/internal/adapters/agent/fake"
	eventjsonl "github.com/diasYuri/agentflow/internal/adapters/events/jsonl"
	eventmemory "github.com/diasYuri/agentflow/internal/adapters/events/memory"
	eventmulti "github.com/diasYuri/agentflow/internal/adapters/events/multi"
	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	worktreefake "github.com/diasYuri/agentflow/internal/adapters/worktree/fake"
	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/run"
	"gopkg.in/yaml.v3"
)

func TestRunWorkflowFanOutPreservesOutputOrder(t *testing.T) {
	dir := t.TempDir()
	workflowRef := writeWorkflow(t, dir, `
version: "1"
name: fanout
inputs:
  files:
    type: array
    required: true
execution:
  max_concurrency: 3
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: split
    kind: transform
    operation: chunk
    input: "${inputs.files}"
    with:
      chunks: 2
  - id: echo
    kind: agent
    depends_on: [split]
    for_each: "${nodes.split.output}"
    concurrency: 2
    prompt: "${item}"
  - id: merge
    kind: transform
    depends_on: [echo]
    operation: merge
    input: "${nodes.echo.outputs}"
`)

	fakeProvider := agentfake.New()
	events := eventmemory.New()
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    events,
		Agents: ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
			"codex": fakeProvider,
		}),
		Shell: shell.NewRunner(),
		Now:   func() time.Time { return time.Unix(1, 0).UTC() },
	}
	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowRef,
		Inputs:      map[string]any{"files": []any{"a", "b", "c", "d"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	echo := result.Summary.Nodes["echo"]
	if len(echo.Outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %#v", echo.Outputs)
	}
	if echo.Outputs[0] != `["a","b"]` || echo.Outputs[1] != `["c","d"]` {
		t.Fatalf("outputs not preserved: %#v", echo.Outputs)
	}
	if result.Summary.AgentCalls != 2 {
		t.Fatalf("expected 2 agent calls, got %d", result.Summary.AgentCalls)
	}
	expanded := findEvent(events.Events, "node.expanded", "echo")
	if expanded == nil {
		t.Fatal("expected node.expanded event for echo")
	}
	if expanded.Data["items"] != 2 {
		t.Fatalf("expected expanded items=2, got %#v", expanded.Data["items"])
	}
	if expanded.Data["concurrency"] != 2 {
		t.Fatalf("expected expanded concurrency=2, got %#v", expanded.Data["concurrency"])
	}
	if expanded.Data["fail_fast"] != true {
		t.Fatalf("expected expanded fail_fast=true, got %#v", expanded.Data["fail_fast"])
	}
}

func TestRunWorkflowValidateDoesNotRequireInputs(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: validate-inputs
inputs:
  required_value:
    type: string
    required: true
nodes:
  - id: ok
    kind: noop
`)
	uc := newTestRunWorkflowUseCase(dir, &scriptedShell{}, eventmemory.New())

	plan, err := uc.Validate(context.Background(), workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Workflow.Name != "validate-inputs" {
		t.Fatalf("unexpected workflow name: %s", plan.Workflow.Name)
	}
}

func TestRunWorkflowRejectsInvalidProvidedInputType(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: typed-inputs
inputs:
  enabled:
    type: boolean
    required: true
nodes:
  - id: ok
    kind: noop
`)
	uc := newTestRunWorkflowUseCase(dir, &scriptedShell{}, eventmemory.New())

	_, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		Inputs:      map[string]any{"enabled": "true"},
	})
	if err == nil {
		t.Fatal("expected invalid input type error")
	}
	if !strings.Contains(err.Error(), `input "enabled"`) {
		t.Fatalf("expected input context, got %v", err)
	}
}

func TestRunWorkflowMapContainerExecutesNestedWorkflow(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: nested-map
inputs:
  items:
    type: array
    required: true
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: outer
    kind: noop
  - id: batch
    kind: map
    depends_on: [outer]
    for_each: "${inputs.items}"
    concurrency: 2
    nodes:
      - id: draft
        kind: agent
        prompt: "${item}-${nodes.outer.output.status}"
      - id: render
        kind: bash
        depends_on: [draft]
        command: "render ${nodes.draft.output}"
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: req.Command + "\n", ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		Inputs:      map[string]any{"items": []any{"alpha", "beta"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	batch := result.Summary.Nodes["batch"]
	if len(batch.Outputs) != 2 {
		t.Fatalf("expected 2 batch outputs, got %#v", batch.Outputs)
	}
	first, ok := batch.Outputs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected batch output item to be map, got %T", batch.Outputs[0])
	}
	if got := first["stdout"]; got != "render alpha-ok\n" {
		t.Fatalf("unexpected first batch stdout: %#v", got)
	}
	if got := persistedNodeStatus(t, result.RunDir, "batch", "0000"); got != run.NodeSuccess {
		t.Fatalf("expected batch instance success, got %s", got)
	}
	assertFileContains(t, filepath.Join(result.RunDir, "nodes", "batch", "0000", "draft", "result.json"), "alpha-ok")
	assertFileContains(t, filepath.Join(result.RunDir, "nodes", "batch", "0000", "render", "stdout.txt"), "render alpha-ok")
}

func TestRunWorkflowWhenSkipsNode(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: when-skip
nodes:
  - id: test
    kind: bash
    command: "exit 0"
    continue_on_error: true
  - id: fix
    kind: noop
    depends_on: [test]
    when: "${nodes.test.exit_code} != 0"
`)
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agentfake.New()}),
		Shell:     shell.NewRunner(),
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}
	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Summary.Nodes["fix"].Status; got != run.NodeSkipped {
		t.Fatalf("expected fix skipped, got %s", got)
	}
}

func TestRunWorkflowGoToIfLoopsUntilConditionTurnsFalse(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: loop
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: loop
    kind: bash
    command: "toggle"
    go_to_if:
      when: "${nodes.loop.output.stdout} == \"again\""
      target: loop
  - id: done
    kind: noop
    depends_on: [loop]
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			if call == 1 {
				return ports.ShellResult{Stdout: "again", ExitCode: 0}, nil
			}
			return ports.ShellResult{Stdout: "stop", ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if got := len(shell.commands()); got != 2 {
		t.Fatalf("expected loop to run twice, got %d commands: %#v", got, shell.commands())
	}
	if got := result.Summary.BashCalls; got != 2 {
		t.Fatalf("expected two bash calls, got %d", got)
	}
	if got := result.Summary.Nodes["done"].Status; got != run.NodeSuccess {
		t.Fatalf("expected done success, got %s", got)
	}
}

func TestRunWorkflowContinueOnErrorCompletesRunAfterFailure(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: continue-on-error
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: flaky
    kind: bash
    command: "fail"
    continue_on_error: true
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: "before failure\n", Stderr: "boom\n", ExitCode: 42}, errors.New("boom")
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected run success, got %s", result.Status)
	}
	flaky := result.Summary.Nodes["flaky"]
	if flaky.Status != run.NodeFailed {
		t.Fatalf("expected failed node to be recorded, got %s", flaky.Status)
	}
	if flaky.Attempts != 1 {
		t.Fatalf("expected one attempt, got %d", flaky.Attempts)
	}
	if result.Summary.BashCalls != 1 {
		t.Fatalf("expected one bash call, got %d", result.Summary.BashCalls)
	}
	if result.Summary.FailedNodes != 1 {
		t.Fatalf("expected one failed node, got %d", result.Summary.FailedNodes)
	}
	if len(shell.commands()) != 1 {
		t.Fatalf("expected shell to run once, got %d", len(shell.commands()))
	}
}

func TestRunWorkflowFailFastCancelsQueuedForEachInstances(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: fail-fast
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
    concurrency: 1
    command: "${item}"
    continue_on_error: true
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stderr: "failed " + req.Command, ExitCode: 1}, errors.New("failed " + req.Command)
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		Inputs:      map[string]any{"items": []any{"first", "second", "third"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	each := result.Summary.Nodes["each"]
	if each.Status != run.NodeCancelled {
		t.Fatalf("expected expanded node cancellation, got %s", each.Status)
	}
	if len(shell.commands()) != 1 {
		t.Fatalf("expected fail_fast to stop after first shell call, got commands %#v", shell.commands())
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0000"); got != run.NodeFailed {
		t.Fatalf("expected first instance failed, got %s", got)
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0001"); got != run.NodeCancelled {
		t.Fatalf("expected second instance cancelled, got %s", got)
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0002"); got != run.NodeCancelled {
		t.Fatalf("expected third instance cancelled, got %s", got)
	}
}

func TestRunWorkflowFailFastFalseRunsAllForEachInstances(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: fail-fast-false
inputs:
  items:
    type: array
    required: true
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  fail_fast: false
nodes:
  - id: each
    kind: bash
    for_each: "${inputs.items}"
    concurrency: 1
    command: "${item}"
    continue_on_error: true
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			if req.Command == "bad" {
				return ports.ShellResult{Stderr: "bad item", ExitCode: 2}, errors.New("bad item")
			}
			return ports.ShellResult{Stdout: req.Command + "\n", ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		Inputs:      map[string]any{"items": []any{"ok-1", "bad", "ok-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(shell.commands()); got != 3 {
		t.Fatalf("expected fail_fast false to run all instances, got %d commands: %#v", got, shell.commands())
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0000"); got != run.NodeSuccess {
		t.Fatalf("expected first instance success, got %s", got)
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0001"); got != run.NodeFailed {
		t.Fatalf("expected second instance failed, got %s", got)
	}
	if got := persistedNodeStatus(t, result.RunDir, "each", "0002"); got != run.NodeSuccess {
		t.Fatalf("expected third instance success, got %s", got)
	}
}

func TestRunWorkflowTimeoutMarksNodeTimeout(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: timeout
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: slow
    kind: bash
    timeout: 1
    command: "sleep"
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			<-ctx.Done()
			return ports.ShellResult{ExitCode: -1}, ctx.Err()
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if result.Status != run.RunFailed {
		t.Fatalf("expected run failed, got %s", result.Status)
	}
	if got := result.Summary.Nodes["slow"].Status; got != run.NodeTimeout {
		t.Fatalf("expected node timeout, got %s", got)
	}
}

func TestRunWorkflowRetriesFailedNode(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: retry
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: flaky
    kind: bash
    retries: 1
    command: "flaky"
`)
	events := eventmemory.New()
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			if call == 1 {
				return ports.ShellResult{Stderr: "try again", ExitCode: 1}, errors.New("try again")
			}
			return ports.ShellResult{Stdout: "ok\n", ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, events)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	flaky := result.Summary.Nodes["flaky"]
	if flaky.Status != run.NodeSuccess {
		t.Fatalf("expected retry success, got %s", flaky.Status)
	}
	if flaky.Attempts != 2 {
		t.Fatalf("expected two attempts, got %d", flaky.Attempts)
	}
	if len(shell.commands()) != 2 {
		t.Fatalf("expected two shell calls, got %d", len(shell.commands()))
	}
	if result.Summary.BashCalls != 2 {
		t.Fatalf("expected two bash calls, got %d", result.Summary.BashCalls)
	}
	if result.Summary.Retries != 1 {
		t.Fatalf("expected one retry, got %d", result.Summary.Retries)
	}
	if result.Summary.FailedNodes != 0 {
		t.Fatalf("expected no failed nodes, got %d", result.Summary.FailedNodes)
	}
	retrying := findEvent(events.Events, "node.retrying", "flaky")
	if retrying == nil {
		t.Fatalf("expected node.retrying event, got %#v", events.Events)
	}
	if retrying.Attempt != 2 {
		t.Fatalf("expected retry attempt 2, got %d", retrying.Attempt)
	}
	if retrying.Data["attempt"] != 2 {
		t.Fatalf("expected retry data attempt=2, got %#v", retrying.Data["attempt"])
	}
	if retrying.Data["max_attempts"] != 2 {
		t.Fatalf("expected retry data max_attempts=2, got %#v", retrying.Data["max_attempts"])
	}
	if retrying.Data["retry"] != 1 {
		t.Fatalf("expected retry data retry=1, got %#v", retrying.Data["retry"])
	}
	if retrying.Data["max_retries"] != 1 {
		t.Fatalf("expected retry data max_retries=1, got %#v", retrying.Data["max_retries"])
	}
	if retrying.Data["delay_ms"] != int64(250) {
		t.Fatalf("expected retry data delay_ms=250, got %#v", retrying.Data["delay_ms"])
	}
	if retrying.Data["previous_status"] != run.NodeFailed {
		t.Fatalf("expected retry previous_status=failed, got %#v", retrying.Data["previous_status"])
	}
}

func TestRunWorkflowPauseWhenFailPausesAfterRetries(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: pause-failure
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: flaky
    kind: bash
    retries: 1
    command: "flaky"
`)
	events := eventmemory.New()
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stderr: "boom", ExitCode: 1}, errors.New("boom")
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, events)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunPaused {
		t.Fatalf("expected run paused, got %s", result.Status)
	}
	if len(shell.commands()) != 2 {
		t.Fatalf("expected initial attempt plus retry, got %#v", shell.commands())
	}
	if result.Summary.Nodes["flaky"].Status != run.NodeFailed {
		t.Fatalf("expected failed node in paused summary, got %s", result.Summary.Nodes["flaky"].Status)
	}
	checkpoint := readCheckpoint(t, filepath.Join(result.RunDir, "checkpoint.json"))
	if checkpoint.Status != run.RunPaused {
		t.Fatalf("expected paused checkpoint, got %s", checkpoint.Status)
	}
	if checkpoint.Reason != run.PauseReasonPauseWhenFail {
		t.Fatalf("expected pause_when_fail reason, got %s", checkpoint.Reason)
	}
	if checkpoint.RetryNodeID != "flaky" {
		t.Fatalf("expected retry node flaky, got %q", checkpoint.RetryNodeID)
	}
	assertFileContains(t, filepath.Join(result.RunDir, "normalized.json"), `"pause_when_fail": true`)
	if findEvent(events.Events, "run.pausing", "flaky") == nil {
		t.Fatalf("expected run.pausing event, got %#v", events.Events)
	}
	if findEvent(events.Events, "run.paused", "") == nil {
		t.Fatalf("expected run.paused event, got %#v", events.Events)
	}
}

func TestRunWorkflowResumeReexecutesOnlyPausedNode(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: pause-resume
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: prepare
    kind: bash
    command: "prepare"
  - id: flaky
    kind: bash
    depends_on: [prepare]
    retries: 1
    command: "flaky ${nodes.prepare.stdout}"
  - id: after
    kind: bash
    depends_on: [flaky]
    command: "after ${nodes.prepare.stdout} ${nodes.flaky.stdout}"
`)
	events := eventmemory.New()
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			switch req.Command {
			case "prepare":
				return ports.ShellResult{Stdout: "ready", ExitCode: 0}, nil
			case "flaky ready":
				if call <= 3 {
					return ports.ShellResult{Stderr: "not yet", ExitCode: 1}, errors.New("not yet")
				}
				return ports.ShellResult{Stdout: "fixed", ExitCode: 0}, nil
			case "after ready fixed":
				return ports.ShellResult{Stdout: "done", ExitCode: 0}, nil
			default:
				return ports.ShellResult{Stderr: req.Command, ExitCode: 2}, errors.New(req.Command)
			}
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, events)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != run.RunPaused {
		t.Fatalf("expected paused first run, got %s", first.Status)
	}
	resumed, err := uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != run.RunSuccess {
		t.Fatalf("expected resumed success, got %s", resumed.Status)
	}
	commands := shell.commands()
	if got := countString(commands, "prepare"); got != 1 {
		t.Fatalf("expected prepare to run once, got %d commands %#v", got, commands)
	}
	if got := countString(commands, "flaky ready"); got != 3 {
		t.Fatalf("expected flaky twice before pause and once after resume, got %d commands %#v", got, commands)
	}
	if got := countString(commands, "after ready fixed"); got != 1 {
		t.Fatalf("expected after to run once, got %d commands %#v", got, commands)
	}
	if resumed.Summary.Nodes["prepare"].Stdout != "ready" {
		t.Fatalf("expected prepare result available after resume, got %#v", resumed.Summary.Nodes["prepare"])
	}
	if resumed.Summary.Nodes["after"].Status != run.NodeSuccess {
		t.Fatalf("expected after success, got %s", resumed.Summary.Nodes["after"].Status)
	}
	assertFileNotExists(t, filepath.Join(resumed.RunDir, "checkpoint.json"))
	if findEvent(events.Events, "run.resumed", "") == nil {
		t.Fatalf("expected run.resumed event, got %#v", events.Events)
	}
}

func TestRunWorkflowWithoutPauseWhenFailStillFails(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: no-pause-failure
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: fail
    kind: bash
    retries: 1
    command: "fail"
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stderr: "boom", ExitCode: 1}, errors.New("boom")
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err == nil {
		t.Fatal("expected failed run error")
	}
	if result.Status != run.RunFailed {
		t.Fatalf("expected failed run, got %s", result.Status)
	}
	assertFileNotExists(t, filepath.Join(result.RunDir, "checkpoint.json"))
}

func TestRunWorkflowPausedCheckpointMasksSecrets(t *testing.T) {
	dir := t.TempDir()
	secret := "pause-secret-token"
	t.Setenv("agentflow_PAUSE_SECRET", secret)
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: pause-secret
secrets:
  api_token:
    env: agentflow_PAUSE_SECRET
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: leak
    kind: bash
    command: "${secrets.api_token}"
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: req.Command, Stderr: req.Command, ExitCode: 1}, errors.New(req.Command)
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmemory.New())

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunPaused {
		t.Fatalf("expected paused run, got %s", result.Status)
	}
	assertFileRedacted(t, filepath.Join(result.RunDir, "checkpoint.json"), secret)
	assertFileRedacted(t, filepath.Join(result.RunDir, "summary.json"), secret)
}

func TestRunWorkflowPersistsRunFiles(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: persistence
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: produce
    kind: bash
    command: "produce"
`)
	events := eventmemory.New()
	jsonlSink, err := eventjsonl.New("")
	if err != nil {
		t.Fatal(err)
	}
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: "persisted stdout\n", Stderr: "persisted stderr\n", ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, eventmulti.New(events, jsonlSink))

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, filepath.Join(result.RunDir, "run.json"))
	assertFileExists(t, filepath.Join(result.RunDir, "workflow.yaml"))
	assertFileExists(t, filepath.Join(result.RunDir, "normalized.json"))
	assertFileExists(t, filepath.Join(result.RunDir, "plan.json"))
	assertFileExists(t, filepath.Join(result.RunDir, "summary.json"))
	assertFileExists(t, filepath.Join(result.RunDir, "events.jsonl"))
	assertFileContains(t, filepath.Join(result.RunDir, "nodes", "produce", "stdout.txt"), "persisted stdout")
	assertFileContains(t, filepath.Join(result.RunDir, "nodes", "produce", "stderr.txt"), "persisted stderr")
	if got := persistedNodeStatus(t, result.RunDir, "produce", ""); got != run.NodeSuccess {
		t.Fatalf("expected persisted node success, got %s", got)
	}
	assertFileContains(t, filepath.Join(result.RunDir, "events.jsonl"), `"type":"run.completed"`)
}

func TestRunWorkflowMasksSecretsInEventsAndPersistedResults(t *testing.T) {
	dir := t.TempDir()
	secret := "super-secret-token"
	t.Setenv("agentflow_TEST_SECRET", secret)
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: mask-secrets
secrets:
  api_token:
    env: agentflow_TEST_SECRET
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: leak
    kind: bash
    command: "${secrets.api_token}"
`)
	events := eventmemory.New()
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: req.Command, Stderr: req.Command, ExitCode: 0}, nil
		},
	}
	uc := newTestRunWorkflowUseCase(dir, shell, events)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if got := result.Summary.Nodes["leak"].Stdout; got != run.MaskReplacement {
		t.Fatalf("expected masked summary stdout, got %q", got)
	}
	if got := result.Summary.Nodes["leak"].Stderr; got != run.MaskReplacement {
		t.Fatalf("expected masked summary stderr, got %q", got)
	}

	assertFileRedacted(t, filepath.Join(result.RunDir, "nodes", "leak", "stdout.txt"), secret)
	assertFileRedacted(t, filepath.Join(result.RunDir, "nodes", "leak", "stderr.txt"), secret)
	assertFileRedacted(t, filepath.Join(result.RunDir, "nodes", "leak", "result.json"), secret)
	assertFileRedacted(t, filepath.Join(result.RunDir, "summary.json"), secret)

	warningIndex := -1
	completedIndex := -1
	for i, event := range events.Events {
		if event.Type == "node.bash.warning" {
			warningIndex = i
			command, _ := event.Data["command"].(string)
			if strings.Contains(command, secret) {
				t.Fatalf("bash warning command contains secret: %q", command)
			}
			if !strings.Contains(command, run.MaskReplacement) {
				t.Fatalf("bash warning command is not masked: %q", command)
			}
		}
		if event.Type == "node.completed" && event.NodeID == "leak" {
			completedIndex = i
		}
	}
	if warningIndex == -1 {
		t.Fatal("expected node.bash.warning event")
	}
	if completedIndex == -1 {
		t.Fatal("expected node.completed event")
	}
	if warningIndex > completedIndex {
		t.Fatalf("expected bash warning before completion, warning=%d completed=%d", warningIndex, completedIndex)
	}
}

func TestRunWorkflowRequiresRequiredSecrets(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: require-secret
secrets:
  api_token:
    env: AGENTFLOW_MISSING_TEST_SECRET
    required: true
nodes:
  - id: leak
    kind: bash
    command: "${secrets.api_token}"
`)
	uc := newTestRunWorkflowUseCase(dir, &scriptedShell{}, eventmemory.New())

	_, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err == nil {
		t.Fatal("expected required secret error")
	}
	if !strings.Contains(err.Error(), `secret "api_token" requires environment variable "AGENTFLOW_MISSING_TEST_SECRET"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWorkflowResolvesWorkingDirAgainstRunRoot(t *testing.T) {
	dir := t.TempDir()
	runRoot := filepath.Join(dir, "project")
	if err := os.Mkdir(runRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: working-dir
execution:
  output_dir: ".agentflow/runs"
nodes:
  - id: shell
    kind: bash
    command: "pwd"
    capture:
      stdout: true
      stderr: true
      exit_code: true
  - id: agent
    kind: agent
    depends_on: [shell]
    prompt: "where am i?"
`)
	shell := &scriptedShell{
		run: func(ctx context.Context, req ports.ShellRequest, call int) (ports.ShellResult, error) {
			return ports.ShellResult{Stdout: req.WorkingDir + "\n", ExitCode: 0}, nil
		},
	}
	agent := &recordingAgentProvider{}
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents: ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
			"codex": agent,
		}),
		Shell: shell,
		Now:   func() time.Time { return time.Unix(1, 0).UTC() },
	}
	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		WorkingDir:  runRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if len(shell.requests) != 1 {
		t.Fatalf("expected 1 shell request, got %d", len(shell.requests))
	}
	if got := shell.requests[0].WorkingDir; got != runRoot {
		t.Fatalf("shell working dir mismatch: got %q want %q", got, runRoot)
	}
	agentDirs := agent.workingDirs()
	if len(agentDirs) != 1 {
		t.Fatalf("expected 1 agent request, got %d", len(agentDirs))
	}
	if got := agentDirs[0]; got != runRoot {
		t.Fatalf("agent working dir mismatch: got %q want %q", got, runRoot)
	}
	expectedRunDir := filepath.Join(dir, "runs")
	if !strings.HasPrefix(result.RunDir, expectedRunDir) {
		t.Fatalf("run dir mismatch: got %q want prefix %q", result.RunDir, expectedRunDir)
	}
}

func TestRunWorkflowMapsPermissionWriteToAgentSandbox(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: agent-permissions
nodes:
  - id: write_enabled
    kind: agent
    permission:
      write: true
    prompt: "write mode"
  - id: write_disabled
    kind: agent
    permission:
      write: false
    prompt: "read mode"
  - id: explicit_sandbox
    kind: agent
    permission:
      write: true
    sandbox:
      mode: danger-full-access
    prompt: "explicit sandbox"
`)
	agent := &recordingAgentProvider{}
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents: ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
			"codex": agent,
		}),
		Shell: &scriptedShell{},
		Now:   func() time.Time { return time.Unix(1, 0).UTC() },
	}

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	modes := agent.sandboxModes()
	if len(modes) != 3 {
		t.Fatalf("expected 3 agent requests, got %d", len(modes))
	}
	if got := modes[0]; got != "workspace-write" {
		t.Fatalf("expected workspace-write for write_enabled, got %q", got)
	}
	if got := modes[1]; got != "read-only" {
		t.Fatalf("expected read-only for write_disabled, got %q", got)
	}
	if got := modes[2]; got != "danger-full-access" {
		t.Fatalf("expected explicit sandbox to win, got %q", got)
	}
}

type scriptedShell struct {
	mu       sync.Mutex
	calls    int
	requests []ports.ShellRequest
	run      func(context.Context, ports.ShellRequest, int) (ports.ShellResult, error)
}

func (s *scriptedShell) Run(ctx context.Context, req ports.ShellRequest) (ports.ShellResult, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	if s.run == nil {
		return ports.ShellResult{ExitCode: 0}, nil
	}
	return s.run(ctx, req, call)
}

func (s *scriptedShell) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	commands := make([]string, len(s.requests))
	for i, req := range s.requests {
		commands[i] = req.Command
	}
	return commands
}

type recordingAgentProvider struct {
	mu       sync.Mutex
	requests []ports.AgentRequest
}

func (p *recordingAgentProvider) Run(ctx context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	_ = ctx
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	return ports.AgentResult{Text: req.WorkingDir}, nil
}

func (p *recordingAgentProvider) workingDirs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	dirs := make([]string, len(p.requests))
	for i, req := range p.requests {
		dirs[i] = req.WorkingDir
	}
	return dirs
}

func (p *recordingAgentProvider) sandboxModes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	modes := make([]string, len(p.requests))
	for i, req := range p.requests {
		modes[i] = req.Sandbox.Mode
	}
	return modes
}

func newTestRunWorkflowUseCase(dir string, shellRunner ports.ShellRunner, events ports.EventSink) *RunWorkflowUseCase {
	return &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    events,
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agentfake.New()}),
		Shell:     shellRunner,
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}
}

func newTestRunWorkflowUseCaseWithWorktree(dir string, shellRunner ports.ShellRunner, events ports.EventSink, worktrees ports.WorktreeProviderRegistry) *RunWorkflowUseCase {
	return &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    events,
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agentfake.New()}),
		Shell:     shellRunner,
		Worktrees: worktrees,
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}
}

func writeWorkflow(t *testing.T, dir string, content string) string {
	t.Helper()
	var spec struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
		t.Fatal(err)
	}
	workflowDir := filepath.Join(dir, "agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(workflowDir, spec.Name+".yaml")
	if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return spec.Name
}

func newTestWorkflowRepository(dir string) *yamlrepo.WorkflowRepository {
	return yamlrepo.NewWorkflowRepository(
		filepath.Join(dir, "agentflow", "workflows"),
		filepath.Join(dir, "home", ".agentflow", "workflows"),
	)
}

func persistedNodeStatus(t *testing.T, runDir string, nodeID string, instanceID string) run.NodeStatus {
	t.Helper()
	path := filepath.Join(runDir, "nodes", nodeID)
	if instanceID != "" {
		path = filepath.Join(path, instanceID)
	}
	data, err := os.ReadFile(filepath.Join(path, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result run.NodeResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result.Status
}

func findEvent(events []run.Event, eventType string, nodeID string) *run.Event {
	for i := range events {
		if events[i].Type == eventType && events[i].NodeID == nodeID {
			return &events[i]
		}
	}
	return nil
}

func readCheckpoint(t *testing.T, path string) run.Checkpoint {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint run.Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func writeCheckpoint(t *testing.T, path string, checkpoint run.Checkpoint) {
	t.Helper()
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func assertFileNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s not to exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(data))
	}
}

func assertFileRedacted(t *testing.T, path string, secret string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, secret) {
		t.Fatalf("%s contains secret: %s", path, content)
	}
	if !strings.Contains(content, run.MaskReplacement) {
		t.Fatalf("%s does not contain mask replacement: %s", path, content)
	}
}

func normalizeMacTempPath(path string) string {
	path = filepath.Clean(path)
	if strings.HasPrefix(path, "/var/") {
		return "/private" + path
	}
	return path
}

func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@agentflow")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial")
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", name}, {"commit", "-m", "change " + name}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
		}
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestRunWorkflowWorktreeCreatesWorktreeBeforeFirstNode(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-bash
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "pwd"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		WorkingDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	stdout := result.Summary.Nodes["shell"].Stdout
	if !strings.Contains(stdout, worktreeBase) {
		t.Fatalf("expected shell stdout inside worktree base %q, got %q", worktreeBase, stdout)
	}
	if strings.HasPrefix(result.RunDir, worktreeBase) {
		t.Fatalf("expected run dir outside worktree, got %q", result.RunDir)
	}
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "status.json"))
}

func TestRunWorkflowWorktreeAgentReceivesWorktreeDir(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-agent
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: agent
    kind: agent
    prompt: "hello"
`)
	agent := &recordingAgentProvider{}
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agent}),
		Shell:     &scriptedShell{},
		Worktrees: worktrees,
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}

	result, err := uc.Run(context.Background(), RunOptions{
		WorkflowRef: workflowPath,
		WorkingDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	dirs := agent.workingDirs()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 agent request, got %d", len(dirs))
	}
	if !strings.HasPrefix(dirs[0], worktreeBase) {
		t.Fatalf("expected agent working dir inside worktree base %q, got %q", worktreeBase, dirs[0])
	}
}

func TestRunWorkflowWorktreeUnknownProviderFailsEarly(t *testing.T) {
	dir := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: unknown-provider
worktree:
  enabled: true
  provider: missing
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo hi"
`)
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, &scriptedShell{}, eventmemory.New(), ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{}))

	_, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err == nil {
		t.Fatal("expected error for unknown worktree provider")
	}
	if !strings.Contains(err.Error(), `unknown worktree provider "missing"`) {
		t.Fatalf("expected unknown provider error, got %v", err)
	}
}

func TestRunWorkflowWorktreePreservesMetadataOnFailure(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-fail
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "false"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath})
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Status != run.RunFailed {
		t.Fatalf("expected failed run, got %s", result.Status)
	}
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "status.json"))
	status := readWorktreeStatus(t, result.RunDir)
	if !status.Enabled || status.Provider != "fake" || status.Name == "" || status.WorktreePath == "" || status.BaseCommit == "" {
		t.Fatalf("expected complete worktree metadata, got %+v", status)
	}
	if status.MergeStatus != run.WorktreeMergeFailed {
		t.Fatalf("expected failed merge status on failed run, got %s", status.MergeStatus)
	}
}

func TestRunWorkflowWorktreeNoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-no-changes
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if status.MergeStatus != run.WorktreeMergeNoChanges {
		t.Fatalf("expected no_changes, got %s", status.MergeStatus)
	}
	if status.CleanupStatus != run.WorktreeCleanupRemoved {
		t.Fatalf("expected cleanup removed, got %s", status.CleanupStatus)
	}
	assertFileNotExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "diff.patch"))
}

func TestRunWorkflowWorktreeWithChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	wtProvider.StatusResult = &ports.WorktreeStatus{Clean: false, Files: []ports.FileStatus{{Path: "a.txt", Status: "M"}}}
	wtProvider.DiffResult = &ports.ChangeSet{
		Empty: false,
		Files: []ports.FileChange{{Path: "a.txt", Status: "M"}},
		Diff:  "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
	}
	wtProvider.ApplyResult = &ports.MergeResult{Success: true}
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-with-changes
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if status.MergeStatus != run.WorktreeMergeMerged {
		t.Fatalf("expected merged, got %s", status.MergeStatus)
	}
	if !status.Enabled || status.Provider != "fake" || status.Name != "wt-worktree-with-changes" {
		t.Fatalf("expected enabled/provider/name metadata, got %+v", status)
	}
	if status.WorktreePath == "" || status.BaseCommit == "" || status.DestinationCommitBefore == "" || status.DestinationCommitAfter == "" {
		t.Fatalf("expected path and commit metadata, got %+v", status)
	}
	if len(status.ChangedFiles) != 1 || status.ChangedFiles[0].Path != "a.txt" {
		t.Fatalf("expected changed file a.txt, got %+v", status.ChangedFiles)
	}
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "diff.patch"))
	assertFileContains(t, filepath.Join(result.RunDir, "artifacts", "worktree", "diff.patch"), "diff --git")
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "merge.log"))
}

func TestRunWorkflowWorktreeConflictKeepsWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	wtProvider.StatusResult = &ports.WorktreeStatus{Clean: false, Files: []ports.FileStatus{{Path: "a.txt", Status: "M"}}}
	wtProvider.DiffResult = &ports.ChangeSet{
		Empty: false,
		Files: []ports.FileChange{{Path: "a.txt", Status: "M"}},
		Diff:  "diff...",
	}
	wtProvider.ApplyResult = &ports.MergeResult{
		Success:   false,
		Conflicts: []ports.Conflict{{Path: "a.txt", Reason: "content conflict"}},
	}
	wtProvider.ApplyError = ports.ErrWorktreeResolvable
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-conflict
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected run success even with conflict (recorded in metadata), got %s", result.Status)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if status.MergeStatus != run.WorktreeMergeConflict {
		t.Fatalf("expected conflict, got %s", status.MergeStatus)
	}
	if len(status.Conflicts) != 1 || status.Conflicts[0].Path != "a.txt" {
		t.Fatalf("expected conflict a.txt, got %+v", status.Conflicts)
	}
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "conflicts.json"))
	assertFileContains(t, filepath.Join(result.RunDir, "artifacts", "worktree", "conflicts.json"), "a.txt")
	if status.CleanupStatus != run.WorktreeCleanupKept {
		t.Fatalf("expected cleanup kept on conflict, got %s", status.CleanupStatus)
	}
}

func TestRunWorkflowWorktreeStructuralErrorNoAgent(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	wtProvider.StatusResult = &ports.WorktreeStatus{Clean: false, Files: []ports.FileStatus{{Path: "a.txt", Status: "M"}}}
	wtProvider.DiffResult = &ports.ChangeSet{
		Empty: false,
		Files: []ports.FileChange{{Path: "a.txt", Status: "M"}},
		Diff:  "diff...",
	}
	wtProvider.ApplyResult = &ports.MergeResult{
		Success:   false,
		Conflicts: []ports.Conflict{{Path: "a.txt", Reason: "structural"}},
	}
	wtProvider.ApplyError = ports.ErrWorktreeStructural
	agent := &recordingAgentProvider{}
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-structural
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agent}),
		Shell:     shell.NewRunner(),
		Worktrees: worktrees,
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected run success (structural error in metadata), got %s", result.Status)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if status.MergeStatus != run.WorktreeMergeFailed {
		t.Fatalf("expected failed, got %s", status.MergeStatus)
	}
	if len(agent.requests) != 0 {
		t.Fatalf("expected no agent calls for structural error, got %d", len(agent.requests))
	}
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "worktree", "conflicts.json"))
}

func TestRunWorkflowWorktreeCleanupOnSuccessFalse(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-keep
worktree:
  enabled: true
  provider: fake
  base: current
  cleanup:
    on_success: false
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if status.CleanupStatus != run.WorktreeCleanupKept {
		t.Fatalf("expected cleanup kept when on_success=false, got %s", status.CleanupStatus)
	}
}

func TestRunWorkflowWorktreeRenameInMetadata(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	wtProvider.StatusResult = &ports.WorktreeStatus{Clean: false, Files: []ports.FileStatus{{Path: "b.txt", Status: "R"}}}
	wtProvider.DiffResult = &ports.ChangeSet{
		Empty: false,
		Files: []ports.FileChange{{Path: "b.txt", Status: "R", OldPath: "a.txt"}},
		Diff:  "diff...",
	}
	wtProvider.ApplyResult = &ports.MergeResult{Success: true}
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-rename
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	status := readWorktreeStatus(t, result.RunDir)
	if len(status.ChangedFiles) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(status.ChangedFiles))
	}
	if status.ChangedFiles[0].Path != "b.txt" || status.ChangedFiles[0].OldPath != "a.txt" || status.ChangedFiles[0].Status != "R" {
		t.Fatalf("expected renamed file metadata, got %+v", status.ChangedFiles[0])
	}
}

func TestRunWorkflowWorktreeConflictRequestsAgent(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	wtProvider := worktreefake.New(worktreeBase)
	wtProvider.StatusResult = &ports.WorktreeStatus{Clean: false, Files: []ports.FileStatus{{Path: "a.txt", Status: "M"}}}
	wtProvider.DiffResult = &ports.ChangeSet{
		Empty: false,
		Files: []ports.FileChange{{Path: "a.txt", Status: "M"}},
		Diff:  "diff...",
	}
	wtProvider.ApplyResult = &ports.MergeResult{
		Success:   false,
		Conflicts: []ports.Conflict{{Path: "a.txt", Reason: "content conflict"}},
	}
	wtProvider.ApplyError = ports.ErrWorktreeResolvable
	agent := &recordingAgentProvider{}
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-agent-conflict
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
nodes:
  - id: shell
    kind: bash
    command: "echo ok"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": wtProvider,
	})
	uc := &RunWorkflowUseCase{
		Workflows: newTestWorkflowRepository(dir),
		Runs:      runrepo.New(filepath.Join(dir, "runs")),
		Events:    eventmemory.New(),
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{"codex": agent}),
		Shell:     shell.NewRunner(),
		Worktrees: worktrees,
		Now:       func() time.Time { return time.Unix(1, 0).UTC() },
	}

	result, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != run.RunSuccess {
		t.Fatalf("expected run success, got %s", result.Status)
	}
	if len(agent.requests) != 1 {
		t.Fatalf("expected 1 agent call for conflict resolution, got %d", len(agent.requests))
	}
	if agent.requests[0].NodeID != "worktree-resolution" {
		t.Fatalf("expected agent node_id worktree-resolution, got %s", agent.requests[0].NodeID)
	}
	if agent.requests[0].Sandbox.Mode != "workspace-write" {
		t.Fatalf("expected workspace-write sandbox, got %s", agent.requests[0].Sandbox.Mode)
	}
}

func readWorktreeStatus(t *testing.T, runDir string) run.WorktreeMetadata {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, "artifacts", "worktree", "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var status run.WorktreeMetadata
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestRunWorkflowWorktreeResumeReusesPausedWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	events := eventmemory.New()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-resume
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: prepare
    kind: bash
    command: "printf prepared >> prepare.log"
  - id: flaky
    kind: bash
    depends_on: [prepare]
    command: "if [ -f flaky.marker ]; then pwd; else touch flaky.marker; false; fi"
  - id: after
    kind: bash
    depends_on: [flaky]
    command: "test -f prepare.log && printf done"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), events, worktrees)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != run.RunPaused {
		t.Fatalf("expected paused, got %s", first.Status)
	}
	checkpoint := readCheckpoint(t, filepath.Join(first.RunDir, "checkpoint.json"))
	if checkpoint.Worktree == nil || checkpoint.Worktree.Path == "" {
		t.Fatalf("expected worktree checkpoint state, got %+v", checkpoint.Worktree)
	}
	pausedWorktreePath := checkpoint.Worktree.Path
	prepareLog := filepath.Join(pausedWorktreePath, "prepare.log")
	assertFileContains(t, prepareLog, "prepared")

	resumed, err := uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != run.RunSuccess {
		t.Fatalf("expected resumed success, got %s", resumed.Status)
	}
	if resumed.Summary.Nodes["prepare"].Status != run.NodeSuccess {
		t.Fatalf("expected prepare result restored, got %s", resumed.Summary.Nodes["prepare"].Status)
	}
	if resumed.Summary.Nodes["flaky"].Status != run.NodeSuccess {
		t.Fatalf("expected flaky success, got %s", resumed.Summary.Nodes["flaky"].Status)
	}
	resumedFlakyDir := normalizeMacTempPath(strings.TrimSpace(resumed.Summary.Nodes["flaky"].Stdout))
	pausedWorktreeRealPath := normalizeMacTempPath(pausedWorktreePath)
	if resumedFlakyDir != pausedWorktreeRealPath {
		t.Fatalf("expected resumed flaky node in %q, got %q", pausedWorktreePath, resumed.Summary.Nodes["flaky"].Stdout)
	}
	if resumed.Summary.Nodes["after"].Stdout != "done" {
		t.Fatalf("expected after node to run, got %#v", resumed.Summary.Nodes["after"])
	}
	assertFileNotExists(t, filepath.Join(resumed.RunDir, "checkpoint.json"))
	assertFileNotExists(t, pausedWorktreePath)
	resumedEvent := findEvent(events.Events, "run.resumed", "")
	if resumedEvent == nil {
		t.Fatalf("expected run.resumed event, got %#v", events.Events)
	}
	if resumedEvent.Data["worktree_path"] != pausedWorktreePath || resumedEvent.Data["worktree_provider"] != "fake" {
		t.Fatalf("expected worktree resume event data, got %#v", resumedEvent.Data)
	}
}

func TestRunWorkflowWorktreePausedCleanupAlwaysKeepsWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-paused-cleanup
worktree:
  enabled: true
  provider: fake
  base: current
  cleanup:
    on_failure: cleanup
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: flaky
    kind: bash
    command: "if [ -f flaky.marker ]; then true; else touch flaky.marker; false; fi"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != run.RunPaused {
		t.Fatalf("expected paused, got %s", first.Status)
	}
	status := readWorktreeStatus(t, first.RunDir)
	if status.CleanupStatus != run.WorktreeCleanupKept {
		t.Fatalf("expected paused cleanup kept, got %s", status.CleanupStatus)
	}
	assertFileExists(t, status.WorktreePath)

	resumed, err := uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != run.RunSuccess {
		t.Fatalf("expected resumed success, got %s", resumed.Status)
	}
}

func TestRunWorkflowWorktreeResumeRequiresCheckpointState(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-missing-checkpoint-state
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: shell
    kind: bash
    command: "false"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	checkpointPath := filepath.Join(first.RunDir, "checkpoint.json")
	checkpoint := readCheckpoint(t, checkpointPath)
	checkpoint.Worktree = nil
	writeCheckpoint(t, checkpointPath, checkpoint)

	_, err = uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err == nil {
		t.Fatal("expected missing worktree checkpoint state error")
	}
	if !strings.Contains(err.Error(), "missing worktree state") {
		t.Fatalf("expected missing worktree checkpoint state error, got %v", err)
	}
}

func TestRunWorkflowWorktreeResumeFailsWhenWorktreePathRemoved(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-path-removed
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: shell
    kind: bash
    command: "false"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := readCheckpoint(t, filepath.Join(first.RunDir, "checkpoint.json"))
	if checkpoint.Worktree == nil {
		t.Fatal("expected worktree checkpoint state")
	}
	if err := os.RemoveAll(checkpoint.Worktree.Path); err != nil {
		t.Fatal(err)
	}

	_, err = uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err == nil {
		t.Fatal("expected removed worktree path error")
	}
	if !strings.Contains(err.Error(), "worktree path") {
		t.Fatalf("expected worktree path error, got %v", err)
	}
}

func TestRunWorkflowWorktreeResumeFailsWhenDestinationHeadChanged(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	worktreeBase := t.TempDir()
	workflowPath := writeWorkflow(t, dir, `
version: "1"
name: worktree-destination-changed
worktree:
  enabled: true
  provider: fake
  base: current
execution:
  output_dir: "`+filepath.ToSlash(filepath.Join(dir, "runs"))+`"
  pause_when_fail: true
nodes:
  - id: shell
    kind: bash
    command: "false"
`)
	worktrees := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"fake": worktreefake.New(worktreeBase),
	})
	uc := newTestRunWorkflowUseCaseWithWorktree(dir, shell.NewRunner(), eventmemory.New(), worktrees)

	first, err := uc.Run(context.Background(), RunOptions{WorkflowRef: workflowPath, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, dir, "changed.txt", "changed")

	_, err = uc.Run(context.Background(), RunOptions{ResumeRunID: first.RunID})
	if err == nil {
		t.Fatal("expected destination HEAD changed error")
	}
	if !strings.Contains(err.Error(), "destination HEAD changed") {
		t.Fatalf("expected destination HEAD changed error, got %v", err)
	}
}
