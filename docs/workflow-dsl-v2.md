# Workflow DSL V2

> **Status:** fully implemented and executed.

## Overview

Workflow DSL V2 introduces structural improvements over V1 while keeping V1 workflows fully compatible. The version string is `version: "2"`.

## Compatibility with V1

V1 workflows continue to work unchanged. The main differences in V2 are:

- New top-level blocks: `imports`, `outputs`, `hooks`, `steps`.
- Nodes can reference reusable steps via `ref` and `params`.
- Inputs and outputs accept an optional `schema` field for JSON Schema subset validation.
- Static expression validation is performed during workflow validation for V2.

Migration from V1 to V2 is mechanical: run `agentflow migrate <workflow.yaml> --to 2`.

## New top-level blocks

### `imports`

List of external workflow files to import. Paths are resolved relative to the file that declares them. Cycles are rejected with a readable path.

```yaml
imports:
  - path: common-steps.yaml
```

Merge rules:

- Imported `inputs`, `vars`, `secrets`, `outputs`, and `steps` are merged by key. The main file may override imported map entries.
- Imported `defaults` and `execution` fields are merged field-by-field; the main file wins when a field is explicitly set.
- Imported `worktree` is only overridden if the main file sets `enabled: true`.
- Imported `hooks` are appended before local hooks.
- Imported `nodes` precede local nodes. Duplicate node IDs across imports and the main file raise an error.
- Duplicate step names across imports and the main file raise an error.

### `outputs`

Named workflow outputs with optional `type` and JSON Schema subset validation.

```yaml
outputs:
  result:
    value: "${nodes.final.output}"
    type: string
  summary:
    value: "${nodes.aggregate.output}"
    schema:
      type: object
      properties:
        count:
          type: integer
```

Rules:

- `value` is required.
- `type` must be one of `string`, `integer`, `number`, `boolean`, `array`, `object`.
- `schema` is validated against a conservative JSON Schema subset (`type`, `required`, `properties`, `items`, `enum`).
- `type` and `schema.type` must not conflict.
- Values are evaluated at the end of a successful run using the final node state.
- Evaluated outputs are persisted to `artifacts/workflow/outputs.json` and emitted as a `workflow.outputs` event.
- If an output value does not match its declared `type` or `schema`, the run fails with a localized error (`outputs.<name>`).

### `hooks`

Lifecycle hooks with shell commands.

```yaml
hooks:
  - phase: before_run
    kind: bash
    command: echo "starting"
    env:
      LOG_LEVEL: debug
    working_dir: .
    timeout: 30
```

Supported phases:

| Phase           | When it runs                                         | Failure behavior                                          |
| --------------- | ---------------------------------------------------- | --------------------------------------------------------- |
| `before_run`    | After run initialization, before worktree and nodes  | Run fails immediately; nodes are not executed             |
| `after_success` | After nodes and workflow outputs finish successfully | Run fails; outputs are already persisted                  |
| `after_failure` | After a node, hook, or output failure                | Error is appended to the original cause                   |
| `after_run`     | At the end of the run (success, failure, cancelled)  | Error is appended to the original cause; skipped on pause |

Rules:

- `phase` is required and must be one of the supported phases.
- `kind` is required and currently only `bash` is supported.
- `command` is required.
- `timeout`, `working_dir`, and `env` follow the same rules as `bash` nodes.
- Commands are evaluated with the same template context as nodes (`inputs`, `vars`, `secrets`, `nodes`, `run`).
- Hooks emit `hook.started`, `hook.finished`, and `hook.failed` events.
- Hook stdout, stderr, and exit code are persisted to `artifacts/hooks/<phase>/<index>/`.
- Secrets are masked in hook artifacts and events.
- `after_run` is not executed when the run is paused, to avoid duplication on resume.
- `before_run` is not re-executed when resuming a paused run.

### `steps`

Reusable step definitions (macros). They expand into concrete nodes before validation and planning.

```yaml
steps:
  notify:
    parameters:
      - message
    nodes:
      - id: send
        kind: bash
        command: "echo ${message}"
```

## Input schema

V2 inputs accept an optional `schema` field for JSON Schema subset validation. When `schema` is present, it takes precedence over `type` for runtime value validation, but `type` must not conflict with `schema.type`.

```yaml
inputs:
  count:
    type: integer
    schema:
      type: integer
      enum: [1, 2, 4, 8]
  config:
    schema:
      type: object
      required: [enabled]
      properties:
        enabled:
          type: boolean
        retries:
          type: integer
```

Supported schema subset:

- `type`: `string`, `integer`, `number`, `boolean`, `array`, `object`
- `required`: array of property names (objects only)
- `properties`: map of property names to nested schemas (objects only)
- `items`: schema for array elements (arrays only)
- `enum`: array of allowed values

Validation occurs when inputs are provided at run time and when defaults are checked during workflow validation.

## Node references (`ref` and `params`)

V2 nodes can reference a reusable step and pass parameters. Parameters are substituted statically using `${paramName}` syntax.

```yaml
nodes:
  - id: greet
    kind: noop
    ref: notify
    params:
      message: "hello world"
```

When a step produces multiple nodes, the caller node **must** declare an `id`, which is used as a prefix for all generated node IDs and their dependencies.

```yaml
steps:
  pipeline:
    parameters:
      - target
    nodes:
      - id: build
        kind: bash
        command: "build ${target}"
      - id: test
        kind: bash
        command: "test ${target}"
        depends_on: [build]

nodes:
  - id: ci
    ref: pipeline
    params:
      target: app
```

This expands to:

```yaml
nodes:
  - id: ci-build
    kind: bash
    command: "build app"
  - id: ci-test
    kind: bash
    command: "test app"
    depends_on: [ci-build]
```

Rules:

- Unknown steps or missing parameters raise an error.
- Recursive step expansion is rejected.
- V1 workflows cannot use `ref` or `steps`.

## Expressions

V2 uses the same expression engine as V1 (`expr-lang`) with an extended set of helpers.

### Available variables

- `inputs` — map of provided inputs
- `vars` — map of workflow vars
- `secrets` — map of resolved secrets
- `nodes` — map of node results (`status`, `output`, `outputs`, `stdout`, `stderr`, `exit_code`, `error`)
- `item` — current item in `for_each`
- `index` — current index in `for_each`
- `total` — total items in `for_each`
- `run` — map with run metadata

### Helpers

| Helper                 | Description                                             | Example                                     |
| ---------------------- | ------------------------------------------------------- | ------------------------------------------- |
| `exists(v)`            | Returns `true` if `v` is not nil                        | `exists(inputs.name)`                       |
| `success(id)`          | Returns `true` if node `id` status is `success`         | `success('plan')`                           |
| `failed(id)`           | Returns `true` if node `id` status is `failed`          | `failed('test')`                            |
| `contains(a, b)`       | Returns `true` if `a` contains `b`                      | `contains(inputs.tags, 'deploy')`           |
| `len(v)`               | Returns length of string, slice, map, or array          | `len(inputs.items)`                         |
| `default(v, fallback)` | Returns `v` unless nil or empty string, then `fallback` | `default(inputs.name, 'world')`             |
| `matches`              | Built-in regex operator (not a function call)           | `inputs.email matches "^.*@example\\.com$"` |
| `json(s)`              | Parses a JSON string into a value                       | `json(nodes.parse.output).count`            |

### Template syntax

Expressions inside strings use `${...}`:

```yaml
command: "echo ${inputs.name}"
when: "len(inputs.items) > 0 && success('load')"
```

When a field is a single `${...}` expression, the evaluated value retains its original type (e.g., array, number). When embedded in a larger string, the result is always a string.

## Validation

- V1 workflows continue to use the same rules and reject V2 fields with clear errors.
- V2 workflows reuse common validation (name, nodes, inputs, worktree, plan) after imports are resolved and macros are expanded.
- V2 performs static expression compilation on node fields (`when`, `prompt`, `command`, `for_each`, etc.) and on `outputs` values to catch syntax errors early.
- Missing or unknown versions are rejected before any other validation.

## Migration guide

To migrate a V1 workflow to V2:

```bash
agentflow migrate workflow.yaml --to 2
```

This produces an equivalent V2 workflow with:

- `version: "2"`
- All existing fields preserved unchanged
- No semantic transformation of macros or imports (V1 has none)

You can redirect the output to a file:

```bash
agentflow migrate workflow.yaml --to 2 --out workflow-v2.yaml
```

After migration, you can optionally:

- Add `outputs` to expose run results explicitly.
- Extract reusable node patterns into `steps` and reference them with `ref`.
- Add `hooks` for lifecycle automation.
- Add `schema` to inputs for stronger validation.

## Roadmap

| Feature                | Status         |
| ---------------------- | -------------- |
| Model & validation     | ✅ Implemented |
| YAML decode            | ✅ Implemented |
| Import resolution      | ✅ Implemented |
| Step expansion         | ✅ Implemented |
| Hook execution         | ✅ Implemented |
| Output materialization | ✅ Implemented |
| Expression helpers     | ✅ Implemented |
| Static expr validation | ✅ Implemented |
| Migration command      | ✅ Implemented |
