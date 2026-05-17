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

AgentFlow executa automações de desenvolvimento como workflows YAML auditáveis, versionáveis e reproduzíveis. Ele combina etapas de agente, shell, transformações, fan-out, paralelismo, aprovação humana e runs persistidos, tudo pela linha de comando.

## O que ele faz

- Define fluxos em YAML com `inputs`, `vars`, dependências e políticas de execução.
- Executa `kind: agent`, `bash`, `transform`, `map` e `noop` no mesmo grafo.
- Valida a estrutura, gera grafo Mermaid e faz `dry-run` antes de rodar.
- Roda localmente em foreground com `-it` ou em background via `agentflowd`.
- Persiste runs, logs, eventos, artefatos e agendamentos no diretório local do usuário.
- Suporta providers `codex`, `claude` e `pi` para workflows com agente.

## Requisitos

- Go 1.26.1+
- `codex`, `claude` ou `pi` no `PATH` quando o workflow usar `kind: agent`

## Build

```bash
go build ./cmd/agentflow
go build ./cmd/agentflowd
go build ./cmd/agentflow-desktop
```

## CLI

### Comandos principais

- `agentflow validate <workflow>`: valida o workflow.
- `agentflow graph <workflow>`: imprime o grafo Mermaid.
- `agentflow dry-run <workflow>`: resolve inputs e mostra o plano.
- `agentflow run <workflow>`: executa via `agentflowd` por padrão.
- `agentflow run <workflow> -it`: executa localmente no foreground.
- `agentflow migrate <workflow>`: migra workflow v1 para v2.
- `agentflow project add <name> <path>`: registra um projeto.
- `agentflow project list`: lista projetos registrados.
- `agentflow project remove <name>`: remove um projeto.
- `agentflow daemon start|stop|status`: controla o daemon.
- `agentflow tui`: abre a interface terminal interativa.

### Namespace `workflow`

- `agentflow workflow run <workflow>`: inicia um run no daemon.
- `agentflow workflow list`: lista runs conhecidos.
- `agentflow workflow status <id>`: mostra status do run.
- `agentflow workflow watch <id>`: acompanha o run até terminar.
- `agentflow workflow logs <id>`: imprime eventos do run.
- `agentflow workflow artifacts <run_id>`: lista artefatos.
- `agentflow workflow artifact show <run_id> <artifact_id>`: mostra artefato.
- `agentflow workflow artifact path <run_id> <artifact_id>`: imprime o caminho local do artefato.
- `agentflow workflow cancel <id>`: cancela um run.
- `agentflow workflow pause <id>`: pede pausa graciosa.
- `agentflow workflow resume <id>`: retoma um run pausado.
- `agentflow workflow approve <id>`: aprova um run aguardando decisão humana.
- `agentflow workflow reject <id>`: rejeita um run aguardando decisão humana.
- `agentflow workflow summary <run_id>`: mostra resumo do run.
- `agentflow workflow timeline <run_id>`: mostra a linha do tempo do run.
- `agentflow workflow inspect <run_id>`: inspeciona diagnósticos do run.
- `agentflow workflow schedule add <workflow>`: cria um agendamento.
- `agentflow workflow schedule list`: lista agendamentos.
- `agentflow workflow schedule remove <id>`: remove um agendamento.

### Flags mais usadas

- `--input key=value`: adiciona ou sobrescreve um input.
- `--input-json <file>`: carrega inputs de um JSON.
- `--var key=value`: sobrescreve variáveis do workflow.
- `--max-concurrency <n>`: sobrescreve `execution.max_concurrency`.
- `--working-dir <path>`: define o diretório base da execução.
- `--project <name>`: resolve o workflow dentro de um projeto registrado.
- `--codex-path <path>`, `--claude-path <path>`, `--pi-path <path>`: apontam para o binário do provider.
- `--log-format text|json`: controla o formato de logs emitidos.
- `--events-jsonl <path>`: grava eventos em JSONL.
- `--dry-run`: valida e planeja sem executar.
- `--tag <name>`: nome amigável para o run ou agendamento.
- `--output text|json`: controla a saída dos comandos de consulta.
- `--no-color`: desativa cores.
- `--watch`: acompanha o run até a conclusão.

## Workflows com agente

Workflows com `kind: agent` podem usar `provider: codex`, `provider: claude` ou `provider: pi`. Quando `provider` é omitido, o padrão é `codex`.

Exemplo com Codex:

```bash
go run ./cmd/agentflow run samples/workflows/fix-github-issue.yaml \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)" \
  -it
```

Exemplo com Claude Code:

```bash
go run ./cmd/agentflow run samples/workflows/claude-code-review.yaml \
  --claude-path "$(which claude)" \
  -it
```

Exemplo com Pi RPC:

```bash
go run ./cmd/agentflow run samples/workflows/pi-code-review.yaml \
  --pi-path "$(which pi)" \
  -it
```

Para resolver workflows por nome dentro de um projeto registrado:

```bash
mkdir -p .agentflow/workflows
cp samples/workflows/fix-github-issue.yaml .agentflow/workflows/fix-github-issue.yaml

go run ./cmd/agentflow project add demo .
go run ./cmd/agentflow run fix-github-issue \
  --project demo \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)"
```

## DSL

Um workflow declara `version`, `name`, `inputs`, `vars`, `defaults`, `execution` e uma lista de `nodes`.

Tipos de node suportados:

- `agent`: delega trabalho para um agente.
- `bash`: executa comandos locais e captura saída.
- `transform`: transforma dados entre etapas.
- `map`: expande uma lista em execuções paralelas.
- `noop`: cria marcos, junções e etapas condicionais sem ação externa.

Recursos comuns do DSL:

- `depends_on` para dependências explícitas.
- `when` para execução condicional.
- `go_to_if` para loops controlados.
- `for_each`, `concurrency` e `max_items` para fan-out.
- `output_schema` para respostas estruturadas de agentes.
- `secrets` para ler valores sensíveis do ambiente.

## Daemon e estado local

Por padrão, `agentflow run <workflow>` usa o daemon local `agentflowd`. O daemon mantém runs e metadados em:

- Socket: `~/.agentflow/agentflowd.sock`
- PID: `~/.agentflow/agentflowd.pid`
- Log: `~/.agentflow/agentflowd.log`
- Banco: `~/.agentflow/agentflowd.sqlite`
- Runs: `~/.agentflow/runs`
- Projetos: `~/.agentflow/projects.json`
- Agendamentos: `~/.agentflow/schedules.json`

Exemplo:

```bash
agentflow daemon start
agentflow workflow run review-changed-files --input-json samples/inputs/review-files.json
agentflow workflow list
agentflow workflow logs <run_id>
```

## Samples

Os exemplos em `samples/workflows/` e `samples/inputs/` cobrem casos como review de arquivos alterados, correção de falhas, notas de release, auditoria de segurança, migração de workflow e agendamentos.

Veja também [samples/README.md](samples/README.md) para comandos prontos e descrições dos exemplos.

## Segurança

Workflows podem executar comandos locais. Revise cada `command` antes de rodar. Use `graph` e `dry-run` para auditar o plano, e prefira `-it` quando quiser manter a execução no foreground.
