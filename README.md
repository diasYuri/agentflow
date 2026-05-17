# AgentFlow

```text
     ___                    __  ______
    /   | ____ ____  ____  / /_/ ____/___ _      __
   / /| |/ __ `/ _ \/ __ \/ __/ /_  / __ \ | /| / /
  / ___ / /_/ /  __/ / / / /_/ __/ / /_/ / |/ |/ /
 /_/  |_\__, /\___/_/ /_/\__/_/    \____/|__/|__/
       /____/

      YAML workflows for local coding agents
```

**AgentFlow transforma automações de desenvolvimento em workflows YAML auditáveis, versionáveis e executáveis localmente.**  
Valide, visualize, simule e rode pipelines com agentes Codex, comandos shell, transformações, fan-out, paralelismo e runs persistidos, tudo pela linha de comando.

## Por que usar

AgentFlow foi feito para tirar fluxos repetitivos da cabeça do time e colocá-los em arquivos claros. Em vez de copiar prompts, rodar comandos soltos e perder o histórico do que aconteceu, você descreve o fluxo uma vez e executa com rastreabilidade.

- **Workflows como código:** defina etapas, dependências, inputs, variáveis e políticas em YAML.
- **Agentes + shell no mesmo plano:** combine `kind: agent`, `bash`, `transform`, `map` e `noop`.
- **Inspeção antes da execução:** rode `validate`, gere grafo Mermaid e faça `dry-run`.
- **Execução local controlada:** use foreground com `-it` ou rode em background pelo `agentflowd`.
- **Paralelismo e fan-out:** distribua trabalho com `map`, `for_each`, `concurrency` e `max_concurrency`.
- **Artefatos persistidos:** acompanhe runs, logs e eventos em `.agentflow/runs` ou `~/.agentflow/runs`.

## Instalação

Requisitos:

- Go 1.24+
- Codex CLI ou Claude Code CLI no `PATH` para workflows com `kind: agent`

Build dos binários:

```bash
go build ./cmd/agentflow
go build ./cmd/agentflowd
```

Ou execute direto durante o desenvolvimento:

```bash
go run ./cmd/agentflow validate samples/workflows/local-health-check.yaml
```

## Comece em 60 segundos

Valide um workflow:

```bash
go run ./cmd/agentflow validate samples/workflows/local-health-check.yaml
```

Veja o grafo de execução:

```bash
go run ./cmd/agentflow graph samples/workflows/local-health-check.yaml --format mermaid
```

Simule o plano sem executar:

```bash
go run ./cmd/agentflow dry-run samples/workflows/local-health-check.yaml
```

Execute localmente em foreground:

```bash
go run ./cmd/agentflow run samples/workflows/local-health-check.yaml -it
```

## Workflows com agentes

Workflows que usam `kind: agent` podem chamar os providers `codex`, `claude` ou `pi`. Quando `provider` é omitido, o padrão continua sendo `codex`. Informe o caminho do binário quando necessário:

```bash
go run ./cmd/agentflow run samples/workflows/fix-github-issue.yaml \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)" \
  -it
```

Exemplo mínimo usando Claude Code:

```yaml
nodes:
  - id: summarize
    kind: agent
    provider: claude
    permission:
      write: false
    prompt: "Resuma o estado do projeto sem alterar arquivos."
```

```bash
go run ./cmd/agentflow run samples/workflows/claude-code-review.yaml \
  --claude-path "$(which claude)" \
  -it
```

Exemplo mínimo usando Pi RPC:

```bash
go run ./cmd/agentflow run samples/workflows/pi-code-review.yaml \
  --pi-path "$(which pi)" \
  -it
```

Para resolver workflows por nome, copie-os para `.agentflow/workflows`:

```bash
mkdir -p .agentflow/workflows
cp samples/workflows/fix-github-issue.yaml .agentflow/workflows/fix-github-issue.yaml

go run ./cmd/agentflow run fix-github-issue \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)"
```

## Exemplo de workflow

```yaml
version: "1"
name: local-health-check
description: "Executa checagens locais rápidas sem depender de Codex."

vars:
  test_command: "go test ./..."

execution:
  max_concurrency: 2
  fail_fast: true

nodes:
  - id: list_module
    kind: bash
    command: go list ./...
    capture:
      stdout: true
      stderr: true
      exit_code: true

  - id: run_tests
    kind: bash
    depends_on: [list_module]
    command: ${vars.test_command}
    capture:
      stdout: true
      stderr: true
      exit_code: true
    continue_on_error: true

  - id: done
    kind: noop
    depends_on: [run_tests]
```

## Comandos principais

```bash
agentflow validate <workflow>              # valida estrutura, referências e grafo
agentflow graph <workflow>                 # imprime grafo Mermaid
agentflow dry-run <workflow>               # resolve inputs e mostra o plano
agentflow run <workflow>                   # inicia no daemon
agentflow run <workflow> -it               # executa localmente no foreground
agentflow workflow list                    # lista runs conhecidos
agentflow workflow status <run_id>         # mostra status de um run
agentflow workflow logs <run_id>           # imprime eventos do run
agentflow workflow cancel <run_id>         # cancela um run em execução
agentflow daemon start|stop|status         # controla o agentflowd
```

Flags úteis:

```bash
--input-json <file>       # carrega inputs de um JSON
--input key=value         # sobrescreve ou adiciona um input
--var key=value           # injeta variáveis do workflow
--max-concurrency <n>     # sobrescreve execution.max_concurrency
--working-dir <path>      # diretório base da execução
--codex-path <path>       # caminho para o binário codex
--claude-path <path>      # caminho para o binário claude
--pi-path <path>          # caminho para o binário pi
--events-jsonl <path>     # grava eventos em JSONL
--tag <name>              # nome amigável opcional para o run
```

## DSL em poucas palavras

Um workflow declara `version`, `name`, `inputs`, `vars`, `defaults`, `execution` e uma lista de `nodes`.

Tipos de node suportados:

- `agent`: delega trabalho para um agente Codex ou Claude Code.
- `bash`: executa comandos locais e captura saída.
- `transform`: transforma dados entre etapas.
- `map`: expande uma lista em execuções paralelizáveis.
- `noop`: cria marcos, junções e etapas condicionais sem ação externa.

Recursos do DSL:

- `depends_on` para dependências explícitas.
- `when` para execução condicional.
- `go_to_if` para loops controlados.
- `for_each`, `concurrency` e `max_items` para fan-out.
- `output_schema` para respostas estruturadas de agentes.
- `secrets` para ler valores sensíveis do ambiente.

## Daemon e runs

Por padrão, `agentflow run <workflow>` usa o daemon local `agentflowd`, retorna imediatamente e deixa o run em background.

```bash
agentflow daemon start
agentflow workflow run review-changed-files --input-json samples/inputs/review-files.json
agentflow workflow list
agentflow workflow logs <run_id>
```

Arquivos padrão:

- Socket: `~/.agentflow/agentflowd.sock`
- PID: `~/.agentflow/agentflowd.pid`
- Log do daemon: `~/.agentflow/agentflowd.log`
- Índice do daemon: `~/.agentflow/agentflowd.sqlite`
- Runs: `~/.agentflow/runs`

## Exemplos incluídos

- `fix-github-issue.yaml`: analisa issue, implementa correção, roda testes e aciona agente para falhas.
- `review-changed-files.yaml`: divide arquivos alterados, revisa em paralelo e consolida findings.
- `test-failure-debugging.yaml`: reproduz falha de teste, diagnostica e tenta correção.
- `security-review.yaml`: faz auditoria defensiva por áreas do repositório.
- `release-notes.yaml`: gera release notes a partir de commits e PRs.
- `product-spec-to-implementation.yaml`: transforma uma spec de produto em plano e implementação.
- `local-health-check.yaml`: roda checagens locais sem depender de agente.
- `claude-code-review.yaml`: sample mínimo com `provider: claude`, permissão somente leitura e saída estruturada.
- `pi-code-review.yaml`: sample mínimo com `provider: pi`, permissão somente leitura e saída estruturada via RPC.

## Segurança

Workflows podem executar comandos locais. Antes de rodar arquivos novos ou alterados, use:

```bash
agentflow validate <workflow>
agentflow graph <workflow>
agentflow dry-run <workflow>
```

Revise especialmente nodes `bash`, permissões de agents, `--working-dir`, `--codex-path`, `--claude-path`, `--pi-path` e qualquer workflow vindo de fora do seu repositório.

## Aplicação Desktop

O Agentflow possui uma aplicação desktop construída com [Wails v3](https://v3.wails.io/), oferecendo uma interface gráfica para carregar, editar, validar, visualizar e executar workflows.

Build do app desktop:

```bash
cd frontend/desktop && npm run build
go build ./cmd/agentflow-desktop
./agentflow-desktop
```

Funcionalidades da UI:

- Editor de workflow YAML e input JSON.
- Validação, grafo Mermaid e dry-run interativos.
- Execução de runs com timeline de eventos, logs e artefatos.
- Lista de runs com detalhes, cancelamento e acompanhamento de progresso.
- Configurações locais (tema, caminhos de agentes, workspace).

Para mais detalhes, consulte [`docs/desktop.md`](docs/desktop.md).

## Documentação

A pasta [`docs/`](docs/) detalha CLI, runtime, DSL, validação, transformações, fan-out, daemon, eventos e persistência.

Para começar pelos fundamentos:

- [`docs/cli.md`](docs/cli.md)
- [`docs/workflow-dsl.md`](docs/workflow-dsl.md)
- [`docs/runtime-execution.md`](docs/runtime-execution.md)
- [`docs/claude-agent.md`](docs/claude-agent.md)
- [`docs/pi-agent.md`](docs/pi-agent.md)
- [`samples/README.md`](samples/README.md)

Para diagnosticar performance e memória do daemon:

- [`docs/debug-profiling.md`](docs/debug-profiling.md)
