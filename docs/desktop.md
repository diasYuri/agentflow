# Agentflow Desktop

Aplicacao desktop do Agentflow construida com [Wails v3](https://v3.wails.io/).

## Estrutura

- `cmd/agentflow-desktop/` — ponto de entrada Go do app Wails.
- `internal/desktop/binding/` — servicos Go expostos ao frontend via bindings.
- `internal/desktop/adapter/` — compatibilizacao entre UI e API do Agentflow.
- `internal/desktop/runtime/` — gerenciamento de runs no processo desktop.
- `frontend/desktop/` — frontend React + TypeScript + Vite.
- `assets.go` — embed do diretorio `frontend/desktop/dist` para distribuicao.

## Dependencias

- Go 1.24+
- Node.js + npm
- Wails v3 CLI (opcional, para gerar bindings e dev mode nativo)

```bash
# Instalar Wails v3 CLI (opcional)
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

## Comandos de desenvolvimento

### Frontend (standalone)

```bash
cd frontend/desktop
npm install
npm run dev       # servidor Vite em http://127.0.0.1:9245
npm run build     # build de producao em frontend/desktop/dist
npm test          # testes de componentes com vitest
```

### Desktop (Go + frontend embutido)

```bash
# Build do frontend antes de compilar o Go
cd frontend/desktop && npm run build

# Compilar o app desktop
go build ./cmd/agentflow-desktop

# Executar
./agentflow-desktop
```

### Modo dev com Wails v3

Se o CLI `wails3` estiver instalado:

```bash
# Gera bindings, sobe Vite em background e compila Go com hot-reload
wails3 dev

# Build de producao completo
wails3 build
```

## Binding Go <-> Frontend

O servico `DesktopService` em `internal/desktop/binding/service.go` expoe metodos ao frontend. O Wails v3 gera automaticamente os stubs TypeScript em `frontend/desktop/bindings/`.

Para regenerar os bindings manualmente:

```bash
wails3 generate bindings
```

Metodos expostos:

- `ListWorkflows`, `LoadWorkflow`
- `ValidateWorkflow`, `GenerateGraph`, `DryRunWorkflow`
- `RunWorkflow`, `CancelRun`, `ListRuns`, `GetRun`
- `GetRunEvents`, `GetRunArtifacts`, `GetRunArtifact`
- `GetRunNodes`, `GetRunNode`, `GetRunPlan`, `GetRunLogs`
- `ResolveInput`, `SaveWorkflow`, `SaveInput`
- `GetAppSettings`, `UpdateAppSettings`, `OpenPath`

## Testes

```bash
# Testes Go (incluem adapter, binding e runtime desktop)
go test ./...

# Testes frontend
cd frontend/desktop && npm test

# Compilar sem rodar
go build ./cmd/agentflow
go build ./cmd/agentflow-desktop
```

## QA manual sugerido

1. Abrir o app desktop ou usar a CLI para validar um workflow:
   ```bash
   go run ./cmd/agentflow validate samples/workflows/local-health-check.yaml
   ```
2. No desktop: abrir `samples/workflows/local-health-check.yaml`, editar, salvar.
3. Clicar em Validate, Graph, Dry-run.
4. Executar Run e observar eventos/timeline/logs/artefatos.
5. Cancelar uma run ativa e verificar que o status muda para `cancelled`.
6. Verificar Settings (tema, paths de agentes).

## Notas e limitacoes conhecidas

- O frontend consome exclusivamente o binding/adapter do desktop; nao importa pacotes internos do core.
- O adapter traduz chamadas da UI para a API existente do Agentflow, normalizando erros em `DesktopError`.
- O runtime desktop reutiliza o mesmo armazenamento de runs da CLI (`~/.agentflow/runs` ou `.agentflow/runs`).
- O empacotamento Wails requer o diretorio `frontend/desktop/dist`; o `assets.go` faz embed via `//go:embed all:frontend/desktop/dist`.
- Preview de artefatos binarios no desktop e limitado; arquivos textuais sao decodificados de base64 para exibicao.
- Cancelamento de runs e cooperative: a run interrompe no proximo ponto de checagem de contexto.
