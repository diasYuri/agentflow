# Fan-out e mapas aninhados

## Objetivo

Esta feature permite expandir listas em execuções paralelas e montar subfluxos aninhados por item.
O objetivo é cobrir dois cenários complementares:

- `for_each` para espalhar um node comum ou um agente em várias instâncias;
- `kind: map` para executar um workflow interno por item, mantendo o escopo externo visível.

O comportamento é pensado para workflows que precisam paralelizar trabalho sem perder rastreabilidade,
ordem de saída ou persistência previsível dos resultados.

## Como funciona

### Expansão por `for_each`

Quando um node declara `for_each`, o runtime avalia a expressão e converte o resultado em uma lista.
Cada item vira uma instância independente do mesmo node, com contexto próprio de execução.

Esse mecanismo vale para nodes comuns e para agentes, como `bash`, `transform`, `noop` e `agent`.
O node expandido recebe:

- `item`: o valor atual da iteração;
- `index`: a posição original do item;
- `total`: a quantidade total de itens.

O runtime usa um semáforo local para respeitar `concurrency` por expansão. Se o valor não for
informado, a expansão usa a quantidade de itens como limite local.

### Mapas aninhados

Um node com `kind: map` representa um subworkflow. Em vez de executar uma única ação, ele monta um
plano interno com os nodes filhos e executa esse plano uma vez por item de `for_each`.

O escopo externo continua acessível dentro do `map`. Isso permite referências como:

- `${nodes.outer.output}`
- `${inputs.*}`
- `${vars.*}`
- `${secrets.*}`

Os nodes internos também podem usar `for_each` e até declarar novos `map`, formando uma hierarquia
de execução.

### Limites e falhas

Os controles `concurrency`, `max_items` e `fail_fast` são aplicados por expansão:

- `concurrency` limita quantas instâncias rodam ao mesmo tempo naquele fan-out;
- `max_items` falha antes de iniciar a expansão se a lista ultrapassar o limite;
- `fail_fast` interrompe instâncias pendentes quando ocorre a primeira falha, salvo override local.

Se `fail_fast` não for definido no node, o runtime usa `execution.fail_fast`. Se ambos estiverem
ausentes, o comportamento padrão é `true`.

### Ordem e persistência

A agregação de saídas preserva a ordem original dos itens, mesmo quando as instâncias terminam fora
de ordem por causa da paralelização.

Cada instância recebe um `instance_id` determinístico no formato `0000`, `0001`, `0002` e assim por
diante. A persistência usa caminhos hierárquicos, o que torna os artefatos estáveis e fáceis de
inspecionar:

```text
nodes/review_chunk/0000/result.json
nodes/per_technical_spec/0000/break_plans/result.json
nodes/per_technical_spec/0001/per_plan/0003/implement_plan/result.json
```

Esse formato é importante para debug, replay manual e leitura de runs complexos com múltiplos níveis
de fan-out.

## Arquivos principais

- [`/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/spec.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/spec.go): define `NodeSpec` com `ForEach`, `Concurrency`, `MaxItems`, `FailFast` e `Nodes`.
- [`/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go): valida `kind: map`, regras de escopo e referências estáticas entre nodes.
- [`/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): coordena a expansão, executa `map`, agrega outputs e emite eventos por instância.
- [`/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): resolve os itens de `for_each` e concentra helpers usados pela execução.
- [`/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): carrega o estado de execução, herança de escopo, caminho hierárquico e fallback de `fail_fast`.
- [`/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobre ordem dos outputs, execução de `map` aninhado, persistência e cancelamento por `fail_fast`.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml): exemplo de `for_each` aplicado a agentes para revisar chunks em paralelo.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml): exemplo de `map` aninhado com fan-out em múltiplos níveis.

## Observações relevantes

- `kind: map` exige nodes filhos; um mapa vazio é inválido.
- A validação permite que nodes internos enxerguem o escopo externo, mas bloqueia referências desconhecidas.
- `for_each` falha se a expressão não resultar em array ou slice.
- O limite global `execution.max_concurrency` continua valendo junto com a concorrência local de cada expansão.
- Os paths persistidos e os `instance_id`s são determinísticos dentro de um mesmo fan-out, o que ajuda a comparar runs e localizar artefatos com precisão.
