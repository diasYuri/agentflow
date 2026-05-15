# Planejamento e grafo

## Objetivo

Esta feature transforma o spec de workflow em um plano executável e em uma visão gráfica Mermaid do grafo de execução. O objetivo é deixar explícitas as dependências entre nós, preservar a ordem declarada quando houver empate e expor ciclos, dependências desconhecidas e saltos condicionais antes da execução.

Em termos práticos, o planejador passa a:

- construir a ordem topológica a partir de `depends_on`;
- manter estabilidade pela ordem original dos nós declarados;
- detectar ciclos com mensagem descritiva;
- rejeitar dependências para nós inexistentes;
- incluir `child_plan` para nós `map` aninhados;
- registrar `go_to_if` como arestas de salto;
- gerar saída Mermaid com arestas sólidas, tracejadas e nós isolados.

## Como funciona

O planejamento é concentrado em [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go).
O ponto de entrada é `BuildPlan`, que recebe o `WorkflowSpec` e retorna um `ExecutionPlan` com:

- `Workflow`: o spec original;
- `Nodes`: mapa de nós planejados com dependências, dependentes, índice original e `child_plan` quando aplicável;
- `Edges`: arestas sólidas vindas de `depends_on`;
- `Jumps`: arestas tracejadas vindas de `go_to_if`;
- `Order`: ordem topológica final.

### Ordem topológica estável

O planner indexa os nós na ordem em que aparecem no YAML e usa esse índice como critério de desempate durante a ordenação topológica. Na prática, isso significa que:

1. dependências válidas vêm antes de seus dependentes;
2. quando dois nós têm o mesmo nível de precedência, a ordem declarada é preservada.

### Detecção de erros estruturais

Ainda em `BuildPlan`, o domínio valida a consistência do grafo:

- se um `depends_on` aponta para um nó que não existe, o planner retorna erro;
- se o grafo contém ciclo, o erro inclui o caminho do ciclo detectado;
- se um `go_to_if.target` aponta para um nó desconhecido, o planner retorna erro;
- se `go_to_if.target` aponta para um nó futuro, o planner rejeita a definição, porque o salto precisa mirar o nó atual ou um anterior.

### Nós `map` aninhados

Quando um nó possui `nodes`, o planner chama recursivamente `buildPlan` para montar um `child_plan`.
Esse plano filho reutiliza o mesmo `WorkflowSpec` base, mas trabalha apenas sobre os nós internos do `map`, mantendo o escopo visível necessário para validar dependências internas e referências herdadas.

### Grafo Mermaid

A renderização do grafo fica em [`internal/core/workflow/graph.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/graph.go), por meio de `WriteMermaidGraph`.

A saída segue estas regras:

- `Edges` viram arestas sólidas (`-->`);
- `Jumps` viram arestas tracejadas (`-.->`);
- nós sem dependências e sem dependentes aparecem isolados, para não sumirem na visualização.

O comando `graph` em [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go) valida o workflow e imprime esse Mermaid no `stdout`. No estado atual, ele aceita apenas `--format mermaid`.

## Arquivos principais

- [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go): monta o `ExecutionPlan`, resolve `depends_on`, cria `child_plan`, ordena topologicamente e valida `go_to_if`.
- [`internal/core/workflow/graph.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/graph.go): serializa o plano para Mermaid.
- [`internal/core/workflow/plan_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan_test.go): cobre ordem topológica, child plan para `map` e salto condicional.
- [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go): expõe o comando `graph` e reaproveita a validação antes da renderização.
- [`internal/cli/root_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root_test.go): garante a saída Mermaid do CLI e a integração visível do comando.

## Observações relevantes

- O planner é parte da validação de domínio: `Validate` chama `BuildPlan` para impedir que workflows inválidos avancem.
- A estabilidade da ordem não vem do grafo em si, mas do índice original dos nós no spec.
- `go_to_if` não entra na ordem topológica como dependência normal; ele é tratado como salto visual e semântico separado.
- Nós isolados continuam aparecendo no Mermaid, o que ajuda a identificar etapas soltas ou ainda não conectadas.
- O comportamento do grafo é exercitado por testes de domínio e por teste de CLI, reduzindo o risco de divergência entre o plano interno e a saída textual.
