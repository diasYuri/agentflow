# Linha de comando

## Objetivo

Esta feature expõe a interface de linha de comando do `agentflow` para validar, inspecionar, simular e executar workflows locais. O fluxo cobre quatro comandos principais:

1. validar a definição do workflow;
2. gerar o grafo de execução;
3. montar um plano sem executar;
4. executar o workflow e registrar o run local.

Além do workflow em si, a CLI permite sobrescrever entradas, variáveis e parâmetros de execução por flags, sem precisar alterar o YAML original.

## Como funciona

O binário principal inicia a CLI em [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go). Esse arquivo apenas cria um contexto com cancelamento por sinal e delega a execução para o pacote [`internal/cli`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go).

Em [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go), o comando raiz registra quatro subcomandos:

- `validate <workflow>`: valida o workflow e imprime um resumo no formato `valid: <nome> (<n> nodes)`.
- `graph <workflow>`: valida o workflow e imprime o grafo em Mermaid.
- `dry-run <workflow>`: resolve entradas, monta o plano e imprime um JSON com `workflow`, `inputs`, `order` e `nodes`.
- `run <workflow>`: executa o workflow localmente e, quando a execução gera um `RunID`, imprime `run_id`, `run_dir` e `status`.

O pipeline de execução usa o caso de uso `RunWorkflowUseCase` com:

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
- [`internal/cli/root_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root_test.go): cobertura dos comportamentos visíveis do CLI, incluindo grafo Mermaid e persistência de runs fora de `--output-dir`.
- [`readme.md`](/Users/yuri/git/diasYuri/agentflow/readme.md): ponto de entrada de uso rápido do projeto e referência para instalação do binário.

## Observações relevantes

- `graph` aceita apenas `--format mermaid`; qualquer outro formato retorna erro.
- `validate` e `graph` validam a definição do workflow, mas não executam etapas nem resolvem inputs externos.
- `dry-run` não executa comandos; ele mostra o plano já resolvido em JSON para inspeção ou automação.
- `run` aceita `--dry-run` para validar e planejar sem executar.
- O diretório de saída informado por `--output-dir` é aceito pela CLI, mas a implementação atual continua gravando os runs no storage local padrão em `.agentflow/runs`.
- `run` imprime os metadados do run apenas quando a execução gera um `RunID`, o que facilita rastrear o artefato correspondente em disco.
