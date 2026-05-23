# Arquitetura do daemon

O AgentFlow passa a separar execução e controle em dois processos locais:

- `agentflowd`: processo central que executa e gerencia workflows em background.
- `agentflow`: CLI cliente que controla o daemon por RPC local.

## Supervisão

O daemon usa `suture` como raiz de supervisão. O supervisor principal registra:

- `agentflowd-workflows`: sub-supervisor que recebe um serviço por workflow em background.
- `Manager`: mantém o estado em memória dos runs conhecidos e cancela workflows no shutdown.
- `Server`: servidor RPC HTTP em Unix socket.

Cada workflow iniciado via RPC é executado como `WorkflowRunService` registrado no sub-supervisor de workflows. O serviço recebe o contexto do `suture`, cria um contexto cancelável para o run e chama o runtime existente. Ao terminar com sucesso, falha ou cancelamento, retorna `suture.ErrDoNotRestart`, porque uma falha de workflow é resultado de negócio e não deve reiniciar automaticamente o mesmo run.

O shutdown do daemon cancela o contexto raiz. Isso faz o `Server` parar de aceitar requisições, o `Manager` cancelar runs ativos e o supervisor aguardar a finalização dos serviços filhos.

## RPC local

A RPC v1 usa HTTP sobre Unix domain socket:

- Socket: `~/.agentflow/agentflowd.sock`
- PID file: `~/.agentflow/agentflowd.pid`
- Log do daemon: `~/.agentflow/agentflowd.log`
- Índice SQLite: `~/.agentflow/agentflowd.sqlite`
- Runs: `~/.agentflow/runs`

O daemon usa SQLite como índice persistente de runs. Os metadados de `workflow runs`
e `workflow status` sobrevivem a restart do processo, enquanto os payloads auditáveis
continuam no filesystem em `~/.agentflow/runs`. Se o daemon reiniciar com runs que
estavam `created` ou `running`, eles são reidratados como `cancelled`, pois não há
processo ativo para continuar a execução anterior. Runs em `paused` continuam `paused`
após restart e podem ser retomados manualmente; o request original do run é persistido
em coluna `request_json` para permitir a reconstrução do `WorkflowRunService` com o
mesmo `run_id`.

Endpoints:

- `GET /v1/daemon/status`
- `POST /v1/daemon/stop`
- `POST /v1/workflows`
- `GET /v1/workflows`
- `GET /v1/workflows/{id}`
- `GET /v1/workflows/{id}/logs`
- `POST /v1/workflows/{id}/cancel`
- `POST /v1/workflows/{id}/pause`
- `POST /v1/workflows/{id}/resume`

`pause` envia uma solicitação cooperativa ao runtime via `PauseController`; o run pausa
no próximo checkpoint seguro. Retorna 409 quando o run já está em estado terminal e
404 quando o `run_id` não é conhecido.

`resume` aceita apenas runs em `paused`. Ele recarrega o request persistido, recria o
`RunWorkflowUseCase` com as mesmas opções e adiciona um novo `WorkflowRunService` ao
sub-supervisor com o mesmo `run_id` em modo resume. Retorna 409 para runs `success`,
`failed` ou `cancelled`.

## CLI

`agentflow workflow run <workflow>` resolve o workflow no CLI, envia o caminho absoluto ao daemon e retorna imediatamente com `run_id`, `run_dir` e `status`.

`agentflow workflow run -it <workflow>` executa no processo da CLI, preservando o comportamento foreground/interativo. O alias legado `agentflow run` segue a mesma regra: daemon por padrão, local apenas com `-it`.

`validate`, `graph` e `dry-run` continuam locais porque são operações de inspeção e não precisam de ciclo de vida em background.
