# Agentes

## Objetivo

Esta feature permite executar nós `agent` do workflow através de um registry de providers, com `codex` como provider padrão. O registry atual inclui `codex` e `claude`, mantendo o runtime desacoplado da implementação concreta do agente.

Na prática, isso habilita workflows como [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml), [`samples/workflows/security-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml) e [`samples/workflows/claude-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml), onde o runtime resolve o nó, escolhe o provider e repassa a execução para o CLI configurado.

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

O registry está em [`internal/core/ports/registry.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/registry.go). Ele mantém um mapa de providers por nome e é a base para a resolução de `provider: codex` e `provider: claude`.

O adapter do Codex fica em [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go). Ele:

- cria o cliente do Codex CLI com `CODEX_PATH` e `OPENAI_API_KEY`;
- combina o ambiente do processo com `env` do workflow;
- resolve `model` com prioridade para o valor do nó e depois `CODEX_MODEL`;
- normaliza o sandbox, incluindo aliases aceitos pelo adapter;
- executa o turno com `output_schema` quando presente;
- injeta o `system` no prompt final no formato `System:` / `User:`;
- coleta a resposta final, eventos e usage do provider.

O adapter do Claude Code fica em [`internal/adapters/agent/claude/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/claude/provider.go). Ele:

- chama o CLI em modo não interativo com `-p --output-format json`;
- resolve o binário por `--claude-path`, `CLAUDE_PATH` ou `PATH`;
- combina o ambiente do processo com `env` do workflow;
- encaminha `model`, `system`, `prompt`, `working_dir`, `sandbox` e `output_schema`;
- restringe ferramentas em `read-only` e autoaprova ferramentas conhecidas por `sandbox.mode` quando o runtime deriva o modo de `permission.write`;
- coleta `structured_output` emitido pelo Claude Code, usage e payload bruto em metadados.

Quando `output_schema` está definido, o provider preenche `result.JSON` quando recebe saída estruturada. O Codex usa a resposta final parseada como JSON; o Claude Code usa `structured_output` e só cai para parse do texto quando esse campo não está presente. Se não houver JSON estruturado, o texto bruto continua disponível em `result.Text`.

## Arquivos envolvidos

- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go): resolve o provider do nó `agent`, aplica o sandbox efetivo e encaminha a chamada ao registry.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): calcula o sandbox efetivo a partir de `permission.write` e dos valores explícitos do nó.
- [`internal/core/ports/agent.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go): define o contrato de request e response entre runtime e provider.
- [`internal/core/ports/registry.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/registry.go): implementa o registry estático de providers.
- [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go): adapter concreto do Codex CLI.
- [`internal/adapters/agent/claude/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/claude/provider.go): adapter concreto do Claude Code CLI.
- [`internal/adapters/agent/codex/provider_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_test.go): cobre normalização de sandbox e merge de ambiente.
- [`internal/adapters/agent/codex/provider_integration_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_integration_test.go): valida o contrato real com o Codex CLI, incluindo forwarding de args, schema, events e usage.
- [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml): exemplo de nó `agent` com `output_schema` e saída estruturada.
- [`samples/workflows/security-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml): exemplo de múltiplos nós `agent` usando o provider padrão.
- [`samples/workflows/claude-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml): exemplo mínimo com `provider: claude`.

## Observações relevantes

- O provider padrão é `codex`; se o workflow não informar `provider`, o runtime usa esse nome.
- A implementação atual preserva os eventos emitidos pelo provider em `RawEvents`, o que ajuda na auditoria e em integrações futuras.
- O usage do provider é propagado para o resultado final quando o CLI retorna informações de tokens.
- O adapter aceita `seatbelt` como alias de `workspace-write` para compatibilidade com ambientes que expõem esse nome.
- `permission.write` só é validado quando o bloco `permission` existe; se o workflow usar essa seção, `write` precisa estar presente.
- O sandbox do Claude Code é expresso por políticas de ferramentas do CLI; ele não é equivalente ao sandbox do Codex.
- Os testes de integração do Codex dependem do binário `codex` ou de `CODEX_PATH`, então podem ser pulados em ambientes sem o CLI instalado.
