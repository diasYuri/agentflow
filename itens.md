O Que Falta

  1. Persistência real do daemon
     O daemon só mantém runs em memória em inte
     rnal/daemon/manager.go:31. Após restart, w
     orkflow list/status/logs perde conhecimento
     dos runs, mesmo com artefatos em disco. Re
     comendo hidratar o estado a partir de
     run.json/summary.json.
  2. Implementar ou remover --output-dir
     A flag existe, mas está explicitamente
     marcada como ignorada em internal/cli/
     root.go:382. O RunOptions.OutputDir também
     não é aplicado em internal/core/runtime/
     handlers/helpers.go:26. Recomendo
     implementar como RunRoot por execução ou
     remover da CLI para não prometer
     comportamento falso.
  3. Validar tipos de inputs fornecidos
     Existe ValidateInputValues, mas ela não é
     chamada no fluxo de execução em internal/
     core/runtime/run_workflow.go:77. Hoje
     defaults são validados, mas inputs passados
     por CLI/JSON podem entrar com tipo errado.
     Recomendo chamar após ResolveInputs.
  4. Aplicar secrets.required
     SecretSpec.Required existe, mas loadSecrets
     só ignora env ausente em internal/core/
     runtime/handlers/helpers.go:91. Recomendo
     falhar cedo quando um secret obrigatório
     não estiver no ambiente.
  5. Ajustar configuração por run no daemon
     runWorkflowViaDaemon não envia codexPath,
     eventsJSONL, logFormat, outputDir ou opções
     equivalentes ao daemon em internal/cli/
     root.go:410. Para uso real, cada run
     precisa receber configuração suficiente, ou
     a documentação deve deixar claro que isso
     só vale no daemon start/local -it.
  6. Usar ou remover campos do DSL ainda sem
     efeito
     NodeSpec.Tools existe em internal/core/
     workflow/spec.go, mas o AgentRequest não
     carrega tools e o provider Codex não usa
     isso. Recomendo implementar passagem de
     ferramentas/políticas ao provider ou
     retirar do DSL por enquanto.
  7. Melhorar UX de dry-run via daemon
     Run(... DryRun) retorna RunPlanned sem
     RunID/persistência de plano em internal/
     core/runtime/run_workflow.go:60. Via
     daemon, isso vira um registro meio
     artificial. Recomendo manter dry-run sempre
     local, ou persistir plano/resolved inputs
     como artefato de run planejado.
  8. Completar observabilidade
     O projeto já salva events.jsonl, resultados
     e summary, mas faltam filtros/paginação/
     tail para logs e consulta de artefatos.
     Recomendo workflow logs --follow, --json,
     workflow artifacts, e status lendo summary
     final.
