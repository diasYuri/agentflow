# Agentswarm Samples

Exemplos de workflows YAML para casos reais de agent coding local.

Os exemplos foram escritos para demonstrar a ferramenta sem depender de UI ou servidor. Workflows com `kind: agent` usam o provider `codex` e exigem o Codex CLI disponível no `PATH` ou via `--codex-path`.

## Preparar

```bash
go run ./cmd/agentflow validate samples/workflows/fix-github-issue.yaml
go run ./cmd/agentflow graph samples/workflows/review-changed-files.yaml --format mermaid
go run ./cmd/agentflow dry-run samples/workflows/review-changed-files.yaml --input-json samples/inputs/review-files.json
```

Para executar um workflow com agentes:

```bash
mkdir -p agentflow/workflows
cp samples/workflows/fix-github-issue.yaml agentflow/workflows/fix-github-issue.yaml
go run ./cmd/agentflow run fix-github-issue \
  --input-json samples/inputs/fix-issue.json \
  --codex-path "$(which codex)"
```

Os workflows são resolvidos primeiro em `./agentflow/workflows` e depois em `~/.agentflow/workflows`. Os runs são sempre persistidos em `~/.agentflow/runs`.

## Exemplos

- `fix-github-issue.yaml`: analisa uma issue, implementa correção, roda testes e aciona um agente para corrigir falhas.
- `review-changed-files.yaml`: divide arquivos alterados em chunks, faz review paralelo e consolida findings.
- `test-failure-debugging.yaml`: reproduz falha de teste, pede diagnóstico e tenta correção.
- `security-review.yaml`: faz auditoria defensiva por áreas do repositório e consolida riscos.
- `release-notes.yaml`: gera release notes a partir de commits/PRs fornecidos por input.
- `product-spec-to-implementation.yaml`: lê uma spec `.md` e demonstra `kind: map` aninhado para iterar por spec técnica e por plano, implementando cada plano no workspace.
- `local-health-check.yaml`: exemplo executável sem Codex; roda comandos locais e resume outputs.

## Segurança

Os samples executam comandos locais quando usados com `run`. Leia cada `command` antes de executar. Use `dry-run` e `graph` para auditar o plano antes de rodar.
