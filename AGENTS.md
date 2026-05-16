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

## Code Quality & Design Principles

- Apply SOLID principles pragmatically: keep each type or package focused on one responsibility, depend on narrow interfaces at package boundaries, and avoid forcing callers to know unnecessary implementation details.
- Keep code DRY, but do not introduce abstractions before duplication has a clear pattern. Prefer small helper functions or well-named types when repeated logic starts to hide intent.
- Avoid `.go` files longer than 300 lines. When a file grows beyond that, split it by responsibility, workflow stage, or cohesive type group.
- Keep functions short and readable. If a function needs many branches, hidden state, or long setup blocks, extract focused helpers or move behavior behind an interface.
- Use design patterns only when they simplify real complexity. For example, prefer Strategy for interchangeable behaviors, Factory for construction with multiple variants, Adapter for external integrations, and Builder/Options for complex configuration.
- Preserve the existing architecture boundaries: domain code should stay independent from CLI, filesystem, process execution, and other adapter concerns.
- Favor explicit error handling with contextual messages. Use wrapping with `%w` when callers may need to inspect the original error.
- Avoid global mutable state. Prefer dependency injection through constructors or small interfaces, especially for filesystem, clock, command execution, and runtime dependencies.
- Keep concurrency simple and bounded. Use `context.Context` for cancellation, close channels from the sender side, and avoid goroutine leaks with clear ownership.

## Go Performance Guidelines

- Start with clear code, then optimize using evidence from benchmarks, profiles, or hot-path analysis.
- Avoid unnecessary allocations in tight loops; reuse buffers carefully when ownership is clear and prefer preallocated slices when final size is known.
- Do not convert between `string` and `[]byte` repeatedly in hot paths unless required by an API.
- Use streaming APIs for large files or command output instead of loading entire payloads into memory.
- Prefer maps, slices, and simple structs before heavier abstractions when modeling hot-path data.
- Keep logging and formatted string construction out of tight loops unless guarded by level checks or proven acceptable.
- Add benchmarks for performance-sensitive parser, planner, runtime, or adapter behavior before making non-trivial optimizations.

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
