# Execução Bash

## Objetivo

Esta feature permite que nós `kind: bash` executem comandos locais no host do runner. Ela é usada para automações rápidas, validações, checagens de ambiente e outras tarefas que precisam de shell local dentro do fluxo de execução do workflow.

O comportamento central é:

- executar o comando via shell local usando `bash -lc` por padrão;
- respeitar o `working_dir` já resolvido pelo runtime e o `env` definido no node;
- capturar `stdout`, `stderr` e `exit_code`;
- expor esses valores no output do node e no resumo do run;
- emitir um warning antes da execução, porque workflows podem disparar comandos arbitrários.

## Como funciona

Quando o executor encontra um node `bash`, o dispatch acontece em [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go).

O fluxo é este:

1. o `command` do node é renderizado com o contexto atual do workflow;
2. o shell efetivo é resolvido, usando `bash` como padrão quando `node.shell` não foi informado;
3. o `working_dir` efetivo é calculado a partir do workflow e do node, e depois resolvido contra a raiz de execução do run;
4. o runtime emite um evento `node.bash.warning` antes de chamar o shell;
5. o comando é executado pelo adapter de shell;
6. a saída é registrada no resultado do node e persistida no run.

O adapter de shell está em [`internal/adapters/shell/os_exec.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/shell/os_exec.go). Ele usa `exec.CommandContext(ctx, shell, "-lc", command)`, o que mantém a semântica do `bash -lc` por padrão. Antes de executar, o adapter:

- aplica `Dir` quando existe `working_dir`;
- parte do ambiente do processo atual;
- injeta as variáveis declaradas em `node.env`;
- captura `stdout` e `stderr` em buffers limitados por `max_node_output_bytes`.

O `exit_code` é derivado do resultado do processo. Quando o comando termina com código diferente de zero, o node é marcado como falho. Se o processo não puder ser iniciado, o erro também falha o node.

No estado de execução, o resultado do node inclui campos explícitos para `output`, `stdout`, `stderr` e `exit_code`. Isso permite que nós seguintes consultem esses valores em expressões e templates, por exemplo em condições `when`, `depends_on` e etapas de correção.

## Arquivos envolvidos

- [`internal/core/runtime/handlers/dispatch.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/dispatch.go): renderiza o comando, resolve `working_dir`, emite o warning e monta o output do node.
- [`internal/adapters/shell/os_exec.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/shell/os_exec.go): executa o processo local, herda o ambiente do sistema, aplica `node.env` e captura saída e código de saída.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobre os cenários de `working_dir`, mascaramento, warning antecipado e persistência de `stdout` e `stderr`.
- [`samples/workflows/local-health-check.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/local-health-check.yaml): exemplo de uso para checagens locais com captura de saída.
- [`samples/workflows/fix-github-issue.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml): fluxo maior que combina nós `agent` e `bash`, usando a saída do shell para decidir passos posteriores.

## Observações relevantes

- A execução é local e não isolada por padrão. O warning antes do run existe justamente porque o workflow pode rodar qualquer comando permitido pelo ambiente.
- O `working_dir` pode vir de `defaults.working_dir` do workflow, ser sobrescrito no node ou ser resolvido como caminho absoluto.
- As variáveis de `node.env` têm precedência sobre o ambiente herdado quando usam a mesma chave.
- O limite de saída por node vem de `execution.max_node_output_bytes`; quando não é informado, o runtime usa o padrão de 1 MiB.
- Os exemplos em `samples/` usam `capture` para deixar explícito que a saída do shell é parte do contrato do node, mesmo quando o runtime já expõe esses campos no resultado.
- O padrão `bash -lc` favorece compatibilidade com scripts e expansão de variáveis do shell, mas também significa que o comando será interpretado pela shell local do host.
