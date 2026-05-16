# Workflows de exemplo

## Objetivo

Esta página documenta os workflows de exemplo que vivem em `samples/` e funciona como referência viva dos padrões suportados pela ferramenta. O conjunto cobre cenários comuns de uso local e mostra, em YAML real, como combinar nós `bash`, `agent`, `transform`, `map`, `for_each`, `output_schema`, `retries` e `continue_on_error`.

Os exemplos são pensados para leitura, validação e execução local. Eles não dependem de UI nem de infraestrutura externa além do CLI do provider escolhido quando o workflow usa nós `agent`.

## Como funciona

Os workflows ficam em [`/Users/yuri/git/diasYuri/agentflow/samples/workflows`](</Users/yuri/git/diasYuri/agentflow/samples/workflows>) e os inputs prontos ficam em [`/Users/yuri/git/diasYuri/agentflow/samples/inputs`](</Users/yuri/git/diasYuri/agentflow/samples/inputs>). A orientação de uso complementar está em [`/Users/yuri/git/diasYuri/agentflow/samples/README.md`](</Users/yuri/git/diasYuri/agentflow/samples/README.md>).

O fluxo recomendado é:

1. validar o workflow com `go run ./cmd/agentflow validate <workflow.yaml>`;
2. inspecionar o grafo com `go run ./cmd/agentflow graph <workflow.yaml>`;
3. simular a execução com `go run ./cmd/agentflow dry-run <workflow.yaml> --input-json <file>`;
4. executar de fato com `go run ./cmd/agentflow run <workflow.yaml> --input-json <file>`.

Os exemplos com `kind: agent` usam `codex` por padrão e podem optar por `provider: claude` ou `provider: pi`. Workflows Codex exigem o Codex CLI no `PATH` ou `--codex-path`; workflows Claude exigem Claude Code CLI no `PATH`, `CLAUDE_PATH`, `AGENTFLOW_CLAUDE_PATH` ou `--claude-path`; workflows Pi exigem Pi no `PATH`, `PI_PATH`, `AGENTFLOW_PI_PATH` ou `--pi-path`. O workflow `local-health-check` roda apenas comandos locais, sem depender de agente.

## Workflows incluídos

- `local-health-check.yaml`: checagem rápida do ambiente local com `bash`, captura de saída, `continue_on_error` e desvio condicional para resumir falhas.
- `fix-github-issue.yaml`: exemplo de correção guiada por issue, combinando análise com `agent`, implementação com `retries`, teste local com `bash` e resumo final.
- `review-changed-files.yaml`: revisão paralela de arquivos alterados com `transform` para dividir lotes, `for_each` para fan-out e `merge` para consolidar findings.
- `security-review.yaml`: auditoria defensiva por áreas do repositório, também baseada em `for_each` e consolidação posterior.
- `release-notes.yaml`: geração de release notes com `transform`, `output_schema` e validação em dois passos para manter o JSON consistente.
- `product-spec-to-implementation.yaml`: leitura de uma spec em Markdown, extração de specs técnicas, `map` aninhado para quebrar e implementar planos, `output_schema` e `continue_on_error` para preservar o avanço do conjunto.
- `test-failure-debugging.yaml`: reprodução de falha de teste, diagnóstico condicionado por `when`, tentativa de correção com `retries` e verificação final.
- `claude-code-review.yaml`: sample mínimo com `provider: claude`, permissão somente leitura e `output_schema` pequeno.
- `pi-code-review.yaml`: sample mínimo com `provider: pi`, permissão somente leitura e `output_schema` pequeno.

## Arquivos principais

- [`/Users/yuri/git/diasYuri/agentflow/samples/README.md`](</Users/yuri/git/diasYuri/agentflow/samples/README.md>): guia de execução dos samples, com comandos prontos e observações sobre resolução de workflows e persistência de runs.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/local-health-check.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/local-health-check.yaml>): fluxo mínimo para validar o comportamento de `bash` e condicionais locais.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/fix-github-issue.yaml>): workflow de ponta a ponta para issue -> análise -> implementação -> teste -> resumo.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/review-changed-files.yaml>): exemplo de fan-out paralelo para code review.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/security-review.yaml>): exemplo de revisão de segurança orientada por áreas.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/release-notes.yaml>): exemplo centrado em `output_schema` para saída estruturada.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/product-spec-to-implementation.yaml>): demonstração mais completa de `map`, `transform` e execução paralela em múltiplos níveis.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/test-failure-debugging.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/test-failure-debugging.yaml>): fluxo focado em troubleshooting de testes.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/claude-code-review.yaml>): fluxo mínimo para validar e executar Claude Code como provider.
- [`/Users/yuri/git/diasYuri/agentflow/samples/workflows/pi-code-review.yaml`](</Users/yuri/git/diasYuri/agentflow/samples/workflows/pi-code-review.yaml>): fluxo mínimo para validar e executar Pi RPC como provider.
- [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/fix-issue.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/fix-issue.json>), [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/review-files.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/review-files.json>), [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/security-review.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/security-review.json>), [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/release-notes.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/release-notes.json>), [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/product-spec-to-implementation.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/product-spec-to-implementation.json>) e [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/test-failure.json`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/test-failure.json>): payloads prontos para rodar os exemplos sem montar JSON manualmente.
- [`/Users/yuri/git/diasYuri/agentflow/samples/inputs/product-spec.md`](</Users/yuri/git/diasYuri/agentflow/samples/inputs/product-spec.md>): spec de produto usada pelo exemplo de spec-to-implementation.

## Observações relevantes

- Os workflows são documentação executável: prefira começar por `validate` e `dry-run` antes de `run`, principalmente em exemplos com `bash` e `agent`.
- `continue_on_error` aparece nos samples para manter o fluxo observável mesmo quando uma etapa falha; isso é útil para relatórios, diagnósticos e retries posteriores.
- `for_each` e `map` geram fan-out; os exemplos mostram tanto revisão paralela simples quanto um pipeline aninhado com escopo próprio.
- `output_schema` é usado para forçar saída estruturada de agentes e reduzir ambiguidade em etapas que precisam produzir JSON estável.
- `retries` aparece nos pontos em que a correção pode falhar de forma transitória e vale a pena tentar novamente com o contexto já coletado.
- Os arquivos em `samples/inputs` foram preparados para copiar e colar em `--input-json`, o que facilita testes repetíveis e documentação viva dos padrões do DSL.
- `samples/README.md` é o ponto de entrada mais prático para quem quer executar os exemplos; este documento complementa aquela visão com o mapa conceitual dos padrões cobertos.
