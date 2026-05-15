# Transformações de dados

## Objetivo

Esta feature adiciona transformações utilitárias para preparar, reorganizar e consolidar dados entre nodes sem exigir lógica externa.
O foco é cobrir tarefas recorrentes de workflow, como dividir revisões em partes, achatar respostas agregadas, extrair campos de JSON
e converter entre texto JSON e estruturas nativas.

As operações disponíveis são:

- `chunk`: divide arrays/slices ou strings em partes equilibradas.
- `merge`: achata uma lista de itens em um único nível.
- `flat_map`: navega em cada item, extrai um valor opcional e concatena resultados aninhados.
- `json_parse`: converte JSON textual em estrutura nativa.
- `json_stringify`: serializa valores nativos para JSON.
- `pick`: acessa campos aninhados por caminho, incluindo índices de array.

## Como funciona

A implementação central está em [`internal/core/workflow/transform.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/transform.go).
O ponto de entrada é `ApplyTransform(operation, input, with)`, que roteia a operação pedida e devolve o valor transformado ou um erro
quando a operação, o tipo de entrada ou os argumentos em `with` são inválidos.

O comportamento de cada operação é este:

- `chunk`
  - aceita arrays/slices e strings;
  - exige `with.chunks` maior que zero;
  - divide os itens em blocos equilibrados, calculando o tamanho de cada bloco a partir do total de elementos;
  - quando a entrada é string, a divisão respeita runes, não bytes.
- `merge`
  - converte a entrada para slice quando possível;
  - achata apenas um nível de profundidade;
  - itens que não são slices são preservados como estão;
  - se a entrada não for uma coleção, o valor original é mantido dentro de uma lista.
- `flat_map`
  - exige uma entrada que possa ser lida como array/slice;
  - quando `with.path` é informado, cada item é navegado antes do achatamento;
  - se o valor encontrado também for um array/slice, seus itens são concatenados no resultado;
  - valores escalares são mantidos.
- `json_parse`
  - aceita apenas string como entrada;
  - usa `json.Unmarshal` para produzir `any` com tipos nativos do Go.
- `json_stringify`
  - serializa qualquer valor aceito por `json.Marshal`;
  - retorna uma string JSON compacta.
- `pick`
  - lê `with.path` como caminho separado por ponto;
  - navega por mapas (`map[string]any`) e arrays (`[]any`);
  - aceita índices numéricos em paths como `items.0.name`;
  - retorna erro quando o caminho passa por um tipo incompatível ou um índice de array é inválido.

Na prática, isso permite separar revisões em lotes, agregar saídas de agentes, extrair listas internas de JSON
e preparar payloads para o próximo node do workflow.

Os workflows de exemplo que usam essas transformações estão em
[`samples/workflows/review-changed-files.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml),
[`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml) e
[`samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml).

## Arquivos principais

- [`internal/core/workflow/transform.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/transform.go): implementa o roteamento das operações e a lógica de transformação.
- [`internal/core/workflow/transform_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/workflow/transform_test.go): cobre os cenários de `flat_map` com arrays aninhados e com `path`.
- [`samples/workflows/review-changed-files.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml): usa `chunk` para repartir arquivos e `merge` para consolidar reviews.
- [`samples/workflows/release-notes.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml): usa `json_stringify` para preparar uma lista de mudanças como texto.
- [`samples/workflows/product-spec-to-implementation.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml): usa `pick` para extrair `technical_specs` e `flat_map` para consolidar saídas de implementação.

## Observações relevantes

- `chunk` valida `with.chunks > 0`; valores inválidos retornam erro.
- `chunk` em strings trabalha com runes, então caracteres multibyte não são quebrados no meio.
- `merge` é propositalmente superficial: ele achata apenas um nível.
- `json_parse` só aceita string; qualquer outro tipo falha antes da desserialização.
- `pick` navega apenas por mapas e arrays/slices, então não é um seletor genérico para structs arbitrários.
- Paths como `items.0.name` são suportados para atravessar estruturas aninhadas.
- As transforms foram desenhadas para compor etapas de workflow, então a saída de uma operação normalmente alimenta diretamente o próximo node.
