# Task: Desktop app do Agentflow com Wails v3

Implementar uma aplicação desktop completa para o `agentflow` usando Wails v3, reaproveitando o core existente em Go e adicionando uma camada de integração própria para a UI. A aplicação deve expor uma API de frontend compatível com os conceitos e operações já existentes no Agentflow, sem acoplar a interface diretamente aos detalhes internos do domínio.

## Objetivo

Entregar um desktop app pronto para uso que permita:

- carregar, validar e visualizar workflows;
- resolver inputs e executar `dry-run`;
- iniciar execuções reais;
- acompanhar runs, eventos e artefatos;
- visualizar grafo, status e logs em tempo real;
- editar arquivos de workflow e input com fluxo de trabalho orientado ao desktop.

O app deve usar Wails v3 como shell desktop e frontend web, com um adapter de binding que traduza chamadas da UI para a API e os serviços já existentes no `agentflow`.

## Direção de arquitetura

Criar uma nova aplicação desktop sem quebrar o CLI atual. O core existente deve continuar sendo a fonte de verdade para execução de workflows.

### Camadas esperadas

- `cmd/agentflow-desktop` ou equivalente para bootstrap do app Wails.
- `internal/desktop/` para o código específico do desktop app.
- `internal/desktop/binding/` para os métodos expostos ao frontend.
- `internal/desktop/adapter/` para compatibilizar a UI com a API do Agentflow.
- reuso de `internal/app/`, `internal/core/`, `internal/adapters/` e `internal/daemon/` sempre que possível.

### Requisito de adapter

O desktop app deve ter um código adapter explícito entre a UI e o core do Agentflow. Esse adapter precisa:

- converter tipos da UI para os tipos internos do domínio;
- encapsular chamadas ao runtime, validação, plan, graph e execução;
- manter um contrato estável para o frontend, mesmo se a API interna mudar;
- normalizar erros em respostas consumíveis pela UI;
- suportar streaming de eventos de execução para a interface.

Esse adapter é obrigatório. A UI não deve chamar diretamente pacotes internos do core.

## Contrato do binding

Expor um conjunto coeso de métodos no binding do Wails para cobrir o uso do app. Como ponto de partida, o binding deve oferecer:

- `ListWorkflows`
- `LoadWorkflow`
- `ValidateWorkflow`
- `GenerateGraph`
- `DryRunWorkflow`
- `RunWorkflow`
- `CancelRun`
- `ListRuns`
- `GetRun`
- `GetRunEvents`
- `GetRunArtifacts`
- `ResolveInput`
- `SaveWorkflow`
- `SaveInput`
- `OpenPath`
- `GetAppSettings`
- `UpdateAppSettings`

Se necessário, incluir métodos auxiliares para:

- observar progresso;
- consultar status da execução;
- recuperar o último erro;
- sincronizar estado da UI após mudanças no filesystem.

## Regras de compatibilidade

O adapter deve permanecer compatível com a API atual do Agentflow e com a organização já existente no repositório.

- Reaproveitar os tipos e regras de workflow do core sempre que possível.
- Não duplicar a lógica de validação, planificação ou execução na camada desktop.
- Reusar os adapters atuais de YAML, runtime, eventos e repositório local.
- Preservar a semântica atual de `validate`, `graph`, `dry-run` e `run`.
- Tratar runs, eventos e artefatos de forma consistente com o armazenamento local já existente.

## UI e design

A interface deve seguir uma linguagem visual inspirada no design kit da Apple, com estética de `Liquid Glass`, similar ao app do Codex Desktop.

### Direção visual

- fundo com camadas translúcidas e profundidade suave;
- superfícies com blur, brilho discreto e bordas delicadas;
- tipografia limpa e sofisticada;
- estados de foco e hover com refinamento sutil;
- animações leves de transição, evitando microinterações genéricas demais;
- composição editorial, com hierarquia clara e poucas caixas visuais pesadas.

### Componentes sugeridos

- sidebar de navegação com workflows, runs e settings;
- painel principal com grafo do workflow;
- painel lateral com detalhes da seleção;
- console/log stream para execuções;
- editor para YAML e input JSON;
- status bar com estado do runtime e run atual;
- toasts/alerts com aparência nativa e discreta.

### Comportamento visual

- aparência desktop nativa, não web app genérico;
- responsivo em janelas pequenas e grandes;
- transições suaves entre abas e painéis;
- tema claro como base, com possibilidade de suportar variação escura se o design system permitir;
- componentes visualmente coerentes com o ecossistema Apple, sem copiar literalmente assets proprietários.

## Funcionalidades mínimas do MVP

1. Abrir um workflow existente.
2. Validar o workflow.
3. Exibir o grafo do workflow.
4. Resolver input e mostrar `dry-run`.
5. Iniciar uma execução.
6. Mostrar progresso e eventos em tempo real.
7. Listar runs anteriores.
8. Abrir detalhes de um run.
9. Inspecionar logs e artefatos.
10. Editar e salvar workflow/input.

## Persistência e estado local

O app deve armazenar estado local suficiente para uma experiência desktop real:

- caminho do workspace atual;
- arquivos recentes;
- preferências de UI;
- últimas execuções acessadas;
- configurações de binários e paths externos se necessário.

Sempre que possível, usar o repositório local já existente para runs e artefatos, sem inventar um armazenamento paralelo incompatível.

## Estratégia técnica

### Backend

- criar bootstrap Wails v3 em Go;
- instanciar o serviço de desktop com dependências do Agentflow;
- conectar o binding ao adapter;
- tratar lifecycle do app, inicialização e shutdown;
- expor stream de eventos via callback, pub/sub ou mecanismo equivalente do Wails;
- manter isolamento entre UI e core.

### Frontend

- implementar a UI em uma stack compatível com Wails v3;
- construir o design system baseado em tokens visuais para blur, translucidez, sombras e bordas;
- estruturar telas para workspace, workflow, run e settings;
- suportar editor de texto com destaque de sintaxe para YAML/JSON;
- mostrar grafo e timeline de execução de forma clara.

## Critérios de aceite

- A aplicação desktop sobe com Wails v3.
- O frontend consome exclusivamente o binding/adapter do desktop.
- O adapter traduz corretamente chamadas da UI para a API existente do Agentflow.
- Validar, gerar grafo, dry-run e run funcionam no desktop.
- Runs e eventos são exibidos com atualização adequada.
- O visual segue a direção `Apple Liquid Glass` com acabamento polido.
- O CLI atual continua funcionando.
- A implementação possui testes suficientes para o adapter e para os fluxos principais.

## Testes esperados

- testes unitários para o adapter de binding;
- testes para serialização e conversão de tipos entre UI e core;
- testes de regressão para as operações principais do desktop;
- testes de integração sempre que houver dependência de filesystem ou runtime real;
- verificação manual da UI desktop com um fluxo completo de run.

## Entregáveis

- aplicação desktop Wails v3 funcional;
- adapter de binding compatível com a API do Agentflow;
- frontend com identidade visual baseada em Liquid Glass;
- documentação mínima de execução e desenvolvimento local;
- ajustes de build para empacotamento desktop quando necessário.

## Observações finais

- Priorizar reuso do core e evitar duplicação de lógica.
- O adapter é a peça central da compatibilidade entre UI e Agentflow.
- O design deve parecer intencional, premium e nativo, não apenas um dashboard genérico.
