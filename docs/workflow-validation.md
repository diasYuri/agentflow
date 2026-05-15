# Validação de workflow

## Objetivo

Esta feature define a camada de validação do spec de workflow antes da execução.
Ela impede que um workflow inválido avance para o planner ou para o runtime,
reduzindo erros em tempo de execução e deixando as regras do formato explícitas
no próprio domínio.

Em termos práticos, a validação cobre:

- `version` deve ser `1`.
- `name` não pode estar vazio.
- o workflow precisa declarar ao menos um nó.
- `inputs` declarados devem ter tipo suportado e `default` compatível.
- valores recebidos por input podem ser checados por um helper dedicado.
- cada tipo de nó tem regras próprias de presença e consistência.
- dependências, ciclos, escopo de mapas aninhados e referências entre nós são verificados.
- campos sensíveis como `go_to_if`, `permission.write`, `provider` e referências `output`/`outputs`
  são restringidos para evitar combinações incoerentes.

## Como funciona

A validação é feita em camadas:

1. `Validate` faz as checagens estruturais do spec:
   - versão do workflow;
   - nome;
   - presença de nós;
   - validação dos `inputs`;
   - validação de escopo e referências;
   - construção do plano de execução como etapa final.
2. `validateInputSpecs` confere se cada input declarado tem tipo conhecido e se o `default`
   bate com esse tipo.
3. `ValidateInputValues` pode ser usada separadamente para conferir valores recebidos no momento
   da entrada de dados, reaproveitando o mesmo sistema de tipos.
4. `validateWorkflowScope` monta o conjunto de nós visíveis no escopo atual, valida IDs duplicados,
   resolve referências entre nós e desce recursivamente para nós `map`.
5. `validateNode` aplica as regras por tipo:
   - `agent` exige `prompt`, aceita `permission` e usa `provider` com fallback para `codex`;
   - `bash` exige `command`;
   - `transform` exige `operation`;
   - `noop` não precisa de configuração adicional;
   - `map` exige nós internos.
6. `BuildPlan` complementa a validação com ordenação topológica, detecção de ciclos e montagem
   de `go_to_if` como salto condicional.

A checagem de referências é feita sobre campos textuais como `when`, `go_to_if.when`, `for_each`,
`prompt`, `system`, `command`, `working_dir`, `input` e valores de `env`. Quando a expressão cita
`nodes.<id>.output` ou `nodes.<id>.outputs`, a validação compara isso com o estado real do nó:

- nó expandido com `for_each` deve ser acessado por `outputs`;
- nó não expandido deve ser acessado por `output`.

## Arquivos envolvidos

- `internal/core/workflow/spec.go`: define `WorkflowSpec`, `NodeSpec`, os tipos de nós e os campos
  que entram na validação.
- `internal/core/workflow/validation.go`: concentra as regras de validação estrutural, de tipos,
  de referências e o helper `ValidateInputValues`.
- `internal/core/workflow/validation_test.go`: cobre os principais cenários de erro e de aceite,
  como tipos inválidos, permissões, referências e escopo de mapa.
- `internal/core/workflow/plan.go`: monta o plano de execução, detecta ciclos e valida saltos
  condicionais.
- `internal/core/workflow/plan_test.go`: garante o comportamento do planner em ordenação, mapas
  aninhados e `go_to_if`.

## Observações

- A validação trata `map` como um escopo aninhado, mas mantém o controle de visibilidade dos nós
  para evitar referências cruzadas ambíguas.
- `go_to_if.target` não pode apontar para um nó futuro; o salto precisa permanecer no mesmo ponto
  de execução ou voltar para um nó anterior.
- `permission` só é aceito em nós `agent`, e `permission.write` precisa estar explicitamente definido
  quando o bloco existe.
- O `provider` do nó `agent` é validado quando o backend de providers é informado. O conjunto padrão
  aceita `codex` e `claude`; caso não haja valor explícito, o domínio usa `codex` como padrão.
- `ValidateInputValues` é útil para validar payloads de entrada sem precisar repetir a lógica de tipo
  do spec.
