# Agentflow TUI

Interface terminal interativa (TUI) para inspecionar, controlar e executar workflows localmente. Construida com [Bubble Tea](https://github.com/charmbracelet/bubbletea), oferece navegacao por teclado e mouse, atualizacao em tempo real de runs e operacoes locais mesmo quando o daemon esta indisponivel.

## Uso basico

```bash
agentflow tui
```

Lanca a TUI em tela cheia. Para sair, pressione `q` ou `ctrl+c`.

## Flags

| Flag               | Descricao                                               |
| ------------------ | ------------------------------------------------------- | ------ | ----------------------------------- |
| `--workflow <ref>` | Seleciona um workflow logo na abertura (rota Workflows) |
| `--run <id>`       | Seleciona um run logo na abertura (rota Runs)           |
| `--daemon`         | Exige conexao com o daemon; encerra se indisponivel     |
| `--no-mouse`       | Desabilita suporte a mouse                              |
| `--theme <auto     | light                                                   | dark>` | Define o tema visual (padrao: auto) |

Exemplos:

```bash
# Abrir com tema claro e sem mouse
agentflow tui --theme light --no-mouse

# Abrir ja selecionando um run
agentflow tui --run run-abc123

# Abrir exigindo daemon
agentflow tui --daemon
```

## Rotas e atalhos

A TUI possui 6 rotas acessiveis via teclado:

| Tecla | Rota      | Descricao                                              |
| ----- | --------- | ------------------------------------------------------ |
| `1`   | Dashboard | Visao geral do daemon e runs recentes                  |
| `2`   | Workflows | Lista, validacao, grafo e dry-run de workflows locais  |
| `3`   | Runs      | Detalhes de um run, timeline, nodes e controles        |
| `4`   | Logs      | Logs textuais e eventos estruturados com filtros       |
| `5`   | Artifacts | Lista e preview de artefatos do run selecionado        |
| `6`   | Settings  | Preferencias de tema, mouse, caminhos e reduced motion |

Navegacao global:

- `[` e `]` — navegar entre rotas
- `←/h` e `→/l` — navegar dentro de uma rota (quando aplicavel)
- `tab` — alternar foco/campos
- `?` — alternar painel de ajuda completa
- `q` / `ctrl+c` — sair

### Dashboard

Mostra o status do daemon, contagem de runs por status e lista dos runs mais recentes. Pressione `enter` em um run para abrir sua rota de detalhes.

### Workflows

Lista workflows descobertos em `.agentflow/workflows` e `~/.agentflow/workflows`. Atalhos:

- `enter` — seleciona workflow e abre painel de detalhes
- `j/k` — navegar na lista
- `/` — filtrar por nome
- `v` — validar workflow selecionado
- `g` — gerar grafo Mermaid
- `d` — executar dry-run
- `h/l` ou `tab/shift+tab` — alternar abas (Overview, Graph, Dry-run)
- `esc` ou `b` — voltar para lista

### Runs

Exibe detalhes do run selecionado, progresso, lista de nodes e timeline de eventos. Atalhos:

- `j/k` — navegar entre nodes
- `enter` — selecionar node para ver stdout/stderr
- `esc` — desselecionar node (mostra timeline)
- `c` — solicitar cancelamento (confirma com `y`)
- `p` — solicitar pausa (confirma com `y`)
- `r` — solicitar retomada (confirma com `y`)
- `n` / `esc` — cancelar confirmacao

### Logs

Mostra logs textuais ou eventos estruturados. Atalhos:

- `e` — alternar entre logs e eventos
- `/` — ativar filtro
- `tab` — alternar campo de filtro (text, node, type)
- `↑/↓` — rolar conteudo
- `esc` — sair do modo de filtro

### Artifacts

Lista artefatos do run selecionado. Atalhos:

- `j/k` — navegar na lista
- `enter` — carregar preview do artefato
- `esc` — limpar preview

Preview suporta conteudo textual e decodificacao base64. Arquivos binarios exibem mensagem de fallback. O preview e limitado a 40 linhas e 4096 bytes por arquivo.

### Settings

Edita preferencias persistidas em `~/.agentflow/tui-settings.json`. Atalhos:

- `j/k` — navegar entre campos
- `enter` — editar valor ou alternar booleano
- `s` — salvar alteracoes

Campos disponiveis:

- Theme (auto / dark / light)
- Mouse (on / off)
- Reduced Motion (on / off)
- Codex Path, Claude Path, Pi Path
- Run Root

## Comportamento com daemon indisponivel

Por padrao, a TUI funciona em modo hibrido:

- Se o daemon (`agentflowd`) estiver rodando, exibe runs em tempo real e permite controles (cancel, pause, resume).
- Se o daemon estiver parado, a TUI continua operacional para funcoes locais: lista de workflows, validacao, grafo e dry-run. O indicador de status no topo muda de `●` (online) para `○` (offline).

Com a flag `--daemon`, a TUI exige conexao e encerra imediatamente se o daemon nao estiver disponivel.

## Operacoes locais disponiveis

Mesmo sem daemon, as seguintes operacoes funcionam:

- Listar workflows locais
- Validar workflow
- Gerar grafo Mermaid
- Executar dry-run
- Navegar entre rotas e visualizar interface

Operacoes que dependem do daemon (runs, logs, artifacts, controles) exibem estados vazios ou mensagens indicando indisponibilidade.

## Acessibilidade e UX

### Teclado como fallback principal

Toda navegacao e acao e acessivel via teclado. O mouse e opcional e pode ser desabilitado com `--no-mouse`. Em terminais estreitos (largura <= 60), a barra lateral e ocultada automaticamente e a navegacao por numeros (1-6) torna-se o principal metodo de troca de rota.

### Reduced motion

A flag `--no-mouse` desabilita apenas o mouse. Para desabilitar animacoes (reduced motion), altere a configuracao em Settings ou use a persistencia de settings. Quando reduced motion esta ativo, barras de progresso e transicoes sao instantaneas.

### Contraste e temas

O tema `auto` detecta o ambiente terminal e escolhe entre dark e light. Temas definidos garantem contraste minimo para leitura em condicoes normais. O tema pode ser alternado a qualquer momento em Settings.

### Truncamento

Textos longos sao truncados visualmente com reticencias (`…`) para evitar quebra de layout em terminais estreitos. Isso se aplica a nomes de workflow, caminhos, mensagens de log e preview de artefatos.

## Limites e seguranca

- Preview de artefatos: limitado a 40 linhas e 4096 bytes por arquivo para evitar consumo excessivo de memoria.
- Eventos em cache: limitados a 5000 eventos por run; eventos mais antigos sao descartados automaticamente.
- Linhas de log em cache: limitadas a 5000 linhas por run.
- A TUI nao executa workflows diretamente; execucao requer o daemon (`agentflowd`) ou a CLI (`agentflow run`).

## QA manual sugerido

1. Abrir a TUI sem daemon rodando: verificar que a interface abre, mostra `○` no titulo e workflows locais funcionam.
2. Abrir com `--daemon`: verificar que encerra com erro se o daemon nao estiver rodando.
3. Iniciar o daemon, abrir a TUI: verificar que mostra `●` e lista runs.
4. Selecionar um run ativo (Dashboard -> enter): verificar progresso, timeline e nodes.
5. Testar controles: `c` para cancelar, `p` para pausar, `r` para retomar.
6. Testar filtros de logs: `/`, digitar texto, `tab` para alternar campos.
7. Testar `--no-mouse`: verificar que cliques de mouse sao ignorados.
8. Testar tema light/dark: alterar em Settings, salvar com `s`, verificar mudanca visual.
9. Testar terminal estreito (redimensionar para <60 colunas): verificar que sidebar desaparece e layout se adapta.
10. Testar `--workflow` e `--run`: verificar que abrem nas rotas corretas.

## Arquivos principais

- [`internal/tui/app`](/internal/tui/app/): modelo raiz Bubble Tea, rotas, mensagens e comandos.
- [`internal/tui/views`](/internal/tui/views/): views de cada rota (Dashboard, Workflows, Runs, Logs, Artifacts, Settings).
- [`internal/tui/components`](/internal/tui/components/): componentes reutilizaveis (sidebar, status bar, key help, charts, timeline).
- [`internal/tui/client`](/internal/tui/client/): camada de cliente com fallback local e integracao com daemon.
- [`internal/tui/theme`](/internal/tui/theme/): definicoes de tema dark/light.
- [`internal/cli/root.go`](/internal/cli/root.go): registro do comando `agentflow tui` e flags.
