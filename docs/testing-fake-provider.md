# Fake Agent Provider

## Objetivo

O provider `fake` permite executar nós `kind: agent` usando respostas predefinidas em
memória, sem depender de LLMs reais como Codex, Claude ou Pi. Ele é útil para:

- testes de regressão de workflows que contêm nós `agent`;
- validação de fluxos de execução em ambientes sem acesso a APIs locais ou remotas;
- execução determinística de pipelines onde a resposta do agente deve ser controlada.

Ao contrário dos providers de produção, o `fake` não realiza chamadas de rede nem
executa binários externos. Todas as respostas são resolvidas localmente a partir de um
mapa configurado em memória.

## Como funciona

O provider implementa a mesma interface [`AgentProvider`](internal/core/ports/agent.go)
que os adapters de produção. Em
[`internal/adapters/agent/fake/provider.go`](internal/adapters/agent/fake/provider.go),
o fluxo de execução é simples:

1. Recebe um [`AgentRequest`](internal/core/ports/agent.go) contendo `NodeID`, `Prompt`
   e outros metadados do runtime.
2. Consulta o mapa interno `Responses` usando `req.NodeID` como chave.
3. Se houver uma resposta previamente registrada para aquele nó, retorna-a
   imediatamente.
4. Caso contrário, retorna um fallback:
   - o texto do `Prompt`, quando presente;
   - ou a string `fake response for <node_id>` quando o prompt está vazio.

Exemplo de uso em testes:

```go
import (
    agentfake "github.com/diasYuri/agentflow/internal/adapters/agent/fake"
    "github.com/diasYuri/agentflow/internal/core/ports"
)

fakeProvider := agentfake.New()
fakeProvider.Responses["review"] = ports.AgentResult{
    Text: `{"status":"ok","issues":[]}`,
}

registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
    "codex": fakeProvider,
})
```

Nesse exemplo, qualquer nó `agent` que use `provider: codex` (ou deixe o provider
implícito, já que `codex` é o padrão) e tenha `id: review` receberá a resposta
registrada. Os demais nós recebem o fallback.

O registry estático usado para injetar o provider está em
[`internal/core/ports/registry.go`](internal/core/ports/registry.go). Ele aceita um
mapa de providers por nome e expõe os métodos `Get` e `HasProvider`, consumidos pelo
dispatcher do runtime.

## Arquivos principais

- [`internal/adapters/agent/fake/provider.go`](internal/adapters/agent/fake/provider.go):
  implementação do provider `fake`, com o mapa `Responses` e a lógica de fallback por
  `NodeID`.
- [`internal/core/ports/registry.go`](internal/core/ports/registry.go):
  `StaticAgentProviderRegistry`, que permite injetar o provider fake (ou qualquer outro)
  em testes e ambientes controlados.
- [`internal/core/ports/agent.go`](internal/core/ports/agent.go):
  contrato `AgentProvider`, estruturas `AgentRequest` e `AgentResult` compartilhados por
  todos os adapters de agente.
- [`internal/app/runtime.go`](internal/app/runtime.go):
  ponto de montagem do registry em produção, onde `codex`, `claude` e `pi` são
  registrados. O provider `fake` não aparece aqui por padrão.
- [`internal/core/runtime/run_workflow_test.go`](internal/core/runtime/run_workflow_test.go):
  uso concreto do provider `fake` na suíte de testes do runtime, substituindo o adapter
  real de `codex` para isolar a execução do workflow.

## Observações relevantes

- O provider `fake` **não está registrado no runtime de produção** por padrão. Ele é
  injetado manualmente em testes via `StaticAgentProviderRegistry`.
- As respostas são indexadas por `NodeID`, não por `Prompt`. Isso significa que dois nós
  com o mesmo `id` receberão a mesma resposta predefinida, mesmo que os prompts sejam
  diferentes.
- O provider não preenche `Usage`, `RawEvents` nem `JSON` automaticamente; esses campos
  devem ser informados explicitamente no `AgentResult` registrado, quando necessários.
- Como não há chamadas externas, a execução é instantânea e não depende de variáveis de
  ambiente como `OPENAI_API_KEY`, `ANTHROPIC_API_KEY` ou binários locais.
- Em testes, é comum registrar o provider `fake` sob o nome `"codex"` para aproveitar o
  fallback padrão do runtime quando o campo `provider` é omitido no workflow.
