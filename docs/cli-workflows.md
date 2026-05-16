# CLI de workflows

## Objetivo

Esta feature expõe a interface de linha de comando do `agentflow` para operar workflows locais por nome. O fluxo cobre quatro etapas principais:

1. validar a definição do workflow;
2. inspecionar o grafo de execução;
3. simular a execução com resolução de inputs;
4. executar o workflow e registrar o run local.

Além do workflow em si, o CLI aplica overrides de `inputs`, `vars` e `max_concurrency` via flags, permitindo ajustar uma mesma definição sem editar o YAML.

## Como funciona

O binário principal inicia o CLI em [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go), que apenas cria um contexto com cancelamento por sinal e delega para o pacote [`internal/cli`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go).

Em [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go), o comando raiz registra quatro subcomandos:

- `validate <workflow>`: valida a definição e imprime um resumo no formato `valid: <nome> (<n> nodes)`.
- `graph <workflow>`: valida o workflow e imprime o grafo em Mermaid.
- `dry-run <workflow>`: resolve inputs, monta o plano e imprime um JSON com `workflow`, `inputs`, `order` e `nodes`.
- `run <workflow>`: executa o workflow localmente e exibe `run_id`, `run_dir` e `status` quando a execução retorna um identificador de run.

O pipeline de execução usa o use case `RunWorkflowUseCase` com:

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
4. `--max-concurrency` sobrescreve `execution.max_concurrency` quando informado.

Para workflows com agentes, `--codex-path` aponta para o binário do provider `codex`, `--claude-path` aponta para o binário do provider `claude` e `--pi-path` aponta para o binário do provider `pi`. Em execuções via daemon, `--claude-path` é propagado como `AGENTFLOW_CLAUDE_PATH` e `--pi-path` como `AGENTFLOW_PI_PATH`; sem override, os providers ainda podem usar `CLAUDE_PATH`, `PI_PATH` ou resolver o binário pelo `PATH`.

O parser também tenta converter valores simples para `bool`, `int`, `float` ou JSON válido antes de manter a string bruta.

### Localização dos workflows

Os workflows são resolvidos por nome/ref, com suporte aos diretórios de trabalho documentados em [`samples/README.md`](/Users/yuri/git/diasYuri/agentflow/samples/README.md). O padrão descrito nos samples é procurar primeiro em `./.agentflow/workflows` e depois em `~/.agentflow/workflows`.

## Arquivos principais

- [`cmd/agentflow/main.go`](/Users/yuri/git/diasYuri/agentflow/cmd/agentflow/main.go): ponto de entrada do binário e integração com sinais do sistema.
- [`internal/cli/root.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root.go): definição dos comandos `validate`, `graph`, `dry-run` e `run`, além do parsing de flags e inputs.
- [`internal/cli/root_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/cli/root_test.go): cobertura dos comportamentos visíveis do CLI, incluindo grafo Mermaid e flags suportadas.
- [`samples/README.md`](/Users/yuri/git/diasYuri/agentflow/samples/README.md): documentação de uso dos workflows de exemplo e dos diretórios de resolução.

## Observações relevantes

- `graph` aceita apenas `--format mermaid`; qualquer outro formato retorna erro.
- `validate` e `graph` validam a definição do workflow, mas não executam etapas nem resolvem inputs externos.
- `dry-run` não executa comandos; ele mostra o plano já resolvido em JSON para inspeção ou automação.
- `run` pode receber `--dry-run` para validar e planejar sem executar, mas o comportamento padrão é executar de fato.
- A CLI não expõe `--output-dir`; os runs continuam sendo gravados no storage local padrão.
- `run` imprime os metadados do run apenas quando a execução gera um `RunID`, o que facilita rastrear o artefato correspondente em disco.
