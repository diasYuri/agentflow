# Persistência e sigilo

## Objetivo

Esta feature registra a trilha completa de cada execução local do `agentflow` e, ao mesmo tempo, evita vazamento de segredos nos artefatos persistidos.

Em cada run, o runtime grava metadados, workflow original, forma normalizada, plano de execução e resumo final. Além disso, persiste a saída de nós e instâncias em disco para permitir auditoria, depuração e retomada de análise depois da execução.

## Como funciona

A persistência local é implementada pelo repositório [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go). Ele cria uma pasta por execução e escreve os arquivos principais do run:

- `run.json`: metadados iniciais do run, como `run_id`, workflow, `workflow_path`, `started_at` e `output_dir`.
- `workflow.yaml`: cópia do workflow de origem, quando o arquivo fonte está disponível.
- `normalized.json`: versão normalizada do workflow já carregado e resolvido.
- `plan.json`: plano de execução montado pelo runtime.
- `summary.json`: resumo final do run com status, métricas e resultados por nó.

A estrutura criada para cada execução também separa os resultados dos nós em `nodes/`, usando o caminho da expansão quando existe `for_each` ou `map`. Dentro de cada nó ou instância, o repositório grava:

- `result.json`: resultado serializado do nó ou da instância.
- `stdout.txt`: saída padrão capturada, quando houver.
- `stderr.txt`: saída de erro capturada, quando houver.

O diretório `artifacts/` é criado desde o início do run e fica reservado para saídas futuras produzidas por etapas do workflow.

O diretório base dos runs é resolvido por padrão em `~/.agentflow/runs`. Quando o diretório home não pode ser resolvido, a implementação cai para `.agentflow/runs` no diretório atual.

### Carregamento de secrets

O carregamento de segredos acontece em [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go) por meio de `loadSecrets`.

- cada secret declarado no spec precisa apontar para uma variável de ambiente em `secrets.*.env`;
- o runtime lê o valor do ambiente apenas quando a variável existe;
- os secrets carregados entram no estado de execução e ficam disponíveis para templates e expressões como `secrets.<nome>`.

### Mascaramento

Os valores sensíveis são mascarados pelo tipo `SecretMasker` em [`internal/core/run/mask.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/mask.go).

- strings secretas são substituídas por `[REDACTED]`;
- eventos são redigidos antes de irem para o sink;
- resultados de nós, saídas textuais, erros e outputs compostos passam pelo mesmo redator;
- o `summary.json` persistido também é salvo já mascarado.

O estado interno pode continuar usando o valor original para avaliar comandos, prompts e expressões, mas tudo que sai da trilha pública da execução é filtrado antes de persistir.

### Fluxo de execução

O encadeamento principal vive em [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go) e [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go).

1. o workflow é carregado e os secrets declarados no spec são resolvidos no ambiente;
2. o estado de execução é inicializado com inputs, vars, secrets e métricas;
3. o run é criado e o storage local recebe `run.json` e a estrutura de diretórios;
4. o workflow normalizado e o plano são salvos;
5. cada nó executado grava seu resultado em `nodes/.../result.json` e, se aplicável, `stdout.txt` e `stderr.txt`;
6. no final, o resumo é mascarado e persistido em `summary.json`.

## Arquivos principais

- [`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go): repositório local de runs, criação da árvore de diretórios e persistência dos arquivos de execução.
- [`internal/core/run/types.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/types.go): tipos serializados para metadados, resultados de nós, eventos e resumo.
- [`internal/core/run/mask.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/mask.go): redator de secrets aplicado em eventos, resultados e resumo.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): leitura de secrets do ambiente e helpers usados pela execução.
- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): estado compartilhado do run, persistência de resultados e finalização com resumo mascarado.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobertura dos arquivos persistidos, da máscara de secrets e do comportamento do storage local.

## Observações relevantes

- A persistência é pensada para execução local e produção de artefatos auditáveis; não há rotação automática de runs antigos.
- `stdout.txt` e `stderr.txt` só são criados quando a respectiva saída existe, o que mantém o storage enxuto.
- A estrutura `nodes/...` preserva o caminho de expansão, então o mesmo nó pode ter resultados separados por instância.
- O mascaramento cobre eventos, erros, resultados e resumo, mas só alcança valores que apareçam como strings conhecidas no conjunto de secrets carregados.
- O arquivo `artifacts/` já nasce pronto para extensões futuras, mesmo quando o workflow atual não produz artefatos próprios.
