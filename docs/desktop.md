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
- macOS: Xcode Command Line Tools
- Linux: `gcc`, `gtk4` e `webkitgtk-6.0`

Depois da instalacao, rode `wails3 doctor` para validar as dependencias do ambiente.

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

## Distribuicao e Instalacao

### Resumo dos requisitos por canal

- Desenvolvimento local: Go, Node.js, npm, Wails CLI e dependencias nativas da plataforma.
- GitHub Actions: runner da plataforma alvo, Go 1.24+, Node.js, Wails CLI e `task` quando o build usar o fluxo padrao do Wails.
- Homebrew: artefatos versionados e publicados em release, com cask para o app desktop e formula apenas para binario CLI.

### GitHub Actions

O pipeline de distribuicao precisa cobrir pelo menos os alvos que o app realmente publica. Para o desktop, o caminho mais simples e usar matriz por sistema operacional e compilar em runners hospedados pelo GitHub.

Checklist recomendado:

- `actions/checkout` para obter o codigo;
- `actions/setup-go` com a versao suportada pelo projeto;
- `actions/setup-node` para instalar dependencias do frontend;
- instalacao do `wails3` via `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`;
- build do frontend antes do empacotamento final;
- upload do conteudo gerado em `bin/` como artifact;
- em macOS, configurar assinatura e notarizacao quando a distribuicao exigir app assinado.

Exemplo de fluxo:

```yaml
name: desktop-build
on:
  push:
    tags:
      - "v*"
jobs:
  build:
    strategy:
      matrix:
        os: [macos-latest, ubuntu-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
      - run: go install github.com/wailsapp/wails/v3/cmd/wails3@latest
      - name: Install Task
        uses: arduino/setup-task@v2
      - run: wails3 build
      - uses: actions/upload-artifact@v4
        with:
          name: agentflow-${{ matrix.os }}
          path: bin/
```

### Homebrew

Para distribuicao via Homebrew, a regra pratica e separar os formatos:

- `formula`: para o binario de linha de comando.
- `cask`: para o app `.app` do desktop.

Isso segue a orientacao do Homebrew de nao colocar apps `.app` em formulae e de evitar formulae binarias no `homebrew/core`. O caminho mais previsivel e publicar os artefatos em GitHub Releases, gerar checksum por versao e apontar o cask para o pacote correspondente.

Checklist recomendado para Homebrew:

- manter releases etiquetadas com versao estavel;
- publicar artefatos com nome e checksum consistentes;
- usar cask para o desktop app;
- usar formula apenas para o CLI quando houver um binario independente;
- evitar depender de instalacao manual extra no usuario final;
- validar a instalacao com `brew install` e `brew install --cask` em ambiente limpo.

Para instalacao local de dependencias de desenvolvimento via Homebrew, tambem e possivel manter um `Brewfile` e usar `brew bundle` para reproduzir o ambiente.

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
- `GetRunEvents`, `GetRunArtifacts`, `GetRunArtifact`, `GetRunArtifactPath`
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
- O desktop consome o contrato de artefatos de primeira classe (`kind`, `node_id`, `instance_id`, `media_type`, `size_bytes`, `created_at`, `description`).
- Preview textual e retornado inline pelo backend (`text_content`) ate 128 KiB; binarios nao sao decodificados no frontend.
- Acoes de open/export para binarios usam `GetRunArtifactPath`, que resolve o path absoluto controlado pelo backend sem reconstruir paths no frontend.
- Nao ha TUI neste checkout; se uma TUI for adicionada depois, ela deve consumir os mesmos DTOs/bindings, nao o filesystem diretamente.
- Cancelamento de runs e cooperative: a run interrompe no proximo ponto de checagem de contexto.
