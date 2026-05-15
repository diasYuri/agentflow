# Repository Guidelines

## Project Structure & Module Organization

- `cmd/agentflow/` contains the CLI entrypoint.
- `internal/` holds the application code, split by responsibility: `cli/`, `app/`, `domain/`, `adapters/`, `ports/`, and `runtime/`.
- `samples/workflows/` stores YAML workflow examples.
- `samples/inputs/` stores matching JSON and Markdown inputs used by the samples.
- Runtime artifacts are written under `.agentflow/runs/` by default.

## Build, Test, and Development Commands

- `go run ./cmd/agentflow validate <workflow.yaml>` validates a workflow file.
- `go run ./cmd/agentflow graph <workflow.yaml>` prints a Mermaid graph for the workflow.
- `go run ./cmd/agentflow dry-run <workflow.yaml> --input-json <file>` resolves inputs and shows the execution plan without running it.
- `go run ./cmd/agentflow run <workflow.yaml> --input-json <file>` executes a workflow locally.
- `go test ./...` runs the full test suite. This is the verified baseline for this repository.
- `go build ./cmd/agentflow` builds the CLI binary.

## Coding Style & Naming Conventions

- Use standard Go formatting with `gofmt` on every changed `.go` file.
- Keep packages small and focused; prefer clear names that match the directory purpose.
- Use `camelCase` for locals and `PascalCase` for exported identifiers.
- Test files should be named `*_test.go` and use `Test...` function names.

## Testing Guidelines

- Favor table-driven unit tests in `internal/domain/...` and `internal/app/...`.
- Keep workflow fixtures in `samples/` when a test needs realistic YAML or JSON input.
- Add regression tests for CLI behavior in `internal/cli/root_test.go` style when flags or output change.

## Commit & Pull Request Guidelines

- No repository-specific commit pattern was visible in this checkout, so use short imperative subjects, such as `add workflow validation test`.
- Keep pull requests focused on one change set.
- Include a summary of behavior changes, test results, and any sample workflow updates.
- Attach screenshots or terminal output only when the change affects CLI output or generated artifacts.

## Security & Configuration Tips

- Review sample workflows before running them; `run` can execute local shell commands.
- Be cautious with `--codex-path` and `--working-dir`, especially when pointing outside the repository.
