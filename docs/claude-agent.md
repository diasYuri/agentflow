# Agente Claude Code

## Objetivo

Esta página documenta o provider `claude` para nós `kind: agent`. Ele executa o Claude Code CLI local em modo não interativo e permite usar workflows AgentFlow com Claude sem mudar o provider padrão do projeto.

Quando `provider` é omitido, o runtime continua usando `codex`. Use `provider: claude` explicitamente no nó que deve chamar Claude Code.

## Uso mínimo

```yaml
nodes:
  - id: review
    kind: agent
    provider: claude
    permission:
      write: false
    prompt: |
      Revise o estado do projeto e retorne um resumo curto.
```

Validação e execução:

```bash
go run ./cmd/agentflow validate samples/workflows/claude-code-review.yaml
go run ./cmd/agentflow run samples/workflows/claude-code-review.yaml --claude-path "$(which claude)" -it
```

## CLI e ambiente

O adapter chama o Claude Code CLI com `-p --output-format json --no-session-persistence`. O `prompt` é enviado pelo stdin do processo (não como argumento), para evitar que flags variádicas como `--tools` absorvam o prompt e para suportar prompts longos e com várias linhas sem se preocupar com escaping.

O caminho do binário é resolvido nesta ordem:

1. `--claude-path <path>` recebido pela CLI ou pelo daemon;
2. `AGENTFLOW_CLAUDE_PATH`, usado para configurar o daemon;
3. `CLAUDE_PATH`, lido pelo adapter no processo de execução;
4. `claude` encontrado no `PATH`.

O ambiente do processo é preservado e mesclado com `env` do nó. Variáveis do próprio Claude Code, como `ANTHROPIC_MODEL`, continuam disponíveis para o CLI. Se o nó definir `model`, o adapter envia `--model <valor>` e esse valor tem prioridade para aquela chamada.

`--no-session-persistence` é passado em toda execução, então a sessão fica em memória apenas durante o run e nada é gravado em `~/.claude/projects`. Sessões automatizadas não poluem o estado local nem podem ser resumidas depois.

## Campos suportados

O provider `claude` recebe os mesmos campos de contrato usados por outros agents:

- `model`: encaminhado para `claude --model`.
- `system`: encaminhado como prompt de sistema adicional via `--append-system-prompt`.
- `prompt`: enviado pelo stdin do processo (não vai como argumento).
- `env`: mesclado ao ambiente do processo antes de executar o CLI.
- `working_dir`: usado como diretório de execução do processo `claude`.
- `sandbox` e `permission.write`: convertidos para `--tools` em read-only e `--permission-mode acceptEdits` em workspace-write.
- `output_schema`: serializado e enviado como `--json-schema` ao CLI para orientar saída JSON estruturada.

Quando `output_schema` está presente, o adapter lê `structured_output` do JSON emitido pelo Claude Code e usa esse valor estruturado como saída do nó. Se esse campo não vier no payload, ele tenta interpretar o campo textual como JSON antes de manter apenas o texto bruto.

Quando o payload do Claude Code traz `is_error: true`, o adapter promove a execução a erro do nó (com `subtype` e `result` no texto da exceção) em vez de marcar o nó como sucesso. `permission_denials` (mesmo em runs bem-sucedidos) e `total_cost_usd` são copiados para `result.Metadata` (`permission_denials` e `claude_cost_usd`). O bloco `usage` aparece em `Metadata["claude_usage"]` com todos os campos do Claude, incluindo cache. `Usage.TotalTokens` soma `input + cache_read + cache_creation + output` para refletir o volume real processado.

## Permissões e sandbox

As permissões do Claude Code não são equivalentes ao sandbox do Codex. No Codex, o sandbox controla o modo de acesso do agente ao workspace. No Claude Code, o adapter traduz o sandbox em flags do CLI:

- `permission.write: false` ou `sandbox.mode: read-only` restringe as ferramentas disponíveis com `--tools Read,Glob,Grep,LS`. Como o Claude Code não consegue invocar ferramentas fora desse conjunto, escrita, execução de Bash e edição ficam fora do alcance do agente.
- `permission.write: true` ou `sandbox.mode: workspace-write` envia `--permission-mode acceptEdits`. O Claude Code aceita edições de arquivo sem prompts interativos, mas continua pedindo confirmação para ações realmente perigosas (por exemplo, comandos Bash arbitrários). O conjunto de ferramentas permanece o default do CLI.
- Modos não reconhecidos não adicionam flags, deixando o comportamento para a configuração local do Claude Code.

Por isso, revise workflows com `provider: claude` do mesmo modo que revisaria comandos locais: confira `prompt`, `env`, `working_dir`, permissões e qualquer dependência de credenciais antes de executar.

## Exemplo estruturado

[`samples/workflows/claude-code-review.yaml`](/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml) é o sample mínimo recomendado. Ele usa `provider: claude`, `permission.write: false` e `output_schema` pequeno para demonstrar validação, grafo e execução sem reescrever os demais samples do projeto.
