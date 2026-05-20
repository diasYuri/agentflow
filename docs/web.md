# Agentflow Web

## Goal

`agentflow web` starts a local-first HTTP server that hosts the conversation studio. The web UI provides a project rail, session rail, workflow execution dashboard, conversation timeline with streaming updates, tool-call and approval cards, workflow YAML/graph editors, diagnostics panels, and settings -- all backed by the local API and SSE stream.

## Quick start

```bash
agentflow web          # starts on 127.0.0.1:38080
agentflow web --no-open --port 0
```

On startup the command prints the chosen URL, listen address, and session token. The URL already carries the token via `?token=...` so the browser can store it on first load.

## Configuration

Settings are merged in this order (each layer overrides the previous one):

1. **Defaults** -- `127.0.0.1:38080`, browser auto-open, daemon mode `auto`.
2. **TOML file** -- `~/.agentflow/settings.toml`, optional.
3. **Environment variables** -- anything prefixed with `AGENTFLOW_WEB_`.
4. **CLI flags** -- see `agentflow web --help`.

### TOML schema

```toml
[web]
host = "127.0.0.1"
port = 38080
open_browser = true
dev_assets = ""             # serves the embedded shell when empty
daemon = "auto"             # auto | required | off

[web.auth]
token_override = ""         # pin a token instead of generating one

[paths]
root = ""                   # override the AgentFlow root directory
daemon_socket = ""          # override the agentflowd unix socket
```

### Environment variables

| Variable                       | Purpose                                |
| ------------------------------ | -------------------------------------- |
| `AGENTFLOW_WEB_HOST`           | bind host                              |
| `AGENTFLOW_WEB_PORT`           | bind port                              |
| `AGENTFLOW_WEB_OPEN_BROWSER`   | open the browser on startup            |
| `AGENTFLOW_WEB_DEV_ASSETS`     | directory served instead of embeds     |
| `AGENTFLOW_WEB_DAEMON`         | daemon requirement (auto/required/off) |
| `AGENTFLOW_WEB_TOKEN`          | pin the session token                  |
| `AGENTFLOW_WEB_DAEMON_SOCKET`  | override the agentflowd socket path    |
| `AGENTFLOW_WEB_ROOT`           | override the AgentFlow root directory  |

### CLI flags

| Flag             | Maps to                              |
| ---------------- | ------------------------------------ |
| `--host`         | `web.host`                           |
| `--port`         | `web.port` (use `0` to auto-select)  |
| `--no-open`      | `web.open_browser = false`           |
| `--dev-assets`   | `web.dev_assets`                     |
| `--daemon`       | `web.daemon`                         |
| `--root`         | `paths.root`                         |
| `--token`        | `web.auth.token_override`            |

## Routes

| Method | Path                                | Auth | Description                                         |
| ------ | ----------------------------------- | ---- | --------------------------------------------------- |
| GET    | `/`, `/index.html`                  | none | Serves the HTML shell                               |
| GET    | `/assets/*`                         | none | Embedded static assets (CSS/JS for the shell)       |
| GET    | `/api/v1/health`                    | none | `{status, version, started_at, daemon_mode}`        |
| GET    | `/api/v1/settings`                  | yes  | Subset of merged settings safe for the browser      |
| GET    | `/api/v1/projects`                  | yes  | List registered projects                            |
| GET    | `/api/v1/projects/{name}`           | yes  | Get project details                                 |
| GET    | `/api/v1/projects/{name}/sessions`  | yes  | List project sessions                               |
| POST   | `/api/v1/projects/{name}/sessions`  | yes  | Create a new session                                |
| GET    | `/api/v1/sessions`                  | yes  | List sessions (optionally filter by project)        |
| GET    | `/api/v1/sessions/{id}`             | yes  | Get session details                                 |
| PATCH  | `/api/v1/sessions/{id}`             | yes  | Update session title or status                      |
| DELETE | `/api/v1/sessions/{id}`             | yes  | Delete a session                                    |
| GET    | `/api/v1/sessions/{id}/messages`    | yes  | List session messages                               |
| POST   | `/api/v1/sessions/{id}/messages`    | yes  | Append a message                                    |
| GET    | `/api/v1/sessions/{id}/tool-calls`  | yes  | List tool calls                                     |
| POST   | `/api/v1/sessions/{id}/tool-calls`  | yes  | Record a tool call                                  |
| GET    | `/api/v1/sessions/{id}/approvals`   | yes  | List approvals                                      |
| POST   | `/api/v1/sessions/{id}/approvals`   | yes  | Create an approval                                  |
| POST   | `/api/v1/approvals/{id}/decide`     | yes  | Approve or reject an approval                       |
| GET    | `/api/v1/sessions/{id}/diagnostics` | yes  | List session diagnostics                            |
| GET    | `/api/v1/sessions/{id}/stream`      | yes  | SSE stream for session events                       |
| GET    | `/api/v1/stream`                    | yes  | SSE stream for all events                           |
| GET    | `/api/v1/diagnostics`               | yes  | Recent diagnostics across all sessions              |
| GET    | `/api/v1/workflows`                 | yes  | List workflow runs from `agentflowd`                |
| GET    | `/api/v1/workflows/{run_id}`        | yes  | Get workflow run status                             |
| GET    | `/api/v1/workflows/{run_id}/inspect` | yes | Get executive run metrics                           |
| GET    | `/api/v1/workflows/{run_id}/nodes`  | yes  | List node results for a run                         |
| GET    | `/api/v1/workflows/{run_id}/timeline` | yes | List run timeline entries                           |
| GET    | `/api/v1/workflows/{run_id}/events` | yes  | List run events                                     |
| GET    | `/api/v1/workflows/{run_id}/artifacts` | yes | List run artifacts                                  |
| POST   | `/api/v1/workflows/{run_id}/pause`  | yes  | Pause an active run                                 |
| POST   | `/api/v1/workflows/{run_id}/resume` | yes  | Resume a paused run                                 |
| POST   | `/api/v1/workflows/{run_id}/approve` | yes | Approve a run waiting for approval                  |
| POST   | `/api/v1/workflows/{run_id}/reject` | yes  | Reject a run waiting for approval                   |
| POST   | `/api/v1/workflows/{run_id}/cancel` | yes  | Cancel a run                                        |

Authenticated routes accept the session token via:

- `Authorization: Bearer <token>`
- `X-AgentFlow-Token: <token>`
- `?token=<token>` query parameter
- `agentflow_session` cookie

Requests from non-loopback peers are rejected with HTTP 403.

## Frontend features

### Workspace layout
- **Project rail** -- left sidebar showing registered projects; selecting a project filters sessions.
- **Session rail** -- second sidebar showing sessions for the selected project with search/filter.
- **Status bar** -- top bar with version, health indicator, daemon mode, theme selector, and nav links.

### Workflow dashboard
- Executive KPIs for active runs, success rate, failures, approvals, duration, and artifacts.
- Recharts widgets for run trend, status mix, and top workflows.
- TanStack Table history with text and status filters.
- Detail panel for the selected run with safe actions: pause, resume, approve, reject, and cancel.
- Polls `agentflowd` through `/api/v1/workflows` every few seconds; start the daemon first for live data.

### Conversation studio
- **Timeline** -- scrollable message history with user/assistant/system/tool role badges.
- **Composer** -- textarea with Ctrl+Enter shortcut to send messages.
- **SSE streaming** -- live message, tool-call, and approval updates via Server-Sent Events.
- **Right panel** -- tool-call lifecycle cards, approval actions (approve/reject), and recent diagnostics.

### Workflow editor
- **YAML editor** -- plain textarea with syntax-friendly monospace font; drafts persist to localStorage.
- **Graph view** -- canvas-based visualization of workflow nodes and dependency edges.
- **Toggle** -- switch between YAML and Graph views.

### Diagnostics
- Filter by level (error, warning, info, debug) and free-text search.
- Table view with time, level, source, message, and session columns.

### Settings
- Theme selection (System / Light / Dark).
- Reduced motion toggle.
- Server config read-only display.
- Session token clear and reload.

## Responsive behavior
- Sidebar can be collapsed via the `<<` / `>>` button in the status bar.
- Right panels (tool calls, approvals, diagnostics) are hidden on narrow screens (< 1024px).
- Reduced motion is respected via `prefers-reduced-motion` and a manual toggle.

## Dev assets

Pass `--dev-assets /path/to/frontend` to serve the shell and `/assets/*` from disk. The embedded copy is used as a fallback so the page still loads while the frontend build is in progress.

### Frontend development

The React frontend source lives in `web/`. To work on it:

```bash
cd web
bun install
bun run dev        # Vite dev server on :5173, proxies /api to :38080
bun run build      # outputs to ../internal/web/assets/
```

The Vite config proxies API calls to the Go server, so run `agentflow web` in another terminal first.

## Daemon coordination

`--daemon required` refuses to start when `agentflowd` is not running. `--daemon auto` (the default) warns but proceeds; `--daemon off` skips the check entirely. The web command never starts the daemon; use `agentflow daemon start` first.
