# Plano DAG e gráfico

## Objetivo

Esta feature transforma o workflow em um plano de execução em formato DAG, com ordenação
topológica baseada em `depends_on`, detecção de ciclos e representação explícita dos saltos
condicionais definidos por `go_to_if`.

Além do plano, a mesma camada exporta o grafo em Mermaid para inspeção visual, incluindo nós
isolados que não participam de nenhuma aresta.

## Como funciona

O fluxo principal acontece no pacote [`internal/core/workflow`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow):

1. [`Validate`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go) chama
   [`BuildPlan`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go) como última
   etapa da validação estrutural.
2. `BuildPlan` monta um [`ExecutionPlan`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go)
   com:
   - `Nodes`: estado planejado de cada node;
   - `Edges`: dependências normais originadas de `depends_on`;
   - `Jumps`: arestas de salto originadas de `go_to_if`;
   - `Order`: sequência topológica final do workflow.
3. A ordenação topológica percorre os nodes em ordem estável de declaração e visita primeiro as
   dependências válidas dentro do escopo atual.
4. Se o algoritmo encontra um node já em processamento, ele reporta ciclo com o caminho completo,
   por exemplo `a -> b -> c -> a`.
5. Depois da ordenação, `conditionalJumps` valida `go_to_if.target`:
   - o alvo precisa existir;
   - o alvo precisa apontar para o node atual ou para um node anterior na ordem;
   - o salto é registrado separadamente como edge pontilhada.
6. [`WriteMermaidGraph`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/graph.go)
   imprime o grafo em `graph TD`:
   - dependências normais usam `-->`;
   - jumps usam `-.->`;
   - nodes sem dependências e sem dependentes são emitidos isoladamente.

### Ordem e escopo

O planner usa a posição original dos nodes como critério de desempate. Isso mantém a saída
determinística mesmo quando vários nodes têm o mesmo conjunto de dependências.

Quando o workflow possui nodes aninhados em `map`, o planner reaproveita a mesma lógica de
ordenação para o subgrafo interno, preservando o escopo do node pai.

### Erros de planejamento

O plano falha antes da execução quando encontra:

- dependência para node inexistente;
- ciclo em `depends_on`;
- referência inválida em `go_to_if.target`;
- salto condicional para node futuro.

## Arquivos principais envolvidos

- [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go): monta o `ExecutionPlan`, gera a ordem topológica, detecta ciclos e valida `go_to_if`.
- [`internal/core/workflow/graph.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/graph.go): serializa o plano para Mermaid, incluindo edges normais, jumps e nodes isolados.
- [`internal/core/workflow/plan_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan_test.go): cobre a ordenação topológica, o plano de nodes aninhados e a inclusão de `go_to_if` como jump edge.
- [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go): expõe o comando `graph`, que valida o workflow e imprime o grafo Mermaid.

## Observações relevantes

- O grafo Mermaid é gerado a partir do plano validado, não diretamente do YAML bruto.
- `go_to_if` não entra na ordenação topológica; ele é tratado como salto condicional separado da
  cadeia de dependências.
- O formato exportado pelo CLI é `graph TD`; qualquer outro formato não é aceito.
- Nós sem conexões continuam visíveis no diagrama, o que ajuda a identificar etapas soltas ou
  blocos ainda não ligados ao fluxo principal.
