# Integração com Codex

## Objetivo

Esta feature conecta nós `agent` ao Codex CLI local, usando `codex` como provider padrão.
Para Claude Code, veja [`docs/claude-agent.md`](/Users/yuri/git/diasYuri/agentflow/docs/claude-agent.md).
O foco é permitir que workflows executem tarefas de geração, análise e revisão com o mesmo
runtime local do projeto, sem exigir uma camada extra de orquestração.

Além do envio do prompt, a integração repassa ao CLI os dados necessários para manter a
execução previsível:

- `model`
- `system`
- `prompt`
- `env`
- `working_dir`
- `sandbox`
- `output_schema`

O provider também expõe telemetria útil para o runtime, como uso de tokens, eventos brutos
emitidos pelo Codex e, quando a resposta é estruturada, o conteúdo já interpretado como JSON.

## Como funciona

O fluxo começa no runtime. Em [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go),
o dispatcher resolve nós `agent` e define `codex` como provider padrão quando o campo
`provider` não é informado no workflow.

Antes de chamar o provider, o runtime monta o `AgentRequest` com:

- `RunID`, `NodeID`, `InstanceID` e `Attempt`, para rastreio;
- `Model`, `System`, `Prompt`, `WorkingDir`, `Env`, `OutputSchema` e `Sandbox`;
- o provider escolhido no workflow, ou `codex` por padrão.

O `working_dir` é normalizado a partir do diretório base do run, usando a regra já adotada
pelos handlers locais. O `env` do nó é combinado com o ambiente do processo, e os valores do
workflow sobrescrevem variáveis já existentes quando há conflito.

Em [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go),
o sandbox do nó é normalizado antes da chamada ao provider. Quando o node não declara
`sandbox` explicitamente, o runtime deriva o modo a partir de `permission.write`:

- `permission.write: true` vira `workspace-write`;
- `permission.write: false` vira `read-only`;
- se não houver `permission`, o sandbox permanece vazio e a decisão fica para o provider.

No adapter [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go),
o request é convertido em uma chamada ao SDK do Codex:

- o binário usa `CODEX_PATH` quando definido, ou o path injetado pelo runtime;
- a API key vem de `OPENAI_API_KEY`;
- o ambiente do processo é preservado e mesclado com `req.Env`;
- o `model` usa o valor do node, com fallback para `CODEX_MODEL`;
- o `sandbox` usa o valor do node, com fallback para `CODEX_SANDBOX`;
- a política de aprovação é fixa em `never`;
- o diretório de trabalho é repassado diretamente ao thread;
- a checagem de repositório Git é ignorada para permitir uso em cenários locais variados.

O texto enviado ao Codex recebe um envelope simples quando há `system`:

```text
System:
...

User:
...
```

Ou seja, o `system` não é perdido, mas incorporado ao prompt que chega ao CLI.

Quando o turn retorna, o provider monta o resultado do agente:

- `Text` recebe a resposta final;
- `Usage` é preenchido a partir do consumo reportado pelo Codex;
- `RawEvents` guarda os itens emitidos pelo turn, preservando `type` e `data`;
- se `output_schema` estiver presente e a resposta final for JSON válido, `JSON` recebe o
  valor já decodificado.

## Arquivos principais

- [`internal/adapters/agent/codex/provider.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider.go): implementação do provider `codex`, criação do cliente, normalização de sandbox, mesclagem de ambiente e parsing do resultado.
- [`internal/adapters/agent/codex/provider_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_test.go): cobertura de normalização de sandbox e preservação de variáveis de ambiente.
- [`internal/adapters/agent/codex/provider_integration_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/agent/codex/provider_integration_test.go): verificação de contrato com o CLI real, incluindo flags, schema, uso e eventos.
- [`internal/core/ports/agent.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go): contrato do provider de agentes, formato do request e estrutura do resultado.
- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go): roteamento de nós `agent` e composição do request enviado ao provider.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): helpers de execução que derivam sandbox, working dir e outros valores efetivos.
- [`samples/workflows/fix-github-issue.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml): fluxo que usa agentes para análise, implementação, testes e resumo.
- [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml): fluxo que depende de `output_schema` para produzir release notes em JSON estruturado.
- [`samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml): exemplo de uso de `agent` dentro de `map`, com `permission.write` e sandbox derivado.

## Observações relevantes

- `codex` é o provider padrão para nós `agent` quando `provider` não é especificado; `provider: claude` também é suportado para Claude Code.
- `permission.write` é obrigatório quando o bloco `permission` existe no node `agent`.
- O sandbox explícito do node tem prioridade; o fallback via `permission.write` só entra em
  cena quando o campo `sandbox` não foi declarado.
- `CODEX_MODEL` e `CODEX_SANDBOX` funcionam como fallback de ambiente, não como substitutos
  de configuração explícita do workflow.
- `OPENAI_API_KEY` precisa estar disponível no ambiente do processo para o SDK inicializar.
- A resposta só é convertida em JSON quando o `output_schema` foi enviado e o texto final é
  JSON válido; caso contrário, o texto bruto continua disponível em `Text`.
- Os eventos brutos preservam o histórico emitido pelo turn, o que facilita auditoria e debug
  sem depender apenas do texto final.
