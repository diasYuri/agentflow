# Eventos e logs

## Objetivo

Esta feature centraliza a emissão de eventos da execução e a entrega desses eventos para múltiplos destinos.
O objetivo é ter um canal único de observabilidade para o runtime, com três usos principais:

- registrar o ciclo de vida do run com `run_id`, `node_id`, `instance_id`, `attempt` e `path`;
- imprimir eventos como texto legível ou JSON no `stdout`;
- persistir uma trilha em `events.jsonl` dentro de cada execução quando o sink de arquivo estiver ativo.

Além disso, a mesma abstração precisa ser fácil de testar, então o runtime mantém um sink em memória que guarda os eventos emitidos durante o run.

## Como funciona

A emissão de eventos nasce no runtime e é propagada por um conjunto pequeno de sinks.
O ponto de entrada da execução é [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go), onde o executor monta eventos com contexto suficiente para depuração e auditoria.

### Estrutura do evento

O tipo base usado pelo runtime fica em [`internal/core/run/types.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/run/types.go).
Cada evento carrega:

- `ts`: timestamp do evento;
- `run_id`: identificador da execução;
- `type`: tipo do evento, como `run.started`, `node.ready` ou `node.completed`;
- `node_id`: nó associado ao evento, quando aplicável;
- `instance_id`: identificador da instância em expansões e fan-outs;
- `path`: caminho lógico da execução, útil para nós aninhados;
- `attempt`: tentativa atual em casos de retry;
- `data`: payload livre com detalhes extras, como warnings ou metadados de retry.

O executor preenche o `run_id` no momento do `emit`, e `emitState` preserva o `path` corrente do estado de execução.
Isso faz com que eventos de nós aninhados carreguem o caminho correto sem duplicar lógica no dispatcher de cada tipo de nó.

### Emissão durante a execução

Ainda em [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go), o executor:

- emite `run.created` e `run.started` no início do run;
- emite `node.ready` antes de disparar um nó elegível;
- emite `node.started` ou `node.instance.started` ao iniciar uma execução;
- emite `node.completed` ou `node.failed` ao encerrar a tentativa;
- emite `node.retrying` quando há nova tentativa;
- emite `node.expanded` ao expandir `for_each` ou `map`;
- emite `node.skipped` quando a execução pula um nó;
- emite `run.completed` ou `run.failed` no encerramento.

Em nós de bash, o runtime ainda produz `node.bash.warning` com uma advertência sobre execução local de comandos, além de detalhes como `shell`, `working_dir` e `command`.

### Encadeamento de sinks

O encadeamento fica em [`internal/adapters/events/multi/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/multi/sink.go).
Esse sink recebe uma lista de destinos e faz fan-out do mesmo evento para todos eles.
Se algum destino falhar ao emitir, o erro é devolvido imediatamente.
No fechamento, o sink tenta encerrar todos os destinos e retorna o primeiro erro encontrado.

Um detalhe importante é o suporte opcional a `Open(string) error`.
Quando o executor cria um run com diretório local, ele tenta abrir `events.jsonl` no diretório do run.
O sink múltiplo repassa esse `Open` apenas para os sinks que sabem lidar com arquivo, o que permite combinar `stdout`, JSONL e memória sem acoplamento extra.

### Sink de `stdout`

O comportamento de console está em [`internal/adapters/events/stdout/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/stdout/sink.go).
Ele suporta dois formatos:

- `text`: imprime uma linha compacta com timestamp, nó e tipo do evento;
- `json`: serializa o evento inteiro como JSON e escreve em uma linha.

No formato textual, eventos com `warning` no `data` aparecem com a mensagem em destaque, o que ajuda a notar avisos operacionais sem precisar abrir arquivos do run.

### Sink JSONL

O arquivo [`internal/adapters/events/jsonl/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/jsonl/sink.go) grava cada evento como uma linha JSON em `events.jsonl`.
O sink é seguro para uso concorrente por causa do `Mutex`, e abre o arquivo com append, então múltiplos eventos do mesmo run são acumulados no mesmo destino.

Quando o caminho informado é vazio, o sink se comporta como inerte:

- `New("")` retorna um sink sem arquivo;
- `Emit` vira no-op;
- `Close` não faz nada.

Isso facilita testes e também permite montar a infraestrutura sem obrigar escrita em disco.

### Sink de memória

O sink de memória está em [`internal/adapters/events/memory/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/memory/sink.go).
Ele acumula os eventos emitidos em `Events []run.Event`, protegidos por mutex.
O objetivo é dar suporte a testes que precisam inspecionar a sequência, o conteúdo ou o mascaramento dos eventos sem depender do filesystem.

## Arquivos principais

- [`internal/core/runtime/handlers/state.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/handlers/state.go): constrói e emite eventos durante a execução, injeta `run_id` e propaga `path`.
- [`internal/adapters/events/stdout/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/stdout/sink.go): imprime eventos em texto ou JSON no `stdout`.
- [`internal/adapters/events/jsonl/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/jsonl/sink.go): grava `events.jsonl` por execução quando o sink de arquivo está ativo.
- [`internal/adapters/events/multi/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/multi/sink.go): encadeia múltiplos sinks e repassa `Open`, `Emit` e `Close`.
- [`internal/adapters/events/memory/sink.go`](/Users/yuri/git/diasYuri/agentflow/internal/adapters/events/memory/sink.go): armazena eventos em memória para testes.
- [`internal/core/runtime/run_workflow_test.go`](/Users/yuri/git/diasYuri/agentflow/internal/core/runtime/run_workflow_test.go): cobre a integração dos sinks, a persistência de `events.jsonl` e a inspeção de eventos em memória.

## Observações relevantes

- O `path` do evento é especialmente útil para fan-out e nós aninhados, porque preserva o contexto lógico da execução sem depender apenas de `node_id`.
- A persistência em `events.jsonl` acontece por run, no diretório criado pelo storage local da execução.
- O sink de console e o sink JSONL são independentes; o mesmo evento pode ser exibido em tempo real e salvo para auditoria.
- O sink de memória é a opção preferida em testes que precisam verificar ordem, contagem ou conteúdo dos eventos.
- O desenho do multi-sink evita acoplamento entre observabilidade em tempo real e persistência de artefatos do run.
