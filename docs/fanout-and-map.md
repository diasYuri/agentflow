# Fan-out e mapas

## Objetivo

Esta feature permite expandir um node sobre uma coleção de itens e executar cada item como uma instância independente, sem perder a ordem original dos resultados. Ela também adiciona suporte a `kind: map`, no qual cada item recebe um workflow aninhado com seu próprio plano filho.

Na prática, o runtime passa a oferecer:

- expansão de `for_each` em múltiplas instâncias;
- `kind: map` com `child_plan` próprio por item;
- fan-out local com concorrência configurável;
- limite de segurança com `max_items`;
- cancelamento por `fail_fast`;
- preservação da ordem dos `outputs` e dos caminhos hierárquicos dos resultados.

## Como funciona

A modelagem começa no spec do workflow, em [`internal/core/workflow/spec.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/spec.go). O node pode declarar:

- `for_each`: expressão que resolve a coleção de itens;
- `concurrency`: limite local de paralelismo para as instâncias;
- `max_items`: teto de itens aceito para a expansão;
- `fail_fast`: controle de cancelamento da expansão;
- `kind: map`: modo em que o node contém um workflow aninhado em `nodes`.

O planner, em [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go), já prepara o fluxo aninhado quando encontra um node `map`. Nesse caso, ele monta um `child_plan` com os nodes internos do bloco e mantém o escopo externo visível para validações e referências herdadas.

Na execução, [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go) segue duas trilhas:

1. nodes comuns com `for_each` usam fan-out paralelo para executar uma instância por item;
2. nodes `kind: map` usam fan-out também, mas cada instância cria um estado filho e percorre o `child_plan` completo.

### Expansão de `for_each`

Quando um node tem `for_each`, o runtime resolve a coleção e transforma cada item em uma instância independente.

- Se `max_items` estiver definido e a coleção exceder esse valor, o node falha antes de iniciar a execução.
- Se `concurrency` não estiver definido, o runtime usa o total de itens como limite local.
- Cada instância ainda precisa respeitar a concorrência global do workflow.
- Os resultados são armazenados no mesmo índice de entrada, então a ordem final dos `outputs` fica estável mesmo que a execução termine fora de ordem.

### `kind: map`

O modo `map` é um fan-out com subworkflow.

- O planner exige que o node `map` tenha `nodes` internos.
- Para cada item, o runtime cria um `ExecutionState` filho com `item`, `index` e `total` preenchidos.
- O caminho do resultado passa a ser hierárquico, como `batch/0000/draft`.
- O output final do item vem do último node executado no `child_plan`.

Esse desenho permite aninhar mapas sem perder o contexto do item atual nem o histórico dos resultados intermediários.

### Cancelamento por `fail_fast`

O comportamento de `fail_fast` é resolvido no estado de execução e segue a precedência:

1. override no node;
2. configuração do workflow;
3. default efetivo `true`.

Quando `fail_fast` está ativo, a primeira falha em uma instância cancela as demais pendências do fan-out e propaga o cancelamento para o contexto compartilhado da expansão. Quando está desativado, o runtime tenta processar todas as instâncias previstas.

### Ordem e caminhos

A ordem dos outputs preserva a posição original dos itens no `for_each`, não a ordem de término das goroutines. Isso vale tanto para nodes simples quanto para `map`.

Os resultados também preservam caminhos hierárquicos por instância, o que facilita auditoria e debug nos artefatos do run. O teste em [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go) cobre esse comportamento com exemplos de fan-out e de workflow aninhado.

## Arquivos principais

- [`internal/core/workflow/spec.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/spec.go): define `for_each`, `concurrency`, `max_items`, `fail_fast` e `kind: map` no DSL.
- [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go): monta o `ExecutionPlan` e cria `child_plan` para nodes `map`.
- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): executa o fan-out, aplica concorrência local, cancela por `fail_fast`, preserva ordem e consolida outputs.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobre a ordem dos outputs, o workflow aninhado em `map` e o cancelamento de instâncias.
- [`samples/workflows/review-changed-files.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml): exemplo de revisão paralela com chunking e fan-out de agentes.
- [`samples/workflows/security-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml): exemplo de auditoria paralela por áreas com limite local de concorrência.
- [`samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml): exemplo mais completo, com `kind: map` aninhado iterando sobre specs técnicas e planos de implementação.

## Observações relevantes

- `kind: map` não é apenas um fan-out de um node único; ele executa um workflow filho completo para cada item.
- `max_items` funciona como guardrail para evitar expansões muito grandes por engano.
- O output agregado de um node expandido continua ordenado, mesmo com concorrência local alta.
- Os caminhos hierárquicos ajudam a localizar a origem de cada resultado em runs persistidos.
- As validações de domínio já diferenciam referências de nodes expandidos, então workflows precisam usar `nodes.<id>.outputs` quando o node foi expandido por `for_each`.
