# AgentFlow

```text
     ___                    __  ______
    /   | ____ ____  ____  / /_/ ____/___ _      __
   / /| |/ __ `/ _ \/ __ \/ __/ /_  / __ \ | /| / /
  / ___ / /_/ /  __/ / / / /_/ __/ / /_/ / |/ |/ /
 /_/  |_\__, /\___/_/ /_/\__/_/    \____/|__/|__/ 
       /____/

      YAML workflows for local coding agents
```

AgentFlow turns development processes into versioned, auditable, reproducible YAML workflows. You describe the sequence. The engine executes it with explicit dependencies, controlled parallelism, human approval, persisted artifacts, and local agent support through the CLI.

It is designed to be one of the most complete platforms for local agent workflows: predictable like classic automation, flexible like a modern orchestrator, and concrete enough for real-world repository work.

## Why Use It

- Repeatable: the same workflow follows the same sequence every time.
- Auditable: `graph` and `dry-run` show the plan before execution.
- Composable: mix `agent`, `bash`, `transform`, `map`, `approval`, and `noop` in the same graph.
- Parallel where it matters: expand lists with `for_each`, `concurrency`, and `max_items`.
- Persistent: runs, logs, events, and artifacts are stored locally.
- Local by default: it runs in your environment, on your repository, with your binaries.

## What It Does

- Defines workflows in YAML with `inputs`, `vars`, `secrets`, `defaults`, `execution`, and `nodes`.
- Runs local agents with `provider: codex`, `provider: claude`, or `provider: pi`.
- Orchestrates local commands with `kind: bash` and data transformations with `kind: transform`.
- Fans out work with `kind: map` and joins branches with `kind: noop`.
- Supports human approval, pause-on-failure, and run resume.
- Controls workflows and runs through the CLI, local daemon, and TUI.

## Requirements

- Go 1.26.1+
- `codex`, `claude`, or `pi` on `PATH` when a workflow uses `kind: agent`

## CLI

### Main Commands

- `agentflow validate <workflow>`: validate a workflow.
- `agentflow graph <workflow>`: print the Mermaid graph.
- `agentflow dry-run <workflow>`: resolve inputs and show the plan.
- `agentflow run <workflow>`: execute through `agentflowd` by default.
- `agentflow run <workflow> -it`: run in the foreground without the daemon.
- `agentflow migrate <workflow>`: migrate workflow v1 to v2.
- `agentflow project add <name> <path>`: register a project.
- `agentflow project list`: list registered projects.
- `agentflow project remove <name>`: remove a project.
- `agentflow daemon start|stop|status`: control the local daemon.
- `agentflow tui`: open the interactive terminal UI.

### `workflow` Namespace

- `agentflow workflow run <workflow>`: start a run in the daemon.
- `agentflow workflow list`: list known runs.
- `agentflow workflow status <id>`: show a run status.
- `agentflow workflow watch <id>`: follow a run until it finishes.
- `agentflow workflow logs <id>`: print run events.
- `agentflow workflow artifacts <run_id>`: list artifacts.
- `agentflow workflow artifact show <run_id> <artifact_id>`: show an artifact.
- `agentflow workflow artifact path <run_id> <artifact_id>`: print the local artifact path.
- `agentflow workflow cancel <id>`: cancel a run.
- `agentflow workflow pause <id>`: request a graceful pause.
- `agentflow workflow resume <id>`: resume a paused run.
- `agentflow workflow approve <id>`: approve a run waiting for human decision.
- `agentflow workflow reject <id>`: reject a run waiting for human decision.
- `agentflow workflow summary <run_id>`: show a run summary.
- `agentflow workflow timeline <run_id>`: show the run timeline.
- `agentflow workflow inspect <run_id>`: show run diagnostics.
- `agentflow workflow schedule add <workflow>`: create a schedule.
- `agentflow workflow schedule list`: list schedules.
- `agentflow workflow schedule remove <id>`: remove a schedule.

## Architecture

AgentFlow is organized to separate intent, planning, and execution. That reduces coupling, makes maintenance easier, and keeps system behavior more predictable.

- `cmd/agentflow/` and `cmd/agentflowd/` contain the application entrypoints.
- `internal/cli/` turns commands and flags into explicit actions.
- `internal/app/` coordinates use cases without depending on infrastructure details.
- `internal/core/workflow/` models and validates the workflow DSL.
- `internal/core/runtime/` executes the plan, tracks state, and handles failures.
- `internal/daemon/` persists runs, exposes RPC, and keeps execution running in the background.
- `internal/adapters/` connects the core to YAML, agents, Git, worktrees, SQLite, and other integrations.
- `internal/tui/` provides the interactive interface without mixing UI concerns into domain logic.

That separation is what puts AgentFlow ahead of simpler tools: a workflow is not just a prompt script. It becomes an execution artifact with validation, persistence, observability, and state recovery.

In practice, that means:

- you can inspect the graph before running anything;
- you can isolate execution in the background through the daemon;
- you can audit results and artifacts after the run;
- you can repeat the same process across teams without re-explaining the mechanics;
- you can combine deterministic steps and agent-driven steps in one flow.

## Useful Flags

- `--input key=value`: add or override an input.
- `--input-json <file>`: load inputs from JSON.
- `--var key=value`: override workflow variables.
- `--max-concurrency <n>`: override `execution.max_concurrency`.
- `--working-dir <path>`: set the base execution directory.
- `--project <name>`: resolve the workflow inside a registered project.
- `--codex-path <path>`, `--claude-path <path>`, `--pi-path <path>`: point to the provider binary.
- `--log-format text|json`: control log format.
- `--events-jsonl <path>`: write events as JSONL.
- `--dry-run`: validate and plan without executing.
- `--tag <name>`: set a friendly run or schedule name.
- `--output text|json`: control the output format of query commands.
- `--no-color`: disable colors.
- `--watch`: follow the run until completion.

## Agent Workflows

Workflows with `kind: agent` can use `provider: codex`, `provider: claude`, or `provider: pi`. When `provider` is omitted, the default is `codex`.

Example with Codex:

```bash
go run ./cmd/agentflow run samples/workflows/fix-github-issue.yaml \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)" \
  -it
```

Example with Claude Code:

```bash
go run ./cmd/agentflow run samples/workflows/claude-code-review.yaml \
  --claude-path "$(which claude)" \
  -it
```

Example with Pi RPC:

```bash
go run ./cmd/agentflow run samples/workflows/pi-code-review.yaml \
  --pi-path "$(which pi)" \
  -it
```

To resolve workflows by name inside a registered project:

```bash
mkdir -p .agentflow/workflows
cp samples/workflows/fix-github-issue.yaml .agentflow/workflows/fix-github-issue.yaml

go run ./cmd/agentflow project add demo .
go run ./cmd/agentflow run fix-github-issue \
  --project demo \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)"
```

## DSL

A workflow declares `version`, `name`, `inputs`, `vars`, `secrets`, `defaults`, `execution`, and a list of `nodes`.

Supported node types:

- `agent`: delegate work to an agent.
- `approval`: pause for human decision.
- `bash`: run local commands and capture output.
- `transform`: transform data between stages.
- `map`: expand a list into parallel executions.
- `noop`: create milestones, joins, and conditional stages without external side effects.

Common DSL features:

- `depends_on` for explicit dependencies.
- `when` for conditional execution.
- `go_to_if` for controlled loops.
- `for_each`, `concurrency`, and `max_items` for fan-out.
- `output_schema` for structured agent responses.
- `secrets` for reading sensitive values from the environment.
- `artifacts` for persisting relevant workflow outputs.
- `execution.pause_when_fail` to pause failed runs and resume later.

## Daemon And Local State

By default, `agentflow run <workflow>` uses the local `agentflowd` daemon. The daemon stores runs and metadata at:

- Socket: `~/.agentflow/agentflowd.sock`
- PID: `~/.agentflow/agentflowd.pid`
- Log: `~/.agentflow/agentflowd.log`
- Database: `~/.agentflow/agentflowd.sqlite`
- Runs: `~/.agentflow/runs`
- Projects: `~/.agentflow/projects.json`
- Schedules: `~/.agentflow/schedules.json`

Example:

```bash
agentflow daemon start
agentflow workflow run review-changed-files --input-json samples/inputs/review-files.json
agentflow workflow list
agentflow workflow logs <run_id>
```

## Included Examples

The examples in `samples/workflows/` and `samples/inputs/` cover real cases such as:

- changed-file reviews
- failure repair
- release notes
- security review
- workflow migration
- schedules
- validation loops
- human approval

See also [samples/README.md](samples/README.md) for ready-to-run commands and descriptions of the samples.

## Security

Workflows can run local commands. Review each `command` before running it. Use `graph` and `dry-run` to audit the plan, and prefer `-it` when you want to keep execution in the foreground.

## Next Steps

1. Run `go run ./cmd/agentflow validate samples/workflows/fix-github-issue.yaml`.
2. Execute `go run ./cmd/agentflow dry-run ...` against a real workflow in your repository.
3. Copy a sample into `.agentflow/workflows/` and adapt it to your process.
