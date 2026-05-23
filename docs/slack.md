# Slack Adapter

`agentflow slack` runs the AgentFlow assistant inside Slack using Socket Mode.
It connects locally, reuses the same `agentchannel` core as the web adapter, and
stores Slack threads as durable sessions.

## Runtime Contract

- Requires an app-level token (`xapp-...`) and a bot token (`xoxb-...`).
- Requires a project name so Slack conversations can resolve to a project root.
- Reuses the same root directory and chat-agent settings as the web adapter.
- Listens for `app_mention` events and direct messages in `message.im`.
- Posts assistant replies back into the originating Slack thread.

## Configuration

The Slack adapter reads its defaults from `~/.agentflow/settings.toml`:

```toml
[slack]
app_token = "xapp-..."
bot_token = "xoxb-..."
project = "demo"
```

CLI flags still override the file when present.

Flags:

- `--app-token`
- `--bot-token`
- `--project`
- `--root`
- `--daemon`
- `--daemon-socket`
- `--log`

Environment variables:

- `AGENTFLOW_SLACK_APP_TOKEN`
- `AGENTFLOW_SLACK_BOT_TOKEN`
- `AGENTFLOW_SLACK_PROJECT`
- `AGENTFLOW_SLACK_ROOT`
- `AGENTFLOW_SLACK_DAEMON`
- `AGENTFLOW_SLACK_DAEMON_SOCKET`
- `AGENTFLOW_SLACK_CHAT_AGENT_PROVIDER`
- `AGENTFLOW_SLACK_CHAT_AGENT_MODEL`
- `AGENTFLOW_SLACK_CHAT_AGENT_TIMEOUT`
- `AGENTFLOW_SLACK_CHAT_AGENT_HISTORY_LIMIT`

The Slack adapter maps these values into the same shared settings shape used by
the web command, which keeps the chat agent and daemon behavior aligned across
channels.
