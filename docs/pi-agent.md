# Pi Agent

Esta página documenta o provider `pi` para nós `kind: agent`. Ele executa o Pi CLI local em modo RPC, sem sessão persistida, para integrar workflows AgentFlow ao protocolo JSONL do Pi.

Quando `provider` é omitido, o runtime continua usando `codex`. Use `provider: pi` explicitamente no nó que deve chamar Pi:

```yaml
nodes:
  - id: review
    kind: agent
    provider: pi
    permission:
      write: false
    prompt: "Revise o projeto e retorne JSON com findings."
```

Execução:

```bash
go run ./cmd/agentflow validate samples/workflows/pi-code-review.yaml
go run ./cmd/agentflow run samples/workflows/pi-code-review.yaml --pi-path "$(which pi)" -it
```

## Resolução do binário

O adapter resolve o executável nesta ordem:

1. `--pi-path <path>` recebido pela CLI ou pelo daemon;
2. `AGENTFLOW_PI_PATH`;
3. `PI_PATH`;
4. `pi` encontrado no `PATH`;
5. fallback literal `pi`.

Ao iniciar o daemon com `agentflow daemon start --pi-path <path>`, a CLI propaga esse valor como `AGENTFLOW_PI_PATH`.

## Contrato

O provider chama `pi --mode rpc --no-session`, envia um comando `prompt`, consome eventos até `agent_end`, consulta `get_last_assistant_text` para preencher `result.Text` e consulta `get_session_stats` para preencher usage quando houver tokens.

Campos encaminhados:

- `model`: vira `--model`.
- `system`: vira `--append-system-prompt`.
- `working_dir`: vira o diretório do processo `pi`.
- `env`: é mesclado sobre o ambiente do processo.
- `permission.write: false` ou `sandbox.mode: read-only`: restringe ferramentas com `--tools read,grep,find,ls`.

O RPC do Pi não expõe JSON schema nativo nos docs usados. Quando `output_schema` existe, AgentFlow adiciona uma instrução curta para que a resposta final seja somente JSON, valida o payload contra o schema no próprio provider e, se a resposta vier inválida, reutiliza a mesma sessão para pedir correção antes de retornar erro. O `result.JSON` continua vindo do último texto válido do assistant.

## Limites e retenção de eventos

A saída funcional do provider continua sendo `Text`, `JSON` e `Usage`. Eventos brutos do RPC **não são retidos por padrão**:

- `RawEvents` permanece vazio a menos que a captura bruta esteja explicitamente habilitada (modo de debug).
- Cada linha JSONL do RPC é limitada a **1 MiB** (`maxRPCRecordBytes`).
- O texto do assistant é limitado a **10 MiB** (`maxTextBytes`); se o limite for excedido, o provider retorna erro.
- A captura bruta destina-se apenas a diagnóstico e não faz parte do contrato funcional do runtime.
