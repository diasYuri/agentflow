# Branching e loops

## Objetivo

Esta feature adiciona controle de fluxo dentro do workflow sem sair do modelo linear do plano.
Com ela, o runtime consegue:

- avaliar `when` antes de cada node e marcar o node como `skipped` quando a condição falha;
- executar `go_to_if` depois da conclusão do node;
- permitir saltos apenas para o node atual ou para nodes anteriores;
- manter loops controlados com `go_to_if` sem aceitar saltos para frente;
- continuar o fluxo após falhas pontuais quando `continue_on_error` estiver habilitado.

Na prática, isso permite montar árvores de decisão, retries de negócio e ciclos curtos de correção
ou verificação sem precisar reescrever o workflow inteiro.

## Como funciona

O comportamento é dividido entre validação, planejamento e execução.

### Validação do fluxo

[`internal/core/workflow/validation.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go)
confere a estrutura do node e valida os campos ligados a branching:

- `go_to_if.when` e `go_to_if.target` são obrigatórios quando o bloco existe;
- `go_to_if.target` precisa apontar para um node local do mesmo escopo;
- referências estáticas em `when` e `go_to_if.when` são checadas contra os nodes visíveis;
- o node referenciado por `go_to_if.target` precisa existir antes da execução.

O planner complementa essa validação em
[`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go):

- monta a ordem de execução do workflow;
- registra os saltos condicionais em `ExecutionPlan.Jumps`;
- rejeita qualquer `go_to_if.target` que aponte para frente;
- aceita apenas saltos para o node atual ou para um node anterior, o que viabiliza loops controlados.

### Execução do node

[`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go)
implementa o loop principal do runtime.
Antes de executar cada node:

- o runtime verifica se o run já falhou e se o node permite continuidade;
- o runtime checa dependências pendentes e pode marcar o node como `skipped`;
- `when` é avaliado no contexto atual do run;
- se `when` retornar `false`, o node é marcado como `skipped` e o evento `node.skipped` é emitido;
- se `when` falhar ao avaliar, o node é marcado como `failed`.

Depois que o node termina com sucesso ou falha, o runtime avalia `go_to_if`:

- `go_to_if.when` é avaliado no mesmo contexto de execução;
- se a condição for `true`, o executor salta para o node alvo;
- se a condição for `false`, o fluxo segue para o próximo node da ordem;
- se a avaliação do salto falhar, o resultado do node pode ser convertido em falha e o comportamento final depende de `continue_on_error`.

### Continuidade após erro

`continue_on_error` permite que o workflow siga vivo mesmo quando um node anterior falha.
Isso é útil em cenários como:

- reproduzir um erro e ainda assim seguir para um node de diagnóstico;
- executar uma correção somente quando uma etapa anterior falha;
- coletar evidências antes de interromper o fluxo;
- manter um branch de recuperação ativo sem encerrar o run inteiro.

No runtime, isso significa que uma falha pontual não encerra automaticamente o loop principal.
Quando `continue_on_error` está desligado, a primeira falha relevante interrompe o restante do plano.

## Arquivos principais

- [`internal/core/workflow/plan.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/plan.go): monta a ordem do plano e valida os saltos condicionais.
- [`internal/core/workflow/validation.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/validation.go): valida `when`, `go_to_if`, referências e limites do escopo.
- [`internal/core/runtime/handlers/execution.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/execution.go): avalia `when`, executa `go_to_if`, aplica `continue_on_error` e marca nodes como `skipped` ou `failed`.
- [`samples/workflows/test-failure-debugging.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/test-failure-debugging.yaml): exemplo de fluxo que reproduz falha, diagnostica e aplica correção sem abortar cedo.
- [`samples/workflows/fix-github-issue.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml): exemplo de fluxo com teste, reparo condicional e reteste controlado.

## Observações relevantes

- `when` não é erro de execução quando avalia para `false`; o node apenas vira `skipped`.
- `go_to_if` acontece depois que o node atual termina, então ele não substitui `depends_on`.
- saltos para frente são rejeitados na validação; o mecanismo foi desenhado para branches reversos e loops curtos;
- `continue_on_error` é o mecanismo correto para manter o fluxo vivo após falhas pontuais;
- os exemplos em `samples/workflows/` mostram o padrão de uso recomendado para diagnóstico, correção e verificação.
