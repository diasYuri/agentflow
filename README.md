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

AgentFlow transforma processos de desenvolvimento em workflows YAML versionáveis, auditáveis e reproduzíveis. Você descreve a sequência. O motor executa com dependências explícitas, paralelismo controlado, aprovação humana, artefatos persistidos e suporte a agentes locais via CLI.

Foi pensado para ser uma das plataformas mais completas para workflows locais de agente: previsível como automação clássica, flexível como um orquestrador moderno e concreta o bastante para uso diário em repositórios reais.

Se a sua rotina hoje depende de passos soltos no terminal, prompts copiados à mão e decisões implícitas, AgentFlow organiza isso em um fluxo que dá para revisar, reproduzir e automatizar.

## Por Que Usar

- Repetível: o mesmo workflow executa a mesma sequência de etapas, toda vez.
- Auditável: `graph` e `dry-run` mostram o plano antes da execução.
- Componível: mistura `agent`, `bash`, `transform`, `map`, `approval` e `noop` no mesmo grafo.
- Paralelo quando faz sentido: expande listas com `for_each`, `concurrency` e `max_items`.
- Persistente: runs, logs, eventos e artefatos ficam salvos localmente.
- Local por padrão: roda no seu ambiente, com o seu repositório e os seus binários.

## O Que Ele Faz

- Define workflows em YAML com `inputs`, `vars`, `secrets`, `defaults`, `execution` e `nodes`.
- Executa agentes locais com `provider: codex`, `provider: claude` ou `provider: pi`.
- Orquestra comandos locais com `kind: bash` e transformações com `kind: transform`.
- Faz fan-out com `kind: map` e junções com `kind: noop`.
- Suporta aprovação humana, pausa em falha e retomada de runs.
- Controla workflows e runs via CLI, daemon local e TUI.

## Requisitos

- Go 1.26.1+
- `codex`, `claude` ou `pi` no `PATH` quando o workflow usar `kind: agent`

## CLI

### Comandos Principais

- `agentflow validate <workflow>`: valida um workflow.
- `agentflow graph <workflow>`: imprime o grafo Mermaid.
- `agentflow dry-run <workflow>`: resolve entradas e mostra o plano.
- `agentflow run <workflow>`: executa via `agentflowd` por padrão.
- `agentflow run <workflow> -it`: executa no foreground, sem daemon.
- `agentflow migrate <workflow>`: migra workflow v1 para v2.
- `agentflow project add <name> <path>`: registra um projeto.
- `agentflow project list`: lista projetos registrados.
- `agentflow project remove <name>`: remove um projeto.
- `agentflow daemon start|stop|status`: controla o daemon local.
- `agentflow tui`: abre a interface terminal interativa.

### Namespace `workflow`

- `agentflow workflow run <workflow>`: inicia um run no daemon.
- `agentflow workflow list`: lista runs conhecidos.
- `agentflow workflow status <id>`: mostra o status de um run.
- `agentflow workflow watch <id>`: acompanha o run até terminar.
- `agentflow workflow logs <id>`: imprime eventos do run.
- `agentflow workflow artifacts <run_id>`: lista artefatos.
- `agentflow workflow artifact show <run_id> <artifact_id>`: mostra um artefato.
- `agentflow workflow artifact path <run_id> <artifact_id>`: imprime o caminho local do artefato.
- `agentflow workflow cancel <id>`: cancela um run.
- `agentflow workflow pause <id>`: pede pausa graciosa.
- `agentflow workflow resume <id>`: retoma um run pausado.
- `agentflow workflow approve <id>`: aprova um run aguardando decisão humana.
- `agentflow workflow reject <id>`: rejeita um run aguardando decisão humana.
- `agentflow workflow summary <run_id>`: mostra o resumo do run.
- `agentflow workflow timeline <run_id>`: mostra a linha do tempo do run.
- `agentflow workflow inspect <run_id>`: exibe diagnósticos do run.
- `agentflow workflow schedule add <workflow>`: cria um agendamento.
- `agentflow workflow schedule list`: lista agendamentos.
- `agentflow workflow schedule remove <id>`: remove um agendamento.

## Arquitetura

AgentFlow foi organizado para separar intenção, planejamento e execução. Isso reduz acoplamento, facilita manutenção e deixa o comportamento do sistema mais previsível.

- `cmd/agentflow/` e `cmd/agentflowd/` concentram as entradas da aplicação.
- `internal/cli/` traduz comandos e flags em ações explícitas.
- `internal/app/` coordena os casos de uso sem depender de detalhes de infraestrutura.
- `internal/core/workflow/` modela e valida a DSL do workflow.
- `internal/core/runtime/` executa o plano, controla estados e trata falhas.
- `internal/daemon/` persiste runs, expõe RPC e mantém a execução em background.
- `internal/adapters/` conecta o core a YAML, agentes, Git, worktrees, SQLite e outras integrações.
- `internal/tui/` entrega a interface interativa sem misturar UI com lógica de domínio.

Essa divisão é o que coloca o AgentFlow em uma faixa acima das ferramentas mais simples do mercado: o workflow não é só um script com prompt. Ele vira um artefato de execução com validação, persistência, observabilidade e recuperação de estado.

Em prática, isso significa:

- você consegue revisar o grafo antes de rodar;
- você consegue isolar execução em background via daemon;
- você consegue auditar resultados e artefatos depois;
- você consegue repetir o mesmo processo em times diferentes sem reexplicar a operação;
- você consegue compor etapas determinísticas e etapas de agente no mesmo fluxo.

## Flags Mais Úteis

- `--input key=value`: adiciona ou sobrescreve um input.
- `--input-json <file>`: carrega inputs de um JSON.
- `--var key=value`: sobrescreve variáveis do workflow.
- `--max-concurrency <n>`: sobrescreve `execution.max_concurrency`.
- `--working-dir <path>`: define o diretório base da execução.
- `--project <name>`: resolve o workflow dentro de um projeto registrado.
- `--codex-path <path>`, `--claude-path <path>`, `--pi-path <path>`: apontam para o binário do provider.
- `--log-format text|json`: controla o formato dos logs.
- `--events-jsonl <path>`: grava eventos em JSONL.
- `--dry-run`: valida e planeja sem executar.
- `--tag <name>`: nome amigável para o run ou agendamento.
- `--output text|json`: controla a saída dos comandos de consulta.
- `--no-color`: desativa cores.
- `--watch`: acompanha o run até a conclusão.

## Workflows Com Agente

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

Um workflow declara `version`, `name`, `inputs`, `vars`, `secrets`, `defaults`, `execution` e uma lista de `nodes`.

Tipos de node suportados:

- `agent`: delega trabalho para um agente.
- `approval`: pausa para decisão humana.
- `bash`: executa comandos locais e captura saída.
- `transform`: transforma dados entre etapas.
- `map`: expande uma lista em execuções paralelas.
- `noop`: cria marcos, junções e etapas condicionais sem efeito externo.

Recursos comuns do DSL:

- `depends_on` para dependências explícitas.
- `when` para execução condicional.
- `go_to_if` para loops controlados.
- `for_each`, `concurrency` e `max_items` para fan-out.
- `output_schema` para respostas estruturadas de agentes.
- `secrets` para ler valores sensíveis do ambiente.
- `artifacts` para persistir saídas relevantes do workflow.
- `execution.pause_when_fail` para pausar runs em falha e retomar depois.

## Daemon E Estado Local

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

## Exemplos Incluídos

Os exemplos em `samples/workflows/` e `samples/inputs/` cobrem casos reais como:

- review de arquivos alterados
- correção de falhas
- notas de release
- auditoria de segurança
- migração de workflow
- agendamentos
- loops de validação
- aprovação humana

Veja também [samples/README.md](samples/README.md) para comandos prontos e descrições dos exemplos.

## Segurança

Workflows podem executar comandos locais. Revise cada `command` antes de rodar. Use `graph` e `dry-run` para auditar o plano, e prefira `-it` quando quiser manter a execução no foreground.

## Próximos Passos

1. Rode `go run ./cmd/agentflow validate samples/workflows/fix-github-issue.yaml`.
2. Execute `go run ./cmd/agentflow dry-run ...` em um workflow real do seu repositório.
3. Copie um sample para `.agentflow/workflows/` e adapte para o seu processo de trabalho.
