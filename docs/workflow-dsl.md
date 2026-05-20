# Esquema de workflow

## Objetivo

Esta feature define o esquema YAML do workflow e a validação estrutural aplicada antes da execução.
O objetivo é impedir que definições inconsistentes cheguem ao planner ou ao runtime, deixando as
regras do domínio explícitas no próprio spec.

Em termos práticos, o workflow precisa:

- declarar `version: "1"`;
- informar `name`;
- conter ao menos um node;
- usar tipos de input suportados;
- manter dependências, referências e saltos condicionais consistentes;
- respeitar o escopo de nós aninhados em `map`.

### Execução

O bloco `execution` aceita controles globais do runtime:

- `max_concurrency`: limite global de nós em execução simultânea;
- `fail_fast`: padrão usado por expansões `for_each` e `map` quando o node não define override;
- `pause_when_fail`: quando `true`, uma falha final de node sem `continue_on_error` pausa o run e mantém um checkpoint para retomada;
- `output_dir`: diretório de persistência do run;
- `max_node_output_bytes`: limite de captura de saída por node.

#### `pause_when_fail` versus `continue_on_error`

- `continue_on_error` é configurado por node e diz "siga o resto do plano mesmo que este node falhe". O run permanece `running` e segue para o próximo node.
- `pause_when_fail` é configurado no `execution` do workflow e diz "se um node sem `continue_on_error` esgotar os retries e falhar, pause o run em vez de finalizar como `failed`".
- Os retries do node são tentados antes da pausa: só depois de esgotar o último retry sem sucesso o runtime pausa o run.
- O resume re-executa o node em `retry_node_id` do checkpoint; nodes anteriores não são re-executados e continuam disponíveis em `${nodes...}`.

## Como funciona

A validação acontece em duas etapas principais:

1. `Validate` confere a estrutura geral do workflow:
   - a versão precisa ser exatamente `"1"`;
   - o nome não pode estar vazio;
   - `nodes` precisa ter pelo menos um item;
   - os `inputs` declarados são verificados antes do restante do spec;
   - o grafo é montado em seguida para detectar ciclos e validar saltos.
2. `validateWorkflowScope` percorre os nodes e aplica as regras do DSL:
   - rejeita `id` vazio e IDs duplicados no mesmo escopo;
   - valida o `kind` do node;
   - valida o provider do node `agent`;
   - verifica `depends_on`;
   - valida referências estáticas para outros nodes;
   - desce recursivamente para filhos de nodes `map`.

### Inputs tipados

O spec suporta os tipos de input:

- `string`
- `integer`
- `number`
- `boolean`
- `array`
- `object`

Cada input pode declarar `default`, e o valor padrão é validado contra o tipo informado.
Além disso, o helper `ValidateInputValues` reaproveita a mesma lógica para conferir valores
fornecidos em tempo de entrada.

### Referências estáticas

A validação procura referências textuais em campos como `when`, `go_to_if.when`, `for_each`,
`prompt`, `system`, `command`, `working_dir`, `input` e `env`.

Quando a expressão cita `nodes.<id>.output` ou `nodes.<id>.outputs`, o validador checa se o nó
referenciado é expandido por `for_each`:

- nó expandido exige `outputs`;
- nó não expandido exige `output`.

Se a referência apontar para um node inexistente, a validação falha antes da execução.

### Artefatos

Nodes `bash` podem declarar artefatos produzidos no `working_dir`:

```yaml
nodes:
  - id: scan
    kind: bash
    command: "trivy filesystem -o reports/security.json ."
    artifacts:
      - name: security-report
        path: reports/security.json
        media_type: application/json
        description: Vulnerability scan results
```

Regras de validação:

- `name` é obrigatório e define a chave de acesso em `${nodes.<id>.artifacts.<name>.id}`.
- `path` é obrigatório, deve ser relativo e não pode conter `..`.
- `media_type` é opcional; quando ausente, o runtime infere a partir da extensão do arquivo.
- `description` é opcional e preservada como metadado.

O runtime copia cada artefato declarado de forma segura do `working_dir` do node para o diretório
de artefatos do run (`<run_dir>/artifacts/nodes/<node_id>/artifacts/<name>`). A cópia rejeita
symlinks e arquivos fora do `working_dir` permitido. Conteúdo textual passa pelo mascaramento de
secrets; binários são copiados sem transformação.

Além dos artefatos declarados, todo node produz automaticamente três artefatos nativos:

- `result.json` — resultado completo do node (status, output, stdout, stderr, exit_code).
- `stdout.txt` — saída padrão capturada.
- `stderr.txt` — saída de erro capturada.

Esses artefatos são indexados com `kind: result`, `kind: stdout` e `kind: stderr`, respectivamente.
Em execuções `for_each` ou `map`, os IDs incluem `instance_id` para garantir unicidade:
`nodes/<node_id>/<instance_id>/stdout.txt`.

Referência em expressões:

```yaml
- id: publish
  kind: bash
  depends_on: [scan]
  command: "upload ${nodes.scan.artifacts.security-report.id}"
```

### `go_to_if`, ciclos e ordem de execução

Os campos `go_to_if.when` e `go_to_if.target` são verificados durante a validação estrutural.
Depois disso, `BuildPlan` monta a ordem topológica do workflow e detecta ciclos em dependências.
O mesmo passo também valida os saltos condicionais, garantindo que `go_to_if.target` aponte para
o node atual ou para um node anterior na ordem de execução.

### Worktree

O bloco `worktree` controla a execução do workflow em um git worktree isolado. Ele pode ser
escrito de forma compacta ou estruturada.

Forma compacta:

- `worktree: true` — habilita o worktree com todos os valores padrão.
- `worktree-provider: pi` — alias legado e deprecated que habilita o worktree e define o
  provider de agente; é um atalho para `worktree.enabled: true` + `worktree.provider: pi`.

Forma estruturada:

```yaml
worktree:
  enabled: true
  provider: codex
  base: current
  merge:
    strategy: deterministic
    on_conflict: agent
  cleanup:
    on_success: true
    on_failure: keep
```

#### Precedência e conflitos

- Quando `worktree-provider` é usado junto com `worktree: true`, o atalho define `provider`.
- Quando `worktree-provider` é usado junto da forma estruturada, a forma estruturada vence,
  mas se os valores divergirem (por exemplo, `worktree.provider: codex` e `worktree-provider: pi`),
  a validação emite erro de conflito explícito.
- `worktree.provider` escolhe o agente usado em `merge.on_conflict: agent`. Criação do
  worktree, diff, apply e cleanup continuam sendo executados por Git local.

#### Valores suportados nesta versão

| Campo                | Valores suportados      | Padrão          |
| -------------------- | ----------------------- | --------------- |
| `provider`           | `codex`, `claude`, `pi` | `codex`         |
| `base`               | `current`               | `current`       |
| `merge.strategy`     | `deterministic`         | `deterministic` |
| `merge.on_conflict`  | `agent`                 | `agent`         |
| `cleanup.on_success` | `true`, `false`         | `true`          |
| `cleanup.on_failure` | `keep`                  | `keep`          |

Workflows sem a chave `worktree` continuam executando no diretório atual, sem alteração de
comportamento.

## Principais arquivos envolvidos

- [`internal/core/workflow/spec.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/spec.go): define `WorkflowSpec`, `InputSpec`, `NodeSpec`, `GoToIfSpec`, `WorktreeSpec` e os kinds suportados pelo DSL.
- [`internal/core/workflow/validation.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go): concentra as regras de validação estrutural, tipos de input, referências e providers de agente.
- [`internal/core/workflow/validation_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation_test.go): cobre defaults inválidos, referências entre nodes, permissões, escopo aninhado, saltos condicionais e worktree.
- [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go): monta o plano de execução, detecta ciclos e valida `go_to_if` durante a ordenação.
- [`internal/adapters/yaml/loader.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/yaml/loader.go): carrega e normaliza o YAML, incluindo os atalhos `worktree: true` e `worktree-provider`.
- [`samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml): exemplo real de workflow com `agent`, `bash`, `transform`, `map`, `output_schema` e referências entre nodes.

## Observações relevantes

- `provider` é validado para nodes `agent`; `codex`, `claude` e `pi` são aceitos pelo conjunto padrão, e quando o campo não é informado o domínio usa `codex` como fallback.
- `permission` só é aceito em nodes `agent`, e `permission.write` precisa estar definido quando o bloco existe.
- `extension` executa scripts JavaScript/TypeScript por Bun RPC a partir de `.agentflow/extensions`; veja [`docs/extension-nodes.md`](extension-nodes.md).
- `map` cria um escopo aninhado, mas mantém visíveis os nodes do escopo externo para referências controladas.
- `ValidateInputValues` é útil para checar payloads recebidos sem repetir a lógica de tipos do spec.
- A validação ocorre antes da execução, então erros de schema, referência e ciclo aparecem cedo e com contexto do node afetado.

## Workflow V2

A versão 2 do DSL estende V1 com `imports`, `outputs`, `hooks`, `steps` (macros), validação de schema em inputs/outputs e referências reutilizáveis via `ref`/`params`. Workflows V1 continuam totalmente compatíveis. Veja o contrato completo e o guia de migração em [`docs/workflow-dsl-v2.md`](workflow-dsl-v2.md).
