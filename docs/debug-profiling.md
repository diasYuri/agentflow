# Debug e Profiling do Daemon

O `agentflowd` expõe endpoints de profiling e métricas de memória quando habilitado via variável de ambiente. Use esses recursos para diagnosticar picos de heap, goroutines acumuladas e gargalos de CPU antes de implementar otimizações.

## Habilitar modo debug

```bash
AGENTFLOWD_DEBUG=1 agentflow daemon start
```

Quando ativo, o daemon:

- Expõe endpoints `pprof` no socket Unix local.
- Configura o logger para nível `debug`, emitindo métricas de memória antes e depois de operações críticas.
- Nunca expõe profiling via TCP — só acessível pelo socket local (`~/.agentflow/agentflowd.sock`).

## Endpoints disponíveis

| Endpoint | Descrição |
|----------|-----------|
| `GET /debug/vars` | Snapshot JSON com `runtime.MemStats`, número de goroutines, CPUs e versão do Go. |
| `GET /debug/pprof/heap` | Heap profile — mostra o que está alocado na memória no momento. |
| `GET /debug/pprof/allocs` | Profile de alocações acumuladas desde o início do processo. |
| `GET /debug/pprof/goroutine` | Stack traces de todas as goroutines ativas. |
| `GET /debug/pprof/profile` | CPU profile (padrão 30s). Use `?seconds=N` para ajustar. |
| `GET /debug/pprof/trace` | Execution trace — útil para analisar contenção e scheduling. |

Todos os endpoints seguem o formato padrão do pacote `net/http/pprof` do Go.

## Como capturar e analisar um heap profile

```bash
# 1. Inicie o daemon em modo debug
AGENTFLOWD_DEBUG=1 agentflow daemon start

# 2. Execute o workflow que causa o pico
agentflow run meu-workflow-grande

# 3. Capture o heap profile no momento do pico
curl --unix-socket ~/.agentflow/agentflowd.sock \
  http://localhost/debug/pprof/heap > agentflow-heap.pb.gz

# 4. Analise interativamente
go tool pprof agentflow-heap.pb.gz

# Comandos úteis dentro do pprof:
(pprof) top              # top 10 funções por alocação
(pprof) top -cum         # top 10 por alocação acumulada
(pprof) list executeFanOut
(pprof) list loadProgress
(pprof) list WorkflowArtifact
(pprof) list WorkflowNodes
(pprof) alloc_space      # alterna para visualização por espaço alocado
(pprof) inuse_space      # alterna para espaço ainda em uso
```

## Como capturar um CPU profile

```bash
# Captura durante 30 segundos (padrão)
curl --unix-socket ~/.agentflow/agentflowd.sock \
  http://localhost/debug/pprof/profile > agentflow-cpu.pb.gz

# Com duração customizada
curl --unix-socket ~/.agentflow/agentflowd.sock \
  'http://localhost/debug/pprof/profile?seconds=60' > agentflow-cpu.pb.gz

# Análise
go tool pprof agentflow-cpu.pb.gz
(pprof) top
(pprof) list executeNodes
```

## Como capturar um trace de execução

```bash
curl --unix-socket ~/.agentflow/agentflowd.sock \
  'http://localhost/debug/pprof/trace?seconds=5' > agentflow-trace.out

# Visualize no navegador
go tool trace agentflow-trace.out
```

## Logs de métricas de memória

Quando `AGENTFLOWD_DEBUG=1`, o daemon loga métricas de memória em operações críticas:

```bash
# Acompanhe em tempo real
tail -f ~/.agentflow/agentflowd.log | jq 'select(.msg | contains("MemStats") or contains("Workflow"))'
```

Exemplo de saída:

```json
{
  "time": "2026-05-16T14:23:01Z",
  "level": "DEBUG",
  "msg": "WorkflowArtifact start",
  "run_id": "abc123",
  "artifact_id": "report.json",
  "alloc_mb": 45,
  "total_alloc_mb": 1280,
  "heap_mb": 38,
  "sys_mb": 256,
  "goroutines": 24
}
{
  "time": "2026-05-16T14:23:02Z",
  "level": "DEBUG",
  "msg": "WorkflowArtifact end",
  "run_id": "abc123",
  "artifact_id": "report.json",
  "alloc_mb": 145,
  "total_alloc_mb": 1380,
  "heap_mb": 138,
  "sys_mb": 512,
  "goroutines": 24
}
```

### Campos do log

| Campo | Descrição |
|-------|-----------|
| `alloc_mb` | Memória atualmente alocada e em uso (MB). |
| `total_alloc_mb` | Total acumulado de memória alocada desde o início (MB). |
| `heap_mb` | Memória do heap alocada e em uso (MB). |
| `sys_mb` | Memória total obtida do SO (MB). |
| `goroutines` | Número de goroutines ativas. |

### Operações instrumentadas

- `refreshRun` / `loadProgress` — chamado em toda `WorkflowStatus`, `ListWorkflows` e polling do `watch`.
- `WorkflowArtifact` — carregamento de artefatos em memória (com tamanho do arquivo).
- `WorkflowNodes` — escaneamento de todos os `result.json` de um run.
- `WorkflowLogs` — carregamento completo do `events.jsonl`.
- `WorkflowEvents` — paginação de eventos (com cursor e limit).

## Workflow de diagnóstico recomendado

1. **Estabeleça uma baseline**: capture `/debug/vars` com o daemon ocioso.
2. **Execute o workflow suspeito** e monitore os logs de debug.
3. **Identifique o pico**: observe qual operação mostra o maior salto em `alloc_mb`.
4. **Capture o heap profile** exatamente após a operação problemática.
5. **Analise com `pprof`**: use `top`, `list <função>` e `alloc_space` para encontrar a fonte.
6. **Se necessário, capture CPU profile** para identificar gargalos de processamento.

## Segurança

Os endpoints de debug são registrados **apenas** quando `AGENTFLOWD_DEBUG=1` e são servidos exclusivamente pelo socket Unix local. Não há exposição via TCP, mesmo que o daemon seja exposto de outra forma.

Nunca habilite `AGENTFLOWD_DEBUG=1` em ambientes compartilhados ou de produção sem entender que:

- `pprof/profile` pode conter dados sensíveis da aplicação.
- `pprof/heap` expõe a estrutura de memória do processo.
- Logs de debug são mais verbosos e podem persistir em disco.
