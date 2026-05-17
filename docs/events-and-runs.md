# Eventos e persistência

## Objetivo

Esta feature centraliza a observabilidade e a persistência de cada execução local do `agentflow`.
Ela cobre dois fluxos complementares:

1. streaming de eventos de `run` e `node` para `stdout`, em texto ou JSON;
2. gravação dos dados completos do run no diretório local correspondente.

Na prática, isso permite acompanhar a execução em tempo real e, ao mesmo tempo, manter um
artefato persistente para auditoria, debug e testes automatizados.

## Como funciona

Os eventos do runtime usam o tipo [`run.Event`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/types.go),
que carrega `ts`, `run_id`, `type`, `node_id`, `instance_id`, `path`, `attempt` e `data`.
O executor emite esses eventos ao longo da execução e fecha os sinks no final do run.

### Sinks de eventos

O pacote de eventos é dividido em quatro adaptações:

- [`internal/adapters/events/stdout/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/stdout/sink.go):
  escreve os eventos em `stdout`. Quando o formato é `json`, serializa o evento inteiro em uma
  linha JSON. No formato texto, imprime timestamp, nó e tipo do evento. Quando o `data.warning`
  existe, a saída textual inclui o aviso na mesma linha.
- [`internal/adapters/events/jsonl/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/jsonl/sink.go):
  persiste eventos em JSONL, abrindo o arquivo sob demanda e anexando um objeto JSON por linha.
- [`internal/adapters/events/memory/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/memory/sink.go):
  guarda todos os eventos em memória, o que facilita testes e validação de sequência/estrutura.
- [`internal/adapters/events/multi/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/multi/sink.go):
  combina múltiplos sinks no mesmo fluxo. Cada evento é enviado para todos os destinos e o `Close`
  tenta encerrar todos, retornando o primeiro erro encontrado.

O CLI monta a cadeia com `multi.New(...)`, combinando o sink de JSONL com o sink de `stdout`.
Quando `--events-jsonl` é fornecido, o arquivo é aberto no diretório do run; caso contrário,
o sink JSONL permanece inerte.

No início da execução, [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go)
cria o run, persiste o workflow e, se o sink suportar `Open(string) error`, inicializa
`events.jsonl` dentro do diretório do run. Em seguida, o executor emite eventos como:

- `run.created`
- `run.started`
- `node.ready`
- `node.skipped`
- `run.pausing`
- `run.paused`
- `run.resumed`
- `worktree.resume_drift_detected`
- `run.completed`
- `run.failed`

Os eventos de pausa/retomada carregam `reason` em `data` (`manual` ou `pause_when_fail`) e o
node ID em `node_id` quando a pausa ocorreu depois da falha de um node específico.

O estado do runtime em [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go)
é responsável por:

- registrar o resultado de cada nó no estado corrente;
- gravar o resultado mascarado no repositório local;
- emitir o evento final de sucesso ou falha;
- fechar o sink de eventos ao terminar a execução.

### Persistência do run

O repositório local em [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go)
mantém um diretório por `run_id` e grava os principais artefatos:

- `run.json`: metadados do run;
- `workflow.yaml`: fonte do workflow, quando disponível;
- `normalized.json`: workflow normalizado;
- `plan.json`: plano de execução;
- `nodes/**/result.json`: resultado persistido por nó;
- `nodes/**/stdout.txt` e `nodes/**/stderr.txt`: saída bruta de nós que produzem texto;
- `summary.json`: resumo final do run;
- `events.jsonl`: eventos emitidos durante a execução;
- `checkpoint.json`: checkpoint atual do run usado para pause/resume. É gravado de forma atômica em cada avanço relevante (antes de iniciar o próximo node, depois de gravar resultado, durante delay de retry) e removido em `success`, `failed` definitivo ou `cancelled`.

O caminho base padrão fica em `.agentflow/runs/` no projeto atual, com fallback para
`~/.agentflow/runs/` quando o diretório do usuário estiver disponível.

## Arquivos envolvidos

- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): cria o run, abre o `events.jsonl` e emite os eventos de ciclo de vida.
- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): persiste resultados de nós e finaliza o resumo do run.
- [`internal/adapters/events/stdout/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/stdout/sink.go): sink para saída em texto ou JSON no terminal.
- [`internal/adapters/events/jsonl/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/jsonl/sink.go): sink persistente em JSONL.
- [`internal/adapters/events/memory/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/memory/sink.go): sink em memória usado em testes.
- [`internal/adapters/events/multi/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/multi/sink.go): composição de múltiplos sinks.
- [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go): armazenamento local do run e de seus artefatos.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobertura de persistência completa, incluindo `events.jsonl` e os arquivos do run.

## Observações relevantes

- O sink de `stdout` é voltado a observabilidade humana; o formato `json` facilita integração com ferramentas externas.
- O sink `multi` não faz fan-out assíncrono; ele escreve em sequência, então uma falha em um destino interrompe o envio do evento para os próximos sinks.
- O sink em memória é útil para afirmar ordem, conteúdo e contagem de eventos sem depender do filesystem.
- O runtime grava resultados mascarados quando secrets estão presentes, incluindo `summary.json`, `stdout.txt` e `stderr.txt`.
- A suíte de testes valida que o diretório do run contém `run.json`, `workflow.yaml`, `normalized.json`, `plan.json`, `summary.json` e `events.jsonl`, além dos arquivos por nó.
