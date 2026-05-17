package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/animation"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/components"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	"github.com/diasYuri/agentflow/internal/tui/views"
	zone "github.com/lrstanley/bubblezone"
)

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	opts  Options
	theme *theme.Theme
	keys  KeyMap

	width  int
	height int

	route     Route
	sidebar   *components.Sidebar
	statusBar *components.StatusBar
	keyHelp   *components.KeyHelp

	dashboard *views.Dashboard
	workflows *views.Workflows
	runs      *views.Runs
	logs      *views.Logs
	artifacts *views.Artifacts
	settings  *views.Settings

	zoneManager  *zone.Manager
	showHelp     bool
	anim         animation.Config
	settingsData TUISettings

	// Client layer
	client      client.Client
	daemonState client.DaemonState

	// Data caches
	runsList      []client.RunSummary
	workflowsList []client.LocalWorkflow
	runDetail     client.RunSummary
	runEvents     []client.EventLine
	runLogs       []string
	runArtifacts  []client.ArtifactSummary
	runNodes      []client.NodeSummary
	runPlan       client.PlanSummary

	// Selection and polling
	selectedRunID       string
	selectedWorkflowRef string
	runEventCursors     map[string]int
	polling             map[tickKind]bool
}

// NewModel creates the root TUI model.
func NewModel(opts Options) *Model {
	t := theme.Default(opts.Theme)
	zm := zone.New()

	daemonClient := client.NewDaemonClient("")
	localClient := client.NewLocalClient()
	c := client.NewComposite(daemonClient, localClient)

	m := &Model{
		opts:            opts,
		theme:           t,
		keys:            DefaultKeyMap(),
		route:           RouteDashboard,
		sidebar:         components.NewSidebar(zm),
		statusBar:       components.NewStatusBar("Agentflow"),
		keyHelp:         components.NewKeyHelp(),
		dashboard:       views.NewDashboard(zm),
		workflows:       views.NewWorkflows(zm),
		runs:            views.NewRuns(zm, animation.NewConfig(opts.ReducedMotion)),
		logs:            views.NewLogs(),
		artifacts:       views.NewArtifacts(zm),
		settings:        views.NewSettings(),
		zoneManager:     zm,
		client:          c,
		runEventCursors: make(map[string]int),
		polling:         make(map[tickKind]bool),
		anim:            animation.NewConfig(opts.ReducedMotion),
		settingsData:    LoadTUISettings(),
	}

	if opts.Run != "" {
		m.selectedRunID = opts.Run
		m.route = RouteRuns
	}
	if opts.Workflow != "" {
		m.selectedWorkflowRef = opts.Workflow
		m.route = RouteWorkflows
	}
	m.sidebar.SetActiveRoute(string(m.route))

	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		refreshDaemonStatusCmd(m.client),
		tickCmd(tickDaemon, "", daemonTickInterval),
		fetchRunsCmd(m.client),
		tickCmd(tickRuns, "", runsTickInterval),
	}
	m.polling[tickDaemon] = true
	m.polling[tickRuns] = true
	if m.selectedRunID != "" {
		cmds = append(cmds,
			fetchRunDetailCmd(m.client, m.selectedRunID),
			fetchRunLogsCmd(m.client, m.selectedRunID),
			tickCmd(tickRunDetail, m.selectedRunID, runDetailInterval),
			tickCmd(tickRunEvents, m.selectedRunID, runEventsInterval),
			tickCmd(tickRunLogs, m.selectedRunID, runLogsInterval),
		)
		m.polling[tickRunDetail] = true
		m.polling[tickRunEvents] = true
		m.polling[tickRunLogs] = true
	}
	if m.selectedWorkflowRef != "" {
		cmds = append(cmds,
			listWorkflowsCmd(m.client),
			validateWorkflowCmd(m.client, m.selectedWorkflowRef),
		)
	}
	return tea.Batch(cmds...)
}

// currentView returns the active view for the current route.
func (m *Model) currentView() viewable {
	switch m.route {
	case RouteWorkflows:
		return m.workflows
	case RouteRuns:
		return m.runs
	case RouteLogs:
		return m.logs
	case RouteArtifacts:
		return m.artifacts
	case RouteSettings:
		return m.settings
	default:
		return m.dashboard
	}
}

// switchToRun selects a run, clears caches, starts polling and navigates to the runs route.
func (m *Model) switchToRun(runID string) tea.Cmd {
	m.selectedRunID = runID
	m.runDetail = client.RunSummary{}
	m.runEvents = nil
	m.runLogs = nil
	m.runNodes = nil
	m.runArtifacts = nil
	m.runPlan = client.PlanSummary{}
	m.runEventCursors = make(map[string]int)
	m.polling[tickRunDetail] = true
	m.polling[tickRunEvents] = true
	m.polling[tickRunLogs] = true
	m.runs.SetRun(client.RunSummary{})
	m.runs.SetNodes(nil)
	m.runs.SetEvents(nil)
	m.logs.SetLines(nil)
	m.logs.SetEvents(nil)
	routeCmd := m.setRoute(RouteRuns)
	return tea.Batch(
		routeCmd,
		fetchRunDetailCmd(m.client, runID),
		fetchRunLogsCmd(m.client, runID),
		fetchRunNodesCmd(m.client, runID),
		fetchRunEventsCmd(m.client, runID, 0),
		tickCmd(tickRunDetail, runID, runDetailInterval),
		tickCmd(tickRunEvents, runID, runEventsInterval),
		tickCmd(tickRunLogs, runID, runLogsInterval),
	)
}

// viewable is the common interface for placeholder views.
type viewable interface {
	SetSize(w, h int)
	Update(msg tea.Msg)
	View(t *theme.Theme) string
}

// ShortHelp returns the current keybindings for the help component.
func (m *Model) ShortHelp() []key.Binding {
	return []key.Binding{
		m.keys.Quit,
		m.keys.PrevRoute,
		m.keys.NextRoute,
	}
}

// FullHelp returns the full keybinding list.
func (m *Model) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.keys.PrevRoute, m.keys.NextRoute, m.keys.Left, m.keys.Right},
		{m.keys.Quit, m.keys.Help},
	}
}
