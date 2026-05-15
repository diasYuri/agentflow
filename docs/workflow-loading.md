# Descoberta de workflows

## Objetivo

Esta feature define como o `agentflow` encontra workflows YAML quando o usuário informa apenas o nome do workflow.
A resolução passa a respeitar dois escopos, com prioridade para o projeto atual, e o carregamento devolve também o
caminho do arquivo que originou o workflow carregado.

## Como funciona

O carregamento segue a ordem abaixo:

1. Procura em `./.agentflow/workflows` do projeto atual.
2. Se não encontrar, procura em `~/.agentflow/workflows`.
3. Em cada escopo, considera apenas arquivos com extensão `.yaml` ou `.yml`.
4. Cada arquivo é decodificado com `yaml.Decoder.KnownFields(true)`, então campos desconhecidos são rejeitados.
5. O `ref` recebido pelo loader é tratado como nome do workflow, não como caminho livre.

O loader percorre os arquivos do diretório, lê o YAML de cada um e compara o campo `name` com o `ref` solicitado.
Quando encontra uma correspondência, retorna:

- a especificação carregada;
- o caminho absoluto ou resolvido do arquivo de origem;
- erro, caso exista qualquer problema de leitura, decode, duplicidade ou ausência do workflow.

Se houver mais de um arquivo com o mesmo `name` dentro do mesmo escopo, o carregamento falha com erro de duplicidade.
Se o workflow existir nos dois escopos, o arquivo local vence e o global não é consultado para esse nome.

Quando o YAML não declara `inputs` ou `vars`, o loader inicializa esses campos com mapas vazios. Isso evita `nil`
em etapas posteriores de validação e execução.

## Arquivos envolvidos

- [`internal/adapters/yaml/loader.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/yaml/loader.go): implementa a busca por escopo, o decode estrito do YAML e a devolução do caminho de origem.
- [`internal/adapters/yaml/loader_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/yaml/loader_test.go): cobre prioridade local, fallback global, duplicidade e erro de não encontrado.
- [`samples/README.md`](/Users/yuri/git/diasYuri/agentflow/samples/README.md): registra a convenção de uso dos workflows de exemplo e a ordem de resolução entre os diretórios local e global.

## Observações relevantes

- A busca acontece apenas no nível raiz de cada diretório de workflows; subdiretórios não participam da resolução.
- O uso de `KnownFields(true)` reduz ambiguidades e falhas silenciosas ao rejeitar chaves inesperadas no YAML.
- O retorno do caminho de origem é importante para auditoria, debug e mensagens de erro mais precisas.
- A prioridade local primeiro, global depois, permite que o projeto sobrescreva um workflow equivalente do usuário.
