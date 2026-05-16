# Run Artifact Management

## Objetivo

Esta feature oferece a base para persistência de artefatos binários e textuais produzidos durante
a execução de um workflow. Um artefato pode ser qualquer arquivo gerado por um nó — por exemplo,
logs compilados, relatórios, imagens, dumps ou saídas transformadas — que precise ser conservado
além do ciclo de vida do processo em memória.

O objetivo é garantir que o repositório local de runs seja capaz de receber, organizar e manter
esses arquivos de forma estruturada, pronta para futura exposição via DSL, CLI ou consumo por
nodes downstream.

## Como funciona

A capability de artefatos está declarada na interface [`RunRepository`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go)
no método:

```go
SaveArtifact(ctx context.Context, runID string, name string, data []byte) error
```

A implementação local em [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go)
grava o conteúdo recebido em `<run_dir>/artifacts/<name>`. Durante a criação do run, o método
`CreateRun` já prepara o subdiretório `artifacts/` com permissões `0o755`, lado a lado com `nodes/`.

O comportamento de `SaveArtifact` é:

1. localizar o diretório do run através de `ensureRunDir`;
2. compor o caminho final dentro de `artifacts/` aplicando `filepath.Clean(name)`;
3. criar subdiretórios intermediários, quando o nome do artefato inclui path, via `os.MkdirAll`;
4. escrever o arquivo com permissões `0o644`.

Isso permite que o artefato seja gravado com um nome simples (`report.html`) ou com estrutura
aninhada (`reports/final.html`). A sanitização por `filepath.Clean` evita paths que escapem do
diretório do run.

### Artefatos de worktree

Quando um workflow tem `worktree.enabled: true`, o runtime produz artefatos adicionais sob
`<run_dir>/artifacts/worktree/`:

- `worktree/status.json` — metadados finais do worktree, incluindo `enabled`, `provider`, `name`,
  `path`, `base_commit`, `destination_commit_before_merge`, `destination_commit_after_merge`,
  `merge_status`, `cleanup_status`, `changed_files`, `conflicts` e `commands`. Esse arquivo é
  atualizado a cada etapa do ciclo de merge/cleanup.
- `worktree/diff.patch` — diff determinístico gerado pelo provider entre o worktree e o
  `base_commit`, persistido somente quando há mudanças.
- `worktree/merge.log` — registro do merge bem-sucedido, com lista de arquivos alterados e
  comandos Git executados.
- `worktree/conflicts.json` — estruturado quando o apply encontra conflitos de conteúdo ou
  falha de merge. Contém os arquivos em conflito, `base_commit`, commits do destino antes/depois
  do merge, caminho do worktree preservado, comandos Git relevantes e resumo das mudanças.

Esses artefatos passam pelo mascaramento de secrets antes da persistência, garantindo que
valores sensíveis não sejam gravados em claro.

### Estado atual da integração

A capability existe na camada de portas e na implementação local, mas **ainda não há**:

- contrato na DSL de workflow para declarar produção ou consumo de artefatos;
- comando CLI para listar, exportar ou inspecionar artefatos de um run;
- integração nos handlers de execução (`bash`, `agent`, `transform`, etc.) para automaticamente
  salvar saídas como artefatos.

Ou seja, `SaveArtifact` está disponível como primitiva do repositório e pode ser utilizada
programaticamente, mas o usuário final ainda não tem mecanismos declarativos ou interativos
para gerenciar artefatos.

## Arquivos envolvidos

- [`internal/core/ports/agent.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/ports/agent.go):
  define a interface `RunRepository`, incluindo a assinatura de `SaveArtifact`.
- [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go):
  implementa `SaveArtifact` no repositório local, com criação de diretórios e sanitização de path.
- [`internal/core/run/types.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/types.go):
  define os tipos compartilhados do domínio de run, como `RunHandle`, `RunMetadata`,
  `WorktreeMetadata`, `WorktreeChangedFile`, `WorktreeConflict` e `WorktreeGitCommand`.
- [`internal/core/runtime/handlers/worktree.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/worktree.go):
  orquestra a geração e persistência dos artefatos de worktree durante a finalização do run.

## Observações relevantes

- A pasta `artifacts/` é criada automaticamente em todo novo run, mesmo que nenhum artefato seja
  produzido, garantindo uma estrutura previsível no filesystem.
- A sanitização via `filepath.Clean` protege contra paths relativos maliciosos, mas a feature ainda
  não valida nomes de artefato no nível da DSL; quando houver integração completa, essa validação
  deverá ser reforçada.
- Artefatos são gravados de forma síncrona e direta no filesystem; não há cache, deduplicação ou
  versionamento automático no estado atual.
- O conteúdo do artefato é recebido como `[]byte`, o que torna a primitiva agnóstica a formatos:
  texto, binário, JSON ou imagens são tratados igualmente.
- Futuras extensões devem considerar: comando `agentflow artifacts <run-id>` para listagem,
  suporte a `artifact` como tipo de output em nodes do workflow, e possível streaming para artefatos
  grandes.
