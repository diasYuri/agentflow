# Agentswarm Samples

Exemplos de workflows YAML para casos reais de agent coding local.

Os exemplos foram escritos para demonstrar a ferramenta sem depender de UI ou servidor. Workflows com `kind: agent` usam `codex` por padrão, mas podem declarar `provider: claude`. Use o CLI correspondente no `PATH` ou informe `--codex-path` / `--claude-path`.

## Preparar

```bash
go run ./cmd/agentflow validate samples/workflows/fix-github-issue.yaml
go run ./cmd/agentflow graph samples/workflows/review-changed-files.yaml --format mermaid
go run ./cmd/agentflow dry-run samples/workflows/review-changed-files.yaml --input-json samples/inputs/review-files.json
go run ./cmd/agentflow validate samples/workflows/claude-code-review.yaml
```

Para executar um workflow com agentes:

```bash
mkdir -p .agentflow/workflows
cp samples/workflows/fix-github-issue.yaml .agentflow/workflows/fix-github-issue.yaml
go run ./cmd/agentflow run fix-github-issue \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)"
```

Para validar e executar o sample mínimo com Claude Code:

```bash
go run ./cmd/agentflow validate samples/workflows/claude-code-review.yaml
go run ./cmd/agentflow graph samples/workflows/claude-code-review.yaml --format mermaid
go run ./cmd/agentflow run samples/workflows/claude-code-review.yaml \
  --claude-path "$(which claude)" \
  -it
```

Os workflows são resolvidos primeiro em `./.agentflow/workflows` e depois em `~/.agentflow/workflows`. Os runs são sempre persistidos em `~/.agentflow/runs`.

## Exemplos

- `fix-github-issue.yaml`: analisa uma issue, implementa correção, roda testes e aciona um agente para corrigir falhas.
- `review-changed-files.yaml`: divide arquivos alterados em chunks, faz review paralelo e consolida findings.
- `test-failure-debugging.yaml`: reproduz falha de teste, pede diagnóstico e tenta correção.
- `security-review.yaml`: faz auditoria defensiva por áreas do repositório e consolida riscos.
- `release-notes.yaml`: gera release notes a partir de commits/PRs fornecidos por input.
- `product-spec-to-implementation.yaml`: lê uma spec `.md` e demonstra `kind: map` aninhado para iterar por spec técnica e por plano, implementando cada plano no workspace.
- `local-health-check.yaml`: exemplo executável sem Codex; roda comandos locais e resume outputs.
- `claude-code-review.yaml`: exemplo mínimo com `provider: claude`, permissão somente leitura e resposta estruturada.
- `pause-on-failure.yaml`: demonstra `execution.pause_when_fail`; o run pausa quando o node `gate` falha (arquivo de flag ausente) e pode ser retomado com `workflow resume <id>` depois de criar o arquivo.

### Fluxo de pause/resume

```bash
mkdir -p .agentflow/workflows
cp samples/workflows/pause-on-failure.yaml .agentflow/workflows/pause-on-failure.yaml
go run ./cmd/agentflow daemon start
go run ./cmd/agentflow workflow run pause-on-failure --input flag_file=/tmp/agentflow-resume.flag
# o run pausa porque /tmp/agentflow-resume.flag não existe
touch /tmp/agentflow-resume.flag
go run ./cmd/agentflow workflow status <run-id>
go run ./cmd/agentflow workflow resume <run-id>
```

## Segurança

Os samples executam comandos locais quando usados com `run`. Leia cada `command` antes de executar. Use `dry-run` e `graph` para auditar o plano antes de rodar.
