# Extension Nodes

`kind: extension` runs local JavaScript or TypeScript extension code through the
official AgentFlow Bun RPC adapter. The adapter owns the RPC protocol; extension
authors export regular functions with the `@agentflow/extensions` SDK.

## Directory Layout

AgentFlow resolves extensions by name, using the project-local extension first
and then the user extension:

1. `<working_dir>/.agentflow/extensions/<extension-name>/`
2. `~/.agentflow/extensions/<extension-name>/`

Each extension should be a Bun-managed project:

```text
.agentflow/extensions/jira/
  package.json
  bun.lock
  src/main.ts
```

The `agentflow-extension-rpc` binary must be available in `PATH`, or its path
must be set with `AGENTFLOW_EXTENSION_RPC`.

## Installation

Install the adapter binary globally:

```bash
npm install -g @agentflow/extensions
```

Then add the SDK to each extension project:

```bash
bun add @agentflow/extensions
```

## Workflow Usage

```yaml
nodes:
  - id: jira_lookup
    kind: extension
    extension: jira
    runtime: bun
    mode: oneshot
    script: src/main.ts
    operation: lookupIssue
    with:
      issue_key: "${inputs.issue_key}"
    env:
      JIRA_BASE_URL: "${vars.jira_base_url}"
```

`runtime` defaults to `bun`. `mode` defaults to `oneshot`; use `server` to keep
one adapter process alive per extension for the duration of a run. `extension`
must be a simple directory name. `script` must be relative to the extension
directory and cannot escape it with `..`.

## Script Contract

Use the SDK so extension code does not need to know about RPC:

```ts
import { defineExtension } from "@agentflow/extensions";

export default defineExtension({
  async run(ctx) {
    return {
      issue_key: ctx.with.issue_key,
      status: "ok",
    };
  },
  operations: {
    async lookupIssue(ctx) {
      return {
        issue_key: ctx.with.issue_key,
        status: "ok",
      };
    },
  },
});
```

The `ctx` object includes:

- `version`: `agentflow.extension.v1`
- `run`: run id and workflow name
- `node`: node id, attempt, instance id, index, and total
- `context`: inputs, vars, secrets, prior node state, and current item
- `with`: rendered node inputs
- `extension`: extension name, directory, and script path
- `working_dir`: resolved workflow working directory
- `logger`: stderr-backed logger

Return values become `nodes.<id>.output`. Logs should use `ctx.logger` or
`console.*`; the adapter redirects them to stderr so stdout remains reserved for
RPC.

Python extensions through `uv` are no longer supported.
