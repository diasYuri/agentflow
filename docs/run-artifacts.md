# Run Artifact Management

## Objetivo

Esta feature oferece a base para persistência de artefatos binários e textuais produzidos durante
a execução de um workflow. Um artefato pode ser qualquer arquivo gerado por um nó — por exemplo,
logs compilados, relatórios, imagens, dumps ou saídas transformadas — que precise ser conservado
além do ciclo de vida do processo em memória.

O objetivo é garantir que o repositório local de runs seja capaz de receber, organizar e manter
esses arquivos de forma estruturada, com metadados públicos separados do path físico, pronta para
futura exposição via DSL, CLI ou consumo por nodes downstream.

## Modelo de domínio

Os tipos `Artifact` e `ArtifactKind` estão definidos em [`internal/core/run/types.go`](../internal/core/run/types.go):

```go
type ArtifactKind string

const (
    ArtifactKindFile    ArtifactKind = "file"
    ArtifactKindStdout  ArtifactKind = "stdout"
    ArtifactKindStderr  ArtifactKind = "stderr"
    ArtifactKindResult  ArtifactKind = "result"
    ArtifactKindSummary ArtifactKind = "summary"
    ArtifactKindCustom  ArtifactKind = "custom"
)

type Artifact struct {
    ID           string       `json:"id"`
    RunID        string       `json:"run_id"`
    NodeID       string       `json:"node_id,omitempty"`
    InstanceID   string       `json:"instance_id,omitempty"`
    Name         string       `json:"name"`
    RelativePath string       `json:"relative_path"`
    MediaType    string       `json:"media_type,omitempty"`
    SizeBytes    int64        `json:"size_bytes"`
    CreatedAt    time.Time    `json:"created_at"`
    Kind         ArtifactKind `json:"kind"`
    Description  string       `json:"description,omitempty"`
}
```

Campos obrigatórios: `id`, `run_id`, `name`, `relative_path`, `size_bytes`, `created_at`, `kind`.
O `relative_path` é relativo a `<run_dir>/artifacts/` e nunca deve ser absoluto ou conter `..`.

## Índice persistido

A implementação local mantém a fonte de verdade dos metadados em `<run_dir>/artifacts/index.json`.
Esse arquivo é atualizado atomicamente (write-then-rename) a cada novo artefato salvo.

A interface [`RunRepository`](../internal/core/ports/agent.go) expõe três métodos para artefatos:

```go
SaveArtifact(ctx context.Context, runID string, artifact run.Artifact, data []byte) error
ListArtifacts(ctx context.Context, runID string) ([]run.Artifact, error)
ReadArtifact(ctx context.Context, runID, artifactID string) ([]byte, run.Artifact, error)
```

- `SaveArtifact` grava o arquivo em disco e atualiza o índice. Se `CreatedAt` for zero, o
  repositório preenche com `time.Now().UTC()`. `SizeBytes` é sempre sobrescrito com `len(data)`.
- `ListArtifacts` retorna todos os artefatos do índice, ordenados de forma estável por
  `created_at`, `node_id`, `instance_id`, `kind`, `id`.
- `ReadArtifact` busca o artefato no índice pelo `id`, valida o `relative_path` armazenado e
  retorna os bytes do arquivo junto com os metadados.

Se `index.json` não existir, `ListArtifacts` e `ReadArtifact` retornam índice vazio ou erro
"artifact not found", respectivamente. Isso mantém compatibilidade com runs antigos que só
possuem arquivos em `artifacts/`.

## Segurança de paths

Toda escrita e leitura de artefatos passa por `validateRelativePath`, que rejeita:

- paths vazios;
- paths absolutos;
- segmentos `..`;
- qualquer path que escape de `artifacts/` via `filepath.Rel`.

Além disso, `SaveArtifact` e `ReadArtifact` rejeitam symlinks no caminho final, impedindo que o conteúdo seja escrito ou lido através de links simbólicos.

## Artefatos de worktree

Quando um workflow tem `worktree.enabled: true`, o runtime produz artefatos adicionais sob
`<run_dir>/artifacts/worktree/`:

- `worktree/status.json` — metadados finais do worktree, incluindo `enabled`, `provider`
  (agente de resolução), `git_provider`, `name`, `path`, `base_commit`,
  `destination_commit_before_merge`, `destination_commit_after_merge`, `merge_status`,
  `cleanup_status`, `changed_files`, `conflicts`, `commands`,
  `merge_failure_cause`, `agent_resolution_status`, `agent_resolution_provider` e
  `agent_resolution_error`. Esse arquivo é atualizado a cada etapa do ciclo de merge/cleanup.
  **Kind:** `custom`.
- `worktree/diff.patch` — diff determinístico gerado pelo executor Git interno entre o worktree e o
  `base_commit`, persistido somente quando há mudanças. **Kind:** `custom`.
- `worktree/merge.log` — registro do merge bem-sucedido, com lista de arquivos alterados e
  comandos Git executados. **Kind:** `custom`.
- `worktree/conflicts.json` — estruturado quando o apply encontra conflitos de conteúdo ou
  falha de merge. Contém os arquivos em conflito, `base_commit`, commits do destino antes/depois
  do merge, caminho do worktree preservado, comandos Git relevantes e resumo das mudanças.
  **Kind:** `custom`.

Esses artefatos passam pelo mascaramento de secrets antes da persistência, garantindo que
valores sensíveis não sejam gravados em claro. Todos são indexados automaticamente via
`SaveArtifact` com metadados explícitos (`kind: custom`, `node_id` vazio, media type adequado).

## Diferença entre path físico e metadados públicos

O `relative_path` controla onde o arquivo reside fisicamente dentro de `<run_dir>/artifacts/`.
O `id` é a chave pública usada para referenciar o artefato no índice e nas APIs. Na maioria dos
casos `id` e `relative_path` coincidem, mas o modelo permite que o identificador público seja
diferente do layout no filesystem quando necessário.

## Integração DSL e Runtime

A DSL de workflow permite declarar artefatos em nodes `bash`:

```yaml
nodes:
  - id: producer
    kind: bash
    command: "mkdir -p reports && echo '# Report' > reports/security.md"
    artifacts:
      - name: report
        path: reports/security.md
        media_type: text/markdown
```

A validação estrutural garante que:

- `artifacts[].name` é obrigatório;
- `artifacts[].path` é relativo, não vazio e sem `..`;
- `artifacts[].media_type` é opcional e usado para decidir se o conteúdo passa por mascaramento de secrets.

O runtime indexa automaticamente após a execução de cada node:

1. Artefatos declarados são copiados do `working_dir` do node para `artifacts/nodes/<node_id>/artifacts/<name>`.
2. `result.json`, `stdout.txt` e `stderr.txt` são sempre indexados como artefatos de primeira classe.
3. Em execuções fan-out (`for_each`), o `instance_id` é incorporado ao path do artefato para evitar colisões.

O estado do node expõe referências consultáveis:

```yaml
- id: consumer
  kind: bash
  depends_on: [producer]
  command: "echo ${nodes.producer.artifacts.report.id}"
```

## Consumo do índice pelo daemon

O daemon e a CLI consomem `artifacts/index.json` como fonte primária de metadados:

- `WorkflowArtifacts` (daemon) retorna DTOs enriquecidos (`node_id`, `instance_id`, `kind`, `media_type`, `size_bytes`, `created_at`, `description`) a partir do índice. Se o índice não existir, usa varredura do filesystem como fallback para runs antigos.
- `WorkflowArtifact` (daemon) valida o `artifact_id` contra o índice antes de ler o arquivo. Artefatos textuais são retornados inline com `encoding: text` até o limite de 128 KiB (`MaxArtifactInline`). Binários retornam apenas metadados, sem payload inline por padrão. Secrets são mascarados antes da exposição quando o run possui `vars`.
- `WorkflowArtifactPath` (daemon) resolve o path absoluto de um artefato apenas quando ele está presente no índice, rejeitando IDs não indexados e path traversal.

A resposta `WorkflowArtifactDTO` mantém aliases legados (`path`, `size`, `content_type`) para compatibilidade com consumers existentes durante a transição.

## Estado atual da integração

- ✅ DSL de declaração de artefatos em nodes `bash`.
- ✅ Validação estrutural de `name`, `path` e `media_type`.
- ✅ Cópia segura de arquivos declarados com rejeição de symlinks e path traversal.
- ✅ Indexação automática de `stdout.txt`, `stderr.txt` e `result.json`.
- ✅ Suporte a fan-out com `instance_id` em paths de artefato.
- ✅ Referências de artefatos no contexto de avaliação (`${nodes.<id>.artifacts.<name>.id}`).
- ✅ Comando CLI para listar, inspecionar e resolver path de artefatos de um run (`workflow artifacts`, `workflow artifact show`, `workflow artifact path`).
- ✅ Desktop UI consome o mesmo contrato de artefatos via bindings (`GetRunArtifacts`, `GetRunArtifact`, `GetRunArtifactPath`), sem reconstruir paths no frontend.
- ⬜ Suporte a artefatos declarativos em nodes `agent`, `transform` e `noop`.

## Arquivos envolvidos

- [`internal/core/run/types.go`](../internal/core/run/types.go):
  define `Artifact`, `ArtifactKind` e tipos relacionados.
- [`internal/core/ports/agent.go`](../internal/core/ports/agent.go):
  define a interface `RunRepository`, incluindo `SaveArtifact`, `ListArtifacts` e `ReadArtifact`.
- [`internal/adapters/runrepo/local/repository.go`](../internal/adapters/runrepo/local/repository.go):
  implementa persistência atômica do índice, validação de paths e ordenação estável.
- [`internal/adapters/runrepo/local/repository_test.go`](../internal/adapters/runrepo/local/repository_test.go):
  cobre criação de índice, paths aninhados, rejeição de path traversal, ordenação e leitura.
- [`internal/core/runtime/handlers/worktree.go`](../internal/core/runtime/handlers/worktree.go):
  orquestra a geração e persistência dos artefatos de worktree durante a finalização do run.

## Observações relevantes

- A pasta `artifacts/` é criada automaticamente em todo novo run, mesmo que nenhum artefato seja
  produzido, garantindo uma estrutura previsível no filesystem.
- A gravação do índice usa `sync.Mutex` para serializar read-modify-write, evitando corrupção em
  cenários de fan-out concorrente.
- Artefatos são gravados de forma síncrona e direta no filesystem; não há cache, deduplicação ou
  versionamento automático no estado atual.
- O conteúdo do artefato é recebido como `[]byte`, o que torna a primitiva agnóstica a formatos:
  texto, binário, JSON ou imagens são tratados igualmente.
- Futuras extensões devem considerar: comando `agentflow artifacts <run-id>` para listagem,
  suporte a `artifact` como tipo de output em nodes do workflow, e possível streaming para artefatos
  grandes.
