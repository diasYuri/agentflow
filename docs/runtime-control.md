# Controle de execução

## Objetivo

Esta feature concentra a orquestração do runtime que executa um workflow já planejado.
Ela garante que os nós sejam processados na ordem definida pelo plano, respeitando
condições, retries, timeout e o limite global de concorrência, enquanto consolida o
estado final do run com eventos de ciclo de vida e resumo persistido.

Em termos práticos, o runtime:

- percorre o `ExecutionPlan` na ordem calculada pelo planner;
- avalia `when` antes de executar cada nó;
- aplica `continue_on_error` para decidir se a execução deve seguir após falhas;
- executa retries com atraso incremental;
- encerra execuções por timeout;
- limita a concorrência global com `max_concurrency`;
- marca nós como `skipped`, `failed`, `timeout` ou `cancelled` conforme o resultado;
- emite eventos que permitem acompanhar o ciclo de vida do run e de cada nó;
- consolida o estado final em `summary.json` e nos artefatos do run.

## Como funciona

O fluxo começa em [`internal/core/runtime/run_workflow.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow.go).
O use case carrega o workflow, resolve inputs, aplica overrides, valida o spec e monta o plano.
Se a execução for real, ele delega para [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go).

### Inicialização do run

- O executor cria um `run_id` estável a partir da data/hora atual e do nome do workflow.
- O run é persistido no repositório local antes de qualquer nó ser executado.
- O workflow e o plano são salvos em disco para auditoria e reprodução.
- O sink de eventos é aberto em `events.jsonl` quando o adaptador suporta essa operação.
- As `secrets` declaradas no workflow são carregadas do ambiente e passam a ser mascaradas nos eventos e no resumo.
- O estado inicial do run é criado em [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go), junto com métricas e mapas de nós/resultados.

### Ordenação e controle do loop

- `executeNodes` percorre `plan.Order` de forma sequencial.
- Antes de executar um nó, o runtime verifica se o run já falhou e se o nó permite continuar com `continue_on_error`.
- Dependências ausentes fazem o nó ser marcado como `skipped`.
- A expressão `when` é avaliada no contexto atual do run; se retornar `false`, o nó é `skipped` e o evento `node.skipped` é emitido.
- Se `when` falhar ao avaliar, o nó é marcado como `failed`; sem `continue_on_error`, o erro interrompe o run.
- Quando a condição passa, o runtime emite `node.ready` e chama a execução efetiva do nó.
- `go_to_if`, quando presente, pode alterar o próximo índice do loop após a conclusão do nó.

### Retries, timeout e concorrência

- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go) calcula os valores efetivos de `retries`, `timeout`, `working_dir`, `shell` e outras opções herdadas do workflow.
- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go) implementa `executeSingle`, que tenta o nó mais de uma vez quando `retries` ou o valor padrão do workflow pedem isso.
- Em cada nova tentativa, o runtime emite `node.retrying` com o histórico da tentativa anterior e aplica um atraso incremental de `250ms` por retry.
- O timeout efetivo é aplicado por tentativa com `context.WithTimeout`.
- O `max_concurrency` global é convertido em um semáforo compartilhado, e cada tentativa precisa adquirir uma permissão antes de rodar.
- Se o contexto expira durante a tentativa, o nó termina como `timeout`.
- Se a concorrência global não puder ser adquirida a tempo, o nó termina como `cancelled`.

### Execução por tipo de nó

- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go) roteia a execução para `noop`, `transform`, `bash` e `agent`.
- `noop` retorna sucesso com um payload trivial.
- `transform` renderiza o input e aplica a operação configurada.
- `bash` renderiza o comando, resolve diretório de trabalho, emite um aviso de segurança e executa o shell.
- `agent` renderiza o prompt, resolve o provider e executa a chamada do agente com sandbox e schema de saída quando aplicável.

### Fan-out e escopo interno

- Quando o nó usa `for_each`, o executor expande o item atual em instâncias independentes.
- Cada instância respeita a concorrência local do nó e continua sujeita ao limite global do run.
- O estado de instâncias é consolidado no nó pai em `outputs`.
- Em nós `map`, o runtime também executa o plano aninhado e reaproveita o mesmo mecanismo de estado e eventos.

### Consolidação final

- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go) registra o resultado de cada nó em memória e persiste o resultado mascarado no repositório de runs.
- O helper `isFailure` considera `failed`, `timeout` e `cancelled` como falhas para fins de propagação.
- O finalizador monta um `Summary` com contagem de chamadas, retries, nós falhos e o mapa final de resultados.
- O evento final do run é `run.completed` em sucesso, `run.failed` quando houve erro e `run.paused` quando o runtime pausa por `pause_when_fail` ou solicitação manual.
- O sink de eventos é encerrado ao final da execução, e o resumo final é persistido no run.

### Pause e resume

- `execution.pause_when_fail` pausa o run quando os retries do node falham em sequência e o node não tem `continue_on_error`. O cursor, o `retry_node_id`, os resultados mascarados e as métricas são gravados em `checkpoint.json`.
- A pausa manual via daemon usa um `PauseController` propagado pelo `RunOptions`. O runtime checa a solicitação em pontos seguros (antes de iniciar o próximo node, depois de gravar resultado, durante delay de retry); um node já em execução conclui antes da pausa.
- O resume reabre o checkpoint e cria um novo `WorkflowRunService` reaproveitando o mesmo `run_id`. Nodes anteriores não são re-executados; em pausa por falha, o node em `retry_node_id` é re-executado e o run continua a partir do seguinte.
- O checkpoint é removido em sucesso, falha definitiva ou cancelamento. Pause não limpa o checkpoint, então o resume funciona mesmo depois de restart do daemon.

### Fila do daemon e estado `queued`

- O daemon mantém uma fila interna de runs no estado `queued`.
- Quando um workflow é submetido via `StartWorkflow`, o run é criado como `queued`, adicionado a fila e persistido no store.
- A fila é ordenada por `priority` (maior primeiro) e desempatada por `queued_at`/`started_at` (mais antigo primeiro).
- Um run e promovido de `queued` para `running` quando houver vaga de acordo com o limite configuravel `MaxConcurrentRuns`.
- O valor padrao de `MaxConcurrentRuns` e `3`; quando a configuracao do daemon e deixada no default, a fila eh promovida automaticamente ate tres runs simultaneos.
- Cancelar um run `queued` remove-o da fila e marca como `cancelled` sem criar um service.
- O estado `queued` e persistido no SQLite, permitindo retomada correta apos restart do daemon.

### Politica de restart do daemon

- Ao subir, o daemon recarrega do store todos os runs operacionais.
- Runs que estavam `running` no crash sao marcados como `failed` com `FailureReason = "daemon_restarted"`.
- Runs que estavam `queued` sao re-enfileirados automaticamente e, se houver capacidade, sao promovidos imediatamente para `running`.
- Runs `paused` e `wait_approval` preservam seu estado e podem ser retomados via resume/approve.
- Estados terminais (`success`, `failed`, `cancelled`) sao carregados como estao.

## Arquivos principais

- [`internal/core/runtime/run_workflow.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow.go): ponto de entrada do use case de runtime, com validação, dry-run e execução real.
- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): loop principal do runtime, retries, timeout, fan-out, eventos e consolidação de resultados.
- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): estado compartilhado do run, métricas, persistência de resultados e fechamento do run.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): resolução de inputs, overrides, caminhos, defaults efetivos e utilitários de evento/erro.
- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go): despacho por tipo de nó para `noop`, `transform`, `bash` e `agent`.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobertura dos principais cenários de execução, incluindo `when`, `continue_on_error`, `fail_fast`, retries, timeout, persistência e mascaramento.

## Observações relevantes

- `continue_on_error` controla se a falha de um nó interrompe o restante do plano ou apenas marca aquele nó como falho.
- `when` que avalia para `false` não é tratado como erro; o nó é simplesmente `skipped`.
- `timeout` e `cancelled` aparecem no resultado do nó, mas ambos contam como falha para o resumo e para a propagação de erro.
- O limite global de concorrência é aplicado a todas as execuções de nós, inclusive retries e instâncias expandidas.
- O runtime mascara segredos tanto nos eventos quanto nos arquivos persistidos do run.
- Os testes cobrem a ordem de execução, cancelamento de instâncias em fan-out, retries, timeout, persistência e proteção de segredos, o que serve como documentação executável do comportamento esperado.
