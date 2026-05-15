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
- `workflow status <id>`: mostra o estado de um run.
- `workflow logs <id>`: imprime o `events.jsonl` do run.
- `workflow cancel <id>`: cancela um run em execução.
- `daemon start|stop|status`: controla o processo `agentflowd`.

O alias legado `run <workflow>` chama `workflow run <workflow>`. Para execução local foreground, use `run -it <workflow>` ou `workflow run -it <workflow>`.

O pipeline de execução local `-it` usa o caso de uso `RunWorkflowUseCase` com:

- repositório YAML para carregar o workflow;
- repositório local de runs para persistir artefatos;
- sink de eventos em `stdout` e, opcionalmente, em JSONL;
- provider de agentes `codex` quando o workflow pede `kind: agent`;
- runner de shell para etapas locais.

### Resolução de entradas

As entradas são combinadas nesta ordem:

1. `--input-json` carrega um arquivo JSON com valores base;
2. `--input key=value` sobrescreve ou adiciona chaves individuais;
3. `--var key=value` injeta variáveis separadas para o workflow;
4. `--max-concurrency` sobrescreve `execution.max_concurrency` quando informado;
5. `--working-dir` define o diretório base da execução;
6. `--codex-path` aponta para o binário `codex` usado pelo provider de agentes.

O parser tenta converter valores simples para `bool`, `int`, `float` ou JSON válido antes de manter a string bruta.

### Localização dos workflows

Os workflows são resolvidos por nome/ref, seguindo a convenção documentada em [`samples/README.md`](/Users/yuri/git/diasYuri/agentflow/samples/README.md). O padrão descrito nos samples é procurar primeiro em `./.agentflow/workflows` e depois em `~/.agentflow/workflows`.

## Arquivos principais

- [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go): ponto de entrada do binário e integração com sinais do sistema.
- [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go): definição dos comandos `validate`, `graph`, `dry-run` e `run`, além do parsing de flags e inputs.
- [`cmd/agentflowd/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflowd/main.go): ponto de entrada do daemon.
- [`internal/daemon`](/Users/yuri/git/diasYuri/agentflow/internal/daemon): RPC local, supervisão e gerenciamento de workflows em background.
- [`internal/cli/root_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root_test.go): cobertura dos comportamentos visíveis do CLI, incluindo grafo Mermaid e flags suportadas.
- [`readme.md`](/Users/yuri/git/diasYuri/agentflow/readme.md): ponto de entrada de uso rápido do projeto e referência para instalação do binário.

## Observações relevantes

- `graph` aceita apenas `--format mermaid`; qualquer outro formato retorna erro.
- `validate` e `graph` validam a definição do workflow, mas não executam etapas nem resolvem inputs externos.
- `dry-run` não executa comandos; ele mostra o plano já resolvido em JSON para inspeção ou automação.
- `run` aceita `--dry-run` para validar e planejar sem executar; por padrão essa solicitação vai para o daemon.
- `run -it` é o caminho compatível para executar no processo da CLI.
- A CLI não expõe `--output-dir`; os runs são gravados no storage local padrão em `.agentflow/runs` ou `~/.agentflow/runs`, dependendo do modo de execução.
- `run` imprime os metadados do run apenas quando a execução gera um `RunID`, o que facilita rastrear o artefato correspondente em disco.
