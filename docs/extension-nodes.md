# Extension Nodes

`kind: extension` runs a local Python extension script through `uv`. This is the
escape hatch for integrations that do not belong in the core runtime, such as
Jira, GitHub, search APIs, internal tools, or project-specific automation.

## Directory Layout

AgentFlow resolves extensions by name, using the project-local extension first
and then the user extension:

1. `<working_dir>/.agentflow/extensions/<extension-name>/`
2. `~/.agentflow/extensions/<extension-name>/`

Each extension should be a uv-managed Python project:

```text
.agentflow/extensions/jira/
  pyproject.toml
  uv.lock
  main.py
```

The runtime invokes:

```bash
uv run --project <extension-dir> python <script-path>
```

`uv` is required. AgentFlow does not silently fall back to global `python3`,
because the extension contract relies on per-extension dependency isolation.

## Workflow Usage

```yaml
nodes:
  - id: jira_lookup
    kind: extension
    extension: jira
    script: main.py
    with:
      issue_key: "${inputs.issue_key}"
    env:
      JIRA_BASE_URL: "${vars.jira_base_url}"
```

`extension` must be a simple directory name. `script` must be relative to the
extension directory and cannot escape it with `..`.

## Script Contract

AgentFlow writes a single JSON object to stdin. The payload includes:

- `version`: `agentflow.extension.v1`
- `run`: run id and workflow name
- `node`: node id, attempt, instance id, index, and total
- `context`: inputs, vars, secrets, prior node state, and current item
- `with`: rendered node inputs
- `extension`: extension name, directory, and script path
- `working_dir`: resolved workflow working directory

The script must write valid JSON to stdout. Any JSON value is accepted and
becomes `nodes.<id>.output`. Write logs to stderr.

Example script:

```python
import json
import sys

payload = json.load(sys.stdin)
issue_key = payload["with"]["issue_key"]
print(json.dumps({"issue_key": issue_key, "status": "ok"}))
```

Non-zero exit codes, invalid JSON stdout, missing scripts, and missing `uv`
fail the node.

