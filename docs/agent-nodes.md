# Agentes

## Objetivo

Esta feature permite executar nós `agent` do workflow através de um registry de providers, com `codex` como provider padrão. O registry atual inclui `codex`, `claude` e `pi`, mantendo o runtime desacoplado da implementação concreta do agente.

Na prática, isso habilita workflows como [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml), [`samples/workflows/security-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml), [`samples/workflows/claude-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml) e [`samples/workflows/pi-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/pi-code-review.yaml), onde o runtime resolve o nó, escolhe o provider e repassa a execução para o CLI configurado.

## Como funciona

O dispatch de nós acontece em [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go). Quando o nó é do tipo `agent`:

1. o runtime renderiza o `prompt` com o contexto de execução;
2. o provider é resolvido por nome no registry;
3. se `provider` estiver vazio, o runtime usa `codex`;
4. o runtime monta o sandbox efetivo a partir de `permission.write`, a menos que `sandbox.mode` já tenha sido definido explicitamente;
5. o request é enviado ao provider com `prompt`, `system`, `model`, `working_dir`, `env`, `sandbox` e `output_schema`;
6. o resultado volta como texto bruto ou JSON estruturado, dependendo da resposta do provider.

A regra de sandbox é aplicada em [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go):

- `sandbox.mode` explícito sempre vence;
- `permission.write: true` mapeia para `workspace-write`;
- `permission.write: false` mapeia para `read-only`;
- quando `permission` não é informado, o sandbox fica indefinido no runtime e o provider pode usar seu fallback próprio.

O contrato entre runtime e provider é definido em [`internal/core/ports/agent.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go). O `AgentRequest` carrega os dados necessários para o provider escolhido, e o `AgentResult` preserva:

- `Text`, com a resposta final em texto;
- `JSON`, quando a resposta final pode ser parseada como JSON;
- `RawEvents`, com os eventos emitidos pelo provider;
- `Usage`, com contadores de tokens;
- `Metadata`, para extensões futuras.

O registry está em [`internal/core/ports/registry.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/registry.go). Ele mantém um mapa de providers por nome e é a base para a resolução de `provider: codex`, `provider: claude` e `provider: pi`.

O adapter do Codex fica em [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go). Ele:

- cria o cliente do Codex CLI com `CODEX_PATH` e `OPENAI_API_KEY`;
- combina o ambiente do processo com `env` do workflow;
- resolve `model` com prioridade para o valor do nó e depois `CODEX_MODEL`;
- normaliza o sandbox, incluindo aliases aceitos pelo adapter;
- executa o turno com `output_schema` quando presente;
- injeta o `system` no prompt final no formato `System:` / `User:`;
- coleta a resposta final, eventos e usage do provider.

O adapter do Claude Code fica em [`internal/adapters/agent/claude/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/claude/provider.go). Ele:

- chama o CLI em modo não interativo com `-p --output-format json --no-session-persistence`;
- envia o `prompt` pelo stdin do processo, evitando absorção por flags variádicas;
- resolve o binário por `--claude-path`, `CLAUDE_PATH` ou `PATH`;
- combina o ambiente do processo com `env` do workflow;
- encaminha `model`, `system`, `working_dir` e `output_schema` como flags do CLI;
- traduz `sandbox.mode`: `read-only` vira `--tools Read,Glob,Grep,LS` e `workspace-write` vira `--permission-mode acceptEdits`;
- promove `is_error: true` no payload do Claude para erro do nó, com `subtype` e `result` no texto;
- coleta `structured_output`, `usage` (com cache tokens somados em `TotalTokens`), `permission_denials` e `total_cost_usd` em metadados.

O adapter do Pi fica em [`internal/adapters/agent/pi/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/pi/provider.go). Ele:

- chama `pi --mode rpc --no-session` e conversa por JSONL em stdin/stdout;
- resolve o binário por `--pi-path`, `AGENTFLOW_PI_PATH`, `PI_PATH` ou `PATH`;
- combina o ambiente do processo com `env` do workflow;
- encaminha `model`, `system`, `prompt`, `working_dir` e `sandbox`;
- restringe ferramentas para `read,grep,find,ls` quando o sandbox é `read-only`;
- usa `get_last_assistant_text` e `get_session_stats` para preencher resposta final e usage;
- quando `output_schema` existe, instrui o agente a responder somente JSON e parseia o texto final.

Quando `output_schema` está definido, o provider preenche `result.JSON` quando recebe saída estruturada. O Codex usa a resposta final parseada como JSON; o Claude Code usa `structured_output` e só cai para parse do texto quando esse campo não está presente; o Pi parseia o último texto do assistant como JSON. Se não houver JSON estruturado, o texto bruto continua disponível em `result.Text`, exceto no Pi quando `output_schema` foi solicitado e o texto não é JSON válido.

## Arquivos envolvidos

- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go): resolve o provider do nó `agent`, aplica o sandbox efetivo e encaminha a chamada ao registry.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): calcula o sandbox efetivo a partir de `permission.write` e dos valores explícitos do nó.
- [`internal/core/ports/agent.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go): define o contrato de request e response entre runtime e provider.
- [`internal/core/ports/registry.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/registry.go): implementa o registry estático de providers.
- [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go): adapter concreto do Codex CLI.
- [`internal/adapters/agent/claude/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/claude/provider.go): adapter concreto do Claude Code CLI.
- [`internal/adapters/agent/pi/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/pi/provider.go): adapter concreto do Pi RPC.
- [`internal/adapters/agent/codex/provider_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_test.go): cobre normalização de sandbox e merge de ambiente.
- [`internal/adapters/agent/codex/provider_integration_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_integration_test.go): valida o contrato real com o Codex CLI, incluindo forwarding de args, schema, events e usage.
- [`internal/adapters/agent/claude/provider_integration_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/claude/provider_integration_test.go): teste com build tag `claude_integration` que executa o Claude Code real e valida `output_schema`, `Usage` e mapeamento de sandbox.
- [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml): exemplo de nó `agent` com `output_schema` e saída estruturada.
- [`samples/workflows/security-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml): exemplo de múltiplos nós `agent` usando o provider padrão.
- [`samples/workflows/claude-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml): exemplo mínimo com `provider: claude`.
- [`samples/workflows/pi-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/pi-code-review.yaml): exemplo mínimo com `provider: pi`.

## Observações relevantes

- O provider padrão é `codex`; se o workflow não informar `provider`, o runtime usa esse nome.
- A implementação atual preserva os eventos emitidos pelo provider em `RawEvents`, o que ajuda na auditoria e em integrações futuras.
- O usage do provider é propagado para o resultado final quando o CLI retorna informações de tokens.
- O adapter aceita `seatbelt` como alias de `workspace-write` para compatibilidade com ambientes que expõem esse nome.
- `permission.write` só é validado quando o bloco `permission` existe; se o workflow usar essa seção, `write` precisa estar presente.
- O sandbox do Claude Code é expresso por políticas de ferramentas do CLI (`--tools` em read-only) e `--permission-mode` em workspace-write; não é equivalente ao sandbox do Codex.
- O sandbox read-only do Pi é expresso por allowlist de ferramentas do CLI; os demais modos usam o conjunto padrão do Pi.
- Os testes de integração do Codex dependem do binário `codex` ou de `CODEX_PATH`, então podem ser pulados em ambientes sem o CLI instalado.
