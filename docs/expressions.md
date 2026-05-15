# Templates e expressões

## Objetivo

Esta feature padroniza a avaliação de expressões dentro do workflow para permitir interpolação
dinâmica em campos que fazem parte da execução. O objetivo é manter o YAML declarativo e, ao
mesmo tempo, permitir que o fluxo resolva valores, condições e referências sem precisar de lógica
externa para montar strings ou consultar estado de execução.

O motor cobre interpolação com `${...}` usando `expr-lang`, preserva valores tipados quando o campo
inteiro é uma expressão e expõe um contexto consistente para inputs, variáveis, segredos, nodes e
metadados do run.

## Como funciona

O núcleo da feature está em [`internal/core/workflow/expr.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/expr.go).
Esse pacote usa `expr-lang` para compilar e executar expressões com variáveis expostas no ambiente.

O comportamento principal segue estas regras:

1. Qualquer trecho no formato `${...}` é avaliado durante a renderização.
2. Quando a string inteira é uma única expressão, o valor é preservado com seu tipo original.
3. Quando a expressão aparece dentro de texto maior, o resultado é convertido para string e interpolado.
4. Expressões booleanas usadas em campos condicionais são avaliadas como `bool`.
5. O motor permite referências a variáveis não declaradas, o que facilita acessar campos opcionais do contexto.

O contexto de avaliação expõe estes objetos:

- `inputs`: entradas resolvidas do workflow.
- `vars`: variáveis do workflow e overrides aplicados em runtime.
- `secrets`: segredos carregados do ambiente.
- `nodes`: estado consolidado dos nodes já executados.
- `item`: item atual em uma iteração de `for_each`.
- `index`: índice do item atual, quando aplicável.
- `total`: total de itens na iteração, quando aplicável.
- `run`: metadados do run corrente, como `id` e `workflow`.

Também ficam disponíveis helpers embutidos:

- `exists(v)`: retorna `true` quando o valor não é `nil`.
- `success(id)`: verifica se o node informado terminou com status `success`.
- `failed(id)`: verifica se o node informado terminou com status `failed`.
- `contains(container, needle)`: verifica presença em strings e coleções.
- `len(v)`: retorna o tamanho de strings, slices, arrays e mapas.

Na prática, isso permite escrever workflows como os exemplos em
[`samples/workflows/fix-github-issue.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml)
e
[`samples/workflows/test-failure-debugging.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/test-failure-debugging.yaml),
onde entradas, saídas de nodes e resultados de comandos são combinados em expressões curtas e
legíveis.

## Arquivos principais

- [`internal/core/workflow/expr.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/expr.go): implementa a compilação, a interpolação `${...}`, a avaliação booleana e os helpers do contexto.
- [`internal/core/workflow/expr_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/expr_test.go): cobre preservação de tipos em expressões completas e uso de referências a nodes em condições booleanas.
- [`samples/workflows/fix-github-issue.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml): mostra interpolação de inputs, leitura de saída de nodes e uso de `success`/`failed` no fluxo de correção.
- [`samples/workflows/test-failure-debugging.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/test-failure-debugging.yaml): mostra uso de expressões em condições, diagnóstico baseado em saída de testes e decisões condicionais entre etapas.

## Observações relevantes

- Quando a expressão ocupa a string inteira, o resultado pode ser `[]any`, `map[string]any`, `bool`, `int` ou outro tipo nativo, sem conversão para texto.
- Quando a expressão aparece dentro de um texto maior, o motor sempre serializa o valor para string antes de interpolar.
- O helper `contains` compara itens com `reflect.DeepEqual` quando a entrada é uma coleção.
- O helper `len` devolve `0` para valores `nil` ou tipos que não sejam coleções e strings.
- `nodes` contém o estado consolidado dos nodes já executados, o que permite consultar status, saída, `stdout`, `stderr`, `exit_code` e `error`.
- `success(id)` e `failed(id)` dependem do status consolidado do node, então o valor refletido no contexto precisa estar atualizado pela execução anterior.
- A avaliação com `EvalBool` mantém compatibilidade com expressões compostas, como combinações entre referências de nodes e operadores lógicos.
