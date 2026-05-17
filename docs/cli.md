# Linha de comando

## Objetivo

Esta feature expõe a interface de linha de comando do `agentflow` para validar, inspecionar, simular e controlar workflows locais. O fluxo cobre comandos para:

1. validar a definição do workflow;
2. gerar o grafo de execução;
3. montar um plano sem executar;
4. iniciar workflows em background no `agentflowd`;
5. listar, inspecionar, acompanhar logs e cancelar runs.

Além do workflow em si, a CLI permite sobrescrever entradas, variáveis e parâmetros de execução por flags, sem precisar alterar o YAML original.

## Como funciona

O binário principal inicia a CLI em [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go). Esse arquivo apenas cria um contexto com cancelamento por sinal e delega a execução para o pacote [`internal/cli`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go).

Em [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go), o comando raiz registra os subcomandos:

- `validate <workflow>`: valida o workflow e imprime um resumo no formato `valid: <nome> (<n> nodes)`.
- `graph <workflow>`: valida o workflow e imprime o grafo em Mermaid.
- `dry-run <workflow>`: resolve entradas, monta o plano e imprime um JSON com `workflow`, `inputs`, `order` e `nodes`.
- `workflow run <workflow>`: inicia o workflow no daemon e imprime `run_id`, `run_dir` e `status`.
- `workflow list`: lista runs conhecidos pelo daemon.
- `workflow status <id>`: mostra o estado de um run. Aceita `--watch` para acompanhar até o run alcançar um estado terminal (`success`, `failed`, `cancelled`, `timeout` ou `paused`).
- `workflow watch <id>`: forma curta do `status --watch`.
- `workflow logs <id>`: imprime o `events.jsonl` do run.
- `workflow artifacts <id>`: lista artefatos indexados do run. Aceita `--output json`.
- `workflow artifact show <run_id> <artifact_id>`: exibe metadados e conteúdo textual inline de um artefato. Para binários, exibe metadados sem payload. Aceita `--output json`.
- `workflow artifact path <run_id> <artifact_id>`: imprime o path local absoluto do artefato (exige índice; rejeita IDs não indexados e path traversal).
- `workflow cancel <id>`: cancela um run em execução ou pausado, removendo o checkpoint salvo.
- `workflow pause <id>`: solicita pausa cooperativa no próximo checkpoint seguro do runtime. Nodes em andamento concluem antes da pausa.
- `workflow resume <id>`: retoma um run pausado a partir do checkpoint. Usa o request original; novos `--input`/`--var` não são aceitos no resume para manter reprodutibilidade.
- `daemon start|stop|status`: controla o processo `agentflowd`.
- `tui`: lança a interface terminal interativa (TUI). Aceita `--workflow`, `--run`, `--daemon`, `--no-mouse` e `--theme`. Veja [`docs/tui.md`](docs/tui.md) para detalhes completos.

O alias legado `run <workflow>` chama `workflow run <workflow>`. Para execução local foreground, use `run -it <workflow>` ou `workflow run -it <workflow>`.

O pipeline de execução local `-it` usa o caso de uso `RunWorkflowUseCase` com:

- repositório YAML para carregar o workflow;
- repositório local de runs para persistir artefatos;
- sink de eventos em `stdout` e, opcionalmente, em JSONL;
- providers de agentes `codex`, `claude` e `pi` quando o workflow pede `kind: agent`;
- runner de shell para etapas locais.

### Resolução de entradas

As entradas são combinadas nesta ordem:

1. `--input-json` carrega um arquivo JSON com valores base;
2. `--input key=value` sobrescreve ou adiciona chaves individuais;
3. `--var key=value` injeta variáveis separadas para o workflow;
4. `--max-concurrency` sobrescreve `execution.max_concurrency` quando informado;
5. `--working-dir` define o diretório base da execução;
6. `--codex-path` aponta para o binário `codex` usado pelo provider `codex`.
7. `--claude-path` aponta para o binário `claude` usado pelo provider `claude`.
8. `--pi-path` aponta para o binário `pi` usado pelo provider `pi`.
9. `--tag <name>` atribui um nome amigável opcional ao run. A tag é exibida em `workflow list`, `workflow status` e preservada nos artefatos do run.

Quando a execução vai para o daemon, esses caminhos são enviados na requisição do run. Ao iniciar o `agentflowd`, a CLI também propaga `--codex-path` como `AGENTFLOW_CODEX_PATH`, `--claude-path` como `AGENTFLOW_CLAUDE_PATH` e `--pi-path` como `AGENTFLOW_PI_PATH`. Se o caminho do Claude ou do Pi não for informado, os adapters ainda podem resolver `CLAUDE_PATH`, `PI_PATH` ou o binário correspondente no `PATH`.

O parser tenta converter valores simples para `bool`, `int`, `float` ou JSON válido antes de manter a string bruta.

### Localização dos workflows

Os workflows são resolvidos por nome/ref, seguindo a convenção documentada em [`samples/README.md`](/Users/yuri/git/diasYuri/agentflow/samples/README.md). O padrão descrito nos samples é procurar primeiro em `./.agentflow/workflows` e depois em `~/.agentflow/workflows`.

## Arquivos principais

- [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go): ponto de entrada do binário e integração com sinais do sistema.
- [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go): definição dos comandos `validate`, `graph`, `dry-run` e `run`, além do parsing de flags e inputs.
- [`cmd/agentflowd/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflowd/main.go): ponto de entrada do daemon.
- [`internal/daemon`](/Users/yuri/git/diasYuri/agentflow/internal/daemon): RPC local, supervisão e gerenciamento de workflows em background.
- [`internal/cli/root_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root_test.go): cobertura dos comportamentos visíveis do CLI, incluindo grafo Mermaid, flags suportadas e comando `tui`.
- [`readme.md`](/Users/yuri/git/diasYuri/agentflow/readme.md): ponto de entrada de uso rápido do projeto e referência para instalação do binário.

## Observações relevantes

- `graph` aceita apenas `--format mermaid`; qualquer outro formato retorna erro.
- `validate` e `graph` validam a definição do workflow, mas não executam etapas nem resolvem inputs externos.
- `dry-run` não executa comandos; ele mostra o plano já resolvido em JSON para inspeção ou automação.
- `run` aceita `--dry-run` para validar e planejar sem executar; por padrão essa solicitação vai para o daemon.
- `run --tag <name>` adiciona um nome descritivo ao run, útil para identificar execuções em listagens.
- `run -it` é o caminho compatível para executar no processo da CLI.
- `provider: claude` exige Claude Code CLI disponível via `--claude-path`, `AGENTFLOW_CLAUDE_PATH`, `CLAUDE_PATH` ou `PATH`.
- `provider: pi` exige Pi CLI disponível via `--pi-path`, `AGENTFLOW_PI_PATH`, `PI_PATH` ou `PATH`.
- A CLI não expõe `--output-dir`; os runs são gravados no storage local padrão em `.agentflow/runs` ou `~/.agentflow/runs`, dependendo do modo de execução.
- `run` imprime os metadados do run apenas quando a execução gera um `RunID`, o que facilita rastrear o artefato correspondente em disco.
- `workflow watch` para automaticamente em `paused`, mostrando o hint `agentflow workflow resume <id>` para que o usuário decida quando continuar.
- `workflow pause` é cooperativa: o runtime só pausa em pontos seguros (entre nodes, depois de gravar um resultado, durante atraso de retry). Um node ainda em execução conclui antes da pausa.
- `workflow resume` reaproveita o request salvo no daemon. Novos `--input` ou `--var` precisam de um novo `workflow run`; o resume mantém entradas, working dir, paths de provider e demais opções do run original.
- Tags não precisam ser únicas e não substituem o `run_id`; servem apenas para identificação visual.
- `agentflow tui` não conflita com subcomandos existentes; é um comando independente que não aceita positional args. Sua execução bloqueia o terminal até que o usuário pressione `q` ou `ctrl+c`.
