# Agent Channels

AgentFlow keeps delivery channels outside the assistant orchestration core.
The intended dependency direction is:

```text
web   -> agentchannel -> agentflow
slack -> agentchannel -> agentflow
```

## Layers

- `internal/agentchannel` owns channel-neutral conversations, messages, tool
  calls, diagnostics, assistant execution, and workflow tools.
- `internal/web` owns HTTP, local auth, assets, SSE, browser diagnostics, and
  request/response translation.
- Future channel adapters, such as Slack, should own their transport details
  and call `agentchannel.Service` with neutral inputs.

## Boundary Rules

`internal/agentchannel` must not import delivery adapters or transport APIs:

- no `internal/web`
- no `internal/slack`
- no `net/http`

The architecture test in `internal/agentchannel` enforces these imports for
production Go files.

## External Conversation Identity

Channel adapters map their native conversation identity into opaque fields:

- `source`
- `external_key`
- `external_workspace_id`
- `external_channel_id`
- `external_thread_id`
- `external_user_id`

For Slack, an adapter can build an `external_key` such as
`slack:{team_id}:{channel_id}:{thread_ts}`. The core stores and reuses that key
without interpreting Slack-specific semantics.
