# Execução do runtime

## Objetivo

Esta feature concentra a orquestração da execução de um workflow já validado e planejado.
Ela resolve inputs, aplica overrides de `vars` e `max_concurrency`, carrega secrets do ambiente
e prepara o estado interno usado durante o run.

Na fase de execução, o runtime respeita dependências entre nós, `when`, retries, timeouts,
`continue_on_error`, `fail_fast` e `execution.pause_when_fail`. Ao final, persiste o status por
nó e por run, junto com contadores de chamadas a agentes, bash e retries.

O mesmo fluxo também suporta `dry-run`: nesse modo, o runtime resolve inputs e monta o plano,
mas não dispara a execução dos nós.

## Como funciona

A entrada principal é [`internal/core/runtime/run_workflow.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow.go),
que implementa o use case `RunWorkflowUseCase`.

O fluxo segue esta sequência:

1. carrega o workflow a partir do repositório configurado;
2. resolve os inputs fornecidos e valida valores obrigatórios;
3. aplica overrides de `vars` e `max_concurrency` no spec;
4. valida novamente o workflow já com os overrides aplicados;
5. monta o `ExecutionPlan`;
6. quando não é `dry-run`, delega a execução para os handlers do runtime.

Na execução propriamente dita, [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go)
coordena a criação do run, grava o workflow e inicializa o estado de execução. Em seguida:

- carrega secrets declarados no workflow a partir do ambiente;
- cria o `ExecutionState` com inputs, vars, secrets, nodes já processados e métricas;
- define o limite global de concorrência com base em `execution.max_concurrency`;
- percorre os nós na ordem planejada, respeitando dependências e saltos condicionais;
- avalia `when` antes de executar cada nó;
- aplica retries e timeout por nó;
- propaga `continue_on_error` e `fail_fast` para decidir quando parar ou continuar;
- grava resultados e eventos enquanto o run avança;
- grava `checkpoint.json` em pontos seguros: antes do próximo nó, depois de resultados e durante atrasos de retry;
- finaliza o run com um resumo mascarado, evitando vazar secrets.

O estado do run fica centralizado em [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go).
Esse estado mantém:

- `inputs`, `vars` e `secrets` disponíveis para templates e expressões;
- resultados por nó, usados para compor o contexto de avaliação;
- o encadeamento de estados em nós expandidos ou aninhados;
- contadores de `agentCalls`, `bashCalls` e `retries`;
- o `failFast` efetivo e o conjunto de nós já concluídos;
- quando `worktree.enabled` é true, mantém o caminho do worktree, commit base, provider e diretório de destino.

Quando `execution.pause_when_fail` está habilitado, uma falha final de node sem
`continue_on_error` emite `run.pausing`, salva um checkpoint com `reason: "pause_when_fail"` e
termina o run como `paused`. O checkpoint aponta para o node falho em `retry_node_id`, então a
retomada reexecuta esse node e preserva os resultados anteriores no contexto `${nodes...}`. Em
sucesso, falha definitiva ou cancelamento, o checkpoint é removido.

A retomada é um contrato do runtime via `RunOptions.ResumeRunID`/`ExecutionRequest.ResumeRunID`.
Ela recarrega `checkpoint.json`, reconstrói o plano a partir do workflow normalizado persistido,
restaura métricas e resultados, emite `run.resumed` e continua a partir do cursor salvo. A
retomada granular dentro de instâncias paralelas de `for_each` ou `map` ainda não é suportada; o
ponto seguro desta versão é o node expandido como unidade.

O checkpoint persistido usa os mesmos resultados mascarados do restante da persistência local.
Assim, secrets não são gravados em claro; se um node posterior dependia exatamente do valor secreto
emitido por um node anterior, a retomada verá o valor mascarado.

### Ciclo de worktree: merge, conflitos e cleanup

Quando o workflow tem `worktree.enabled: true`, ao final de um run bem-sucedido o runtime
executa um ciclo determinístico de merge antes de finalizar:

1. **Status** — `provider.Status` verifica se o worktree tem mudanças. Se estiver clean,
   registra `merge_status: no_changes`, salva `worktree/status.json` e pula o apply.
2. **Diff** — `provider.Diff` gera o diff e a lista de arquivos alterados. O diff é salvo em
   `worktree/diff.patch` e a lista ordenada de arquivos (adicionados, modificados, removidos,
   renomeados) vai para os metadados.
3. **Apply** — `provider.Apply` aplica o diff no diretório de destino, validando que o
   `HEAD` do destino ainda corresponde ao `base_commit`. Se tiver sucesso, registra
   `merge_status: merged`, salva `worktree/merge.log` e emite `worktree.merged`.
4. **Conflitos** — se o apply retornar erro classificado como `ErrWorktreeResolvable`, o runtime
   registra `merge_status: conflict`, salva `worktree/conflicts.json` e, quando
   `worktree.merge.on_conflict` é `"agent"`, solicita um agente de resolução. O agente recebe
   um prompt com workflow, base commit, arquivos alterados, conflitos e logs. Após a resolução,
   o runtime reavalia status e reexecuta apply. Se o agente resolver e a validação subsequente
   passar, o status vira `merged`; se continuar conflitando, permanece `conflict`.
5. **Erros estruturais** — erros classificados como `ErrWorktreeStructural` (Git ausente,
   destino não é repo, HEAD mudou, permissão negada) registram `merge_status: failed` e
   **não** disparam agente.
6. **Cleanup** — após o merge/cleanup, `provider.Cleanup` é chamado conforme política:
   - sucesso com `cleanup.on_success: true` (default): remove o worktree;
   - `no_changes` com `on_success: true`: remove se a política permitir;
   - conflito, falha de merge ou `on_success: false`: preserva o worktree;
   - falha do run com `cleanup.on_failure: "cleanup"`: remove; caso contrário, preserva.

O evento `worktree.resolution_agent.requested` deixa claro que a resolução de conflitos é uma
etapa de runtime fora do grafo de nodes. O `FinalizeRun` só é chamado após o ciclo completo
de merge e cleanup, garantindo que o `summary.json` reflita o estado auditável final.

Os helpers de [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go)
definem as regras de apoio usadas pela execução:

- `ResolveInputs` combina defaults, required inputs e valores recebidos;
- `ApplyWorkflowOverrides` injeta `vars` e ajusta `max_concurrency`;
- `loadSecrets` lê secrets do ambiente;
- `effectiveRetries`, `effectiveTimeout` e `effectiveWorkingDir` calculam os valores finais;
- `effectiveShell`, `effectiveAgentSandbox` e `resolvePath` normalizam a execução local.

O dispatch de nós acontece no pacote de handlers e separa o comportamento por tipo:

- `noop` retorna sucesso sem efeito colateral;
- `transform` avalia templates e aplica transforms do domínio;
- `bash` renderiza o comando, executa o shell e registra stdout, stderr e exit code;
- `agent` resolve prompt, provider, sandbox e repassa a chamada ao provider configurado.

O modo `dry-run` reaproveita a mesma preparação do workflow. Ele resolve inputs e gera o plano,
mas não chama `handlers.Execute`, então nenhum nó é disparado.

## Arquivos envolvidos

- [`internal/core/runtime/run_workflow.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow.go): use case principal, preparação do workflow, `DryRun` e `Run`.
- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): orquestração da execução, fan-out, retries, timeouts, eventos e finalização do run.
- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): estado compartilhado do run, métricas e composição do resumo final.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): resolução de inputs, overrides, secrets e helpers de configuração efetiva.
- [`internal/core/runtime/handlers/worktree.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/worktree.go): ciclo de merge determinístico, detecção de conflitos, agente de resolução e cleanup de worktree.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobertura de fan-out, `when`, `continue_on_error`, `fail_fast`, timeouts, retries, secrets, working dir, persistência e ciclo de worktree.

## Observações relevantes

- `continue_on_error` permite que o run siga adiante mesmo depois de falhas de nó, enquanto o default do workflow continua sendo encerrar a execução em falha.
- `fail_fast` controla o cancelamento de expansões e fan-outs; quando desabilitado, o runtime tenta processar todas as instâncias previstas.
- `pause_when_fail` pausa somente depois que os retries do node se esgotam; nodes com `continue_on_error` continuam usando a semântica normal de prosseguir.
- Retries são contabilizados separadamente das tentativas totais do nó, e cada retry emite evento próprio.
- Timeouts por nó são aplicados via `context.WithTimeout`, e a falha resultante é marcada como `timeout`.
- Secrets são carregados do ambiente apenas quando a variável configurada existe; o resumo e os artefatos persistidos passam por mascaramento.
- O resumo final inclui status do run, métricas agregadas e o mapa de resultados por nó, o que facilita auditoria e debug.
- `dry-run` resolve a mesma preparação usada no run real, então é útil para validar efeitos de overrides e entradas sem executar comandos locais ou chamar agentes.
- O ciclo de worktree ocorre **após** a execução dos nodes, então falhas de merge ou conflitos não alteram o `run_status` do workflow; eles são registrados em `worktree/status.json`.
- O agente de resolução de conflitos é uma etapa de runtime, não um node do grafo. Isso garante que métricas e eventos não confundam a execução normal com a resolução de merge.
