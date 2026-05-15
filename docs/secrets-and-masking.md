# Segredos e mascaramento

## Objetivo

Esta feature permite declarar secrets no workflow e carregá-los a partir de variáveis de
ambiente do processo. Esses valores ficam disponíveis no contexto de expressão durante a
execução, para que prompts, comandos, transforms e demais campos interpolados possam usar
`secrets.*` sem duplicar configuração no YAML.

Ao mesmo tempo, o runtime redige automaticamente qualquer vazamento desses valores em eventos,
resultados, erros e artefatos persistidos do run. O valor bruto continua disponível para a
execução interna, mas tudo o que sai do executor passa por mascaramento antes de ser emitido
ou salvo em disco.

## Como funciona

A base da feature está em [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go),
onde `loadSecrets` percorre `spec.Secrets` e lê cada variável de ambiente declarada no workflow.
Somente secrets cujo `env` exista no processo entram no mapa de secrets do run.

Esse mapa é então usado na criação do estado de execução em
[`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go).
O `ExecutionState` mantém:

- `secrets`, que é exposto ao motor de expressões;
- `masker`, construído a partir dos secrets carregados;
- `nodes` e `results`, que acumulam o estado do run enquanto ele avança.

O contexto de expressão retornado por `evalContext` inclui `Secrets: s.secrets`, então os valores
carregados ficam disponíveis para qualquer interpolação que passe por esse contexto. Na prática,
isso cobre a renderização de prompts, comandos, transforms e condições do fluxo.

A redaction propriamente dita acontece no pacote
[`internal/core/run/mask.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/mask.go).
O `SecretMasker` coleta somente valores string não vazios, remove duplicados e ordena por tamanho
decrescente para evitar vazamentos parciais quando um secret é prefixo de outro. A partir daí, ele:

- substitui ocorrências exatas por `[REDACTED]`;
- percorre strings, slices e mapas aninhados;
- mascara `Event.Data`, `NodeResult` e `Summary` antes da publicação;
- mantém o formato dos dados, trocando apenas os trechos sensíveis.

No fluxo de execução, [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go)
aplica esse mascaramento nos pontos de saída:

- `recordNode` salva o `NodeResult` mascarado no repositório de runs;
- `emitState` mascara o payload do evento antes de entregá-lo ao sink;
- `finish` mascara o resumo final antes de chamar `FinalizeRun`;
- `maskError` redige a mensagem final retornada ao chamador.

O repositório local de runs grava os dados já redigidos em
`stdout.txt`, `stderr.txt`, `result.json` e `summary.json`, como implementado em
[`internal/adapters/runrepo/local/repository.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/runrepo/local/repository.go).

## Arquivos envolvidos

- [`internal/core/run/mask.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/mask.go): implementa `SecretMasker`, a substituição por `[REDACTED]` e o mascaramento de eventos, resultados e resumo.
- [`internal/core/runtime/handlers/helpers.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/helpers.go): carrega secrets do ambiente a partir do workflow.
- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): mantém secrets e masker no estado de execução e aplica redaction na saída do run.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobre o cenário de vazamento em bash, eventos e arquivos persistidos.

## Observações relevantes

- Secrets só entram no run quando o `env` declarado no workflow existe no processo.
- Apenas valores string são usados pelo mascarador; valores vazios ou não string são ignorados.
- A redaction usa substituição literal, então qualquer ocorrência do segredo em texto livre vira `[REDACTED]`.
- O ordenamento por tamanho evita que um secret menor masque apenas parte de outro mais longo.
- O estado interno precisa manter os valores reais para avaliar expressões e montar chamadas de runtime; o mascaramento é aplicado nas bordas de saída.
- O teste em [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go) valida que comando, `stdout`, `stderr`, `result.json` e `summary.json` não preservam o secret bruto.
