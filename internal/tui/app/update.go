package app

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/animation"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	"github.com/diasYuri/agentflow/internal/tui/views"
)

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case DaemonStatusMsg:
		if msg.Err != nil {
			if client.IsDaemonUnavailable(msg.Err) {
				if m.opts.Daemon {
					m.daemonState = client.DaemonState{Status: client.DaemonRequiredMissing}
					return m, tea.Quit
				}
				m.daemonState = client.DaemonState{Status: client.DaemonUnavailable}
			} else {
				m.daemonState = client.DaemonState{Status: client.DaemonUnknown}
			}
		} else {
			m.daemonState = msg.State
		}
		m.statusBar.SetTitle("Agentflow — " + string(m.route) + m.daemonIndicator())
		m.dashboard.SetDaemonState(m.daemonState)
		return m, nil

	case RunsListMsg:
		if msg.Err == nil {
			m.runsList = msg.Runs
		}
		m.dashboard.SetRuns(m.runsList)
		return m, nil

	case RunDetailMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runDetail = msg.Run
			m.runs.SetRun(m.runDetail)
		}
		return m, nil

	case RunLogsMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runLogs = msg.Lines
			if len(m.runLogs) > maxCachedLogLines {
				m.runLogs = m.runLogs[len(m.runLogs)-maxCachedLogLines:]
			}
			m.logs.SetLines(m.runLogs)
		}
		return m, nil

	case RunEventsMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runEvents = append(m.runEvents, msg.Batch.Events...)
			if len(m.runEvents) > maxCachedEvents {
				m.runEvents = m.runEvents[len(m.runEvents)-maxCachedEvents:]
			}
			m.runEventCursors[msg.RunID] = msg.Batch.NextCursor
			if !msg.Batch.HasMore {
				m.polling[tickRunEvents] = false
			}
			m.runs.SetEvents(m.runEvents)
			m.logs.SetEvents(m.runEvents)
		}
		return m, nil

	case RunArtifactsMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runArtifacts = msg.Artifacts
			m.artifacts.SetArtifacts(m.runArtifacts)
		}
		return m, nil

	case ArtifactMsg:
		if msg.Err != nil {
			m.artifacts.SetPreview(client.ArtifactSummary{}, msg.Err)
		} else {
			m.artifacts.SetPreview(msg.Artifact, nil)
		}
		return m, nil

	case RunNodesMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runNodes = msg.Nodes
			m.runs.SetNodes(m.runNodes)
		}
		return m, nil

	case RunPlanMsg:
		if msg.Err == nil {
			if msg.RunID != m.selectedRunID {
				return m, nil
			}
			m.runPlan = msg.Plan
		}
		return m, nil

	case ControlMsg:
		if msg.Err == nil && m.selectedRunID != "" {
			m.polling[tickRunEvents] = true
			m.polling[tickRunDetail] = true
			m.polling[tickRunLogs] = true
			return m, tea.Batch(
				fetchRunDetailCmd(m.client, m.selectedRunID),
				fetchRunLogsCmd(m.client, m.selectedRunID),
				fetchRunNodesCmd(m.client, m.selectedRunID),
				fetchRunEventsCmd(m.client, m.selectedRunID, m.runEventCursors[m.selectedRunID]),
				tickCmd(tickRunEvents, m.selectedRunID, runEventsInterval),
				tickCmd(tickRunDetail, m.selectedRunID, runDetailInterval),
				tickCmd(tickRunLogs, m.selectedRunID, runLogsInterval),
			)
		}
		return m, nil

	case WorkflowsListMsg:
		if msg.Err == nil {
			m.workflowsList = msg.Workflows
			m.workflows.SetWorkflows(m.workflowsList)
			if m.selectedWorkflowRef != "" {
				for i, wf := range m.workflowsList {
					if workflowMatchesRef(wf, m.selectedWorkflowRef) {
						m.workflows.SelectByIndex(i)
						break
					}
				}
			}
		} else {
			m.workflows.SetListError(msg.Err)
		}
		return m, nil

	case WorkflowValidateMsg:
		if msg.Err != nil {
			m.workflows.SetValidationResult(msg.Ref, msg.Err)
		} else {
			m.workflows.SetValidationResult(msg.Ref, nil)
		}
		return m, nil

	case WorkflowGraphMsg:
		m.workflows.SetGraphResult(msg.Ref, msg.Output)
		return m, nil

	case WorkflowDryRunMsg:
		m.workflows.SetDryRunResult(msg.Ref, msg.Output)
		return m, nil

	case tickMsg:
		return m.handleTick(msg)

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.keys.Help) {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if key.Matches(msg, m.keys.NextRoute) {
			idx := RouteIndex(m.route)
			cmd := m.setRoute(RouteFromIndex(idx + 1))
			return m, cmd
		}
		if key.Matches(msg, m.keys.PrevRoute) {
			idx := RouteIndex(m.route)
			cmd := m.setRoute(RouteFromIndex(idx - 1))
			return m, cmd
		}

		// Dashboard: open selected run.
		if m.route == RouteDashboard {
			switch msg.String() {
			case "enter":
				if run, ok := m.dashboard.SelectedRun(); ok {
					return m, m.switchToRun(run.ID)
				}
			}
		}

		// Runs route controls and navigation.
		if m.route == RouteRuns {
			m.runs.Update(msg)
			if m.runs.Confirming() != "" {
				switch msg.String() {
				case "y":
					action := m.runs.Confirming()
					m.runs.SetConfirming("")
					switch action {
					case "cancel":
						return m, cancelRunCmd(m.client, m.selectedRunID)
					case "pause":
						return m, pauseRunCmd(m.client, m.selectedRunID)
					case "resume":
						return m, resumeRunCmd(m.client, m.selectedRunID)
					}
				case "n", "esc":
					m.runs.SetConfirming("")
					return m, nil
				}
				return m, nil
			}
			switch msg.String() {
			case "c":
				if canControlStatus(m.runDetail.Status, "cancel") {
					m.runs.SetConfirming("cancel")
				}
			case "p":
				if canControlStatus(m.runDetail.Status, "pause") {
					m.runs.SetConfirming("pause")
				}
			case "r":
				if canControlStatus(m.runDetail.Status, "resume") {
					m.runs.SetConfirming("resume")
				}
			}
		}

		// Artifacts route: enter fetches preview.
		if m.route == RouteArtifacts {
			m.artifacts.Update(msg)
			switch msg.String() {
			case "enter":
				if art, ok := m.artifacts.SelectedArtifact(); ok && m.selectedRunID != "" {
					return m, fetchArtifactCmd(m.client, m.selectedRunID, art.ID)
				}
			}
		}

		// Settings route: s saves.
		if m.route == RouteSettings {
			m.settings.Update(msg)
			if msg.String() == "s" {
				return m, m.saveSettings()
			}
		}

		// Workflow actions when on workflows route.
		if m.route == RouteWorkflows {
			switch msg.String() {
			case "v":
				if wf, ok := m.workflows.SelectedWorkflow(); ok {
					return m, validateWorkflowCmd(m.client, wf.Path)
				}
			case "g":
				if wf, ok := m.workflows.SelectedWorkflow(); ok {
					return m, graphWorkflowCmd(m.client, wf.Path)
				}
			case "d":
				if wf, ok := m.workflows.SelectedWorkflow(); ok {
					return m, dryRunWorkflowCmd(m.client, wf.Path, nil, nil)
				}
			}
		}

		// Number keys 1-6 jump to routes.
		switch msg.String() {
		case "1":
			return m, m.setRoute(RouteDashboard)
		case "2":
			return m, m.setRoute(RouteWorkflows)
		case "3":
			return m, m.setRoute(RouteRuns)
		case "4":
			return m, m.setRoute(RouteLogs)
		case "5":
			return m, m.setRoute(RouteArtifacts)
		case "6":
			return m, m.setRoute(RouteSettings)
		}

	case tea.MouseMsg:
		if !m.opts.Mouse {
			return m, nil
		}
		if m.zoneManager != nil {
			if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
				for _, r := range []string{"dashboard", "workflows", "runs", "logs", "artifacts", "settings"} {
					zi := m.zoneManager.Get("sidebar_" + r)
					if zi != nil && zi.InBounds(msg) {
						return m, m.setRoute(Route(r))
					}
				}
			}
		}
	}

	// Pass messages to the current view.
	m.currentView().Update(msg)
	return m, nil
}

func (m *Model) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	if !m.polling[msg.kind] {
		return m, nil
	}
	switch msg.kind {
	case tickDaemon:
		return m, tea.Batch(
			refreshDaemonStatusCmd(m.client),
			tickCmd(tickDaemon, "", daemonTickInterval),
		)
	case tickRuns:
		return m, tea.Batch(
			fetchRunsCmd(m.client),
			tickCmd(tickRuns, "", runsTickInterval),
		)
	case tickRunDetail:
		if m.selectedRunID == "" {
			m.polling[tickRunDetail] = false
			return m, nil
		}
		if m.runDetail.Status != "" && isTerminalStatus(m.runDetail.Status) {
			m.polling[tickRunDetail] = false
			return m, nil
		}
		cmds := []tea.Cmd{
			fetchRunDetailCmd(m.client, m.selectedRunID),
			tickCmd(tickRunDetail, m.selectedRunID, runDetailInterval),
		}
		return m, tea.Batch(cmds...)
	case tickRunEvents:
		if m.selectedRunID == "" {
			m.polling[tickRunEvents] = false
			return m, nil
		}
		cursor := m.runEventCursors[m.selectedRunID]
		cmds := []tea.Cmd{
			fetchRunEventsCmd(m.client, m.selectedRunID, cursor),
			tickCmd(tickRunEvents, m.selectedRunID, runEventsInterval),
		}
		if m.runDetail.Status != "" && isTerminalStatus(m.runDetail.Status) {
			m.polling[tickRunEvents] = false
		}
		return m, tea.Batch(cmds...)
	case tickRunLogs:
		if m.selectedRunID == "" {
			m.polling[tickRunLogs] = false
			return m, nil
		}
		if m.runDetail.Status != "" && isTerminalStatus(m.runDetail.Status) {
			m.polling[tickRunLogs] = false
			return m, nil
		}
		return m, tea.Batch(
			fetchRunLogsCmd(m.client, m.selectedRunID),
			tickCmd(tickRunLogs, m.selectedRunID, runLogsInterval),
		)
	}
	return m, nil
}

func (m *Model) daemonIndicator() string {
	switch m.daemonState.Status {
	case client.DaemonAvailable:
		return " ●"
	case client.DaemonUnavailable:
		return " ○"
	case client.DaemonRequiredMissing:
		return " ✗"
	default:
		return " ?"
	}
}

func isTerminalStatus(status string) bool {
	switch strings.ToLower(status) {
	case "success", "failed", "cancelled", "canceled", "timeout", "paused":
		return true
	default:
		return false
	}
}

func canControlStatus(status, action string) bool {
	s := strings.ToLower(status)
	switch action {
	case "cancel":
		return s == "running" || s == "created" || s == "paused" || s == "wait_approval"
	case "pause":
		return s == "running" || s == "created"
	case "resume":
		return s == "paused"
	}
	return false
}

func workflowMatchesRef(wf client.LocalWorkflow, ref string) bool {
	if wf.Path == ref || wf.Name == ref {
		return true
	}
	return filepath.Clean(wf.Path) == filepath.Clean(ref)
}

// bottomPanelHeight returns the height allocated to the optional bottom panel.
// Currently no route requests a bottom panel; this is reserved for future plans.
func (m *Model) bottomPanelHeight() int {
	return 0
}

// layout recalculates dimensions for all components based on current size.
func (m *Model) layout() {
	sidebarWidth := 0
	if m.width > 60 {
		sidebarWidth = 20
	}

	statusHeight := 1
	helpHeight := 1
	bpHeight := m.bottomPanelHeight()
	mainHeight := m.height - statusHeight - bpHeight - helpHeight
	if mainHeight < 0 {
		mainHeight = 0
	}

	m.sidebar.SetSize(sidebarWidth, mainHeight)
	m.statusBar.SetTitle("Agentflow — " + string(m.route) + m.daemonIndicator())

	mainWidth := m.width
	if sidebarWidth > 0 {
		mainWidth = m.width - sidebarWidth - 1 // gap for border
	}

	m.dashboard.SetSize(mainWidth, mainHeight)
	m.workflows.SetSize(mainWidth, mainHeight)
	m.runs.SetSize(mainWidth, mainHeight)
	m.logs.SetSize(mainWidth, mainHeight)
	m.artifacts.SetSize(mainWidth, mainHeight)
	m.settings.SetSize(mainWidth, mainHeight)
}

// setRoute changes the active route and updates dependent state.
// When switching to the workflows route, it sets loading and returns a command to list workflows.
func (m *Model) setRoute(r Route) tea.Cmd {
	prev := m.route
	m.route = r
	m.sidebar.SetActiveRoute(string(r))
	if r == RouteWorkflows && prev != RouteWorkflows {
		m.workflows.SetLoading(true)
		return listWorkflowsCmd(m.client)
	}
	if r == RouteArtifacts && prev != RouteArtifacts && m.selectedRunID != "" {
		return fetchRunArtifactsCmd(m.client, m.selectedRunID)
	}
	if r == RouteSettings && prev != RouteSettings {
		m.syncSettingsFields()
	}
	return nil
}

func (m *Model) syncSettingsFields() {
	fields := []views.SettingField{
		{Label: "Theme", Key: "theme", Value: string(m.settingsData.Theme), Options: []string{"auto", "dark", "light"}},
		{Label: "Mouse", Key: "mouse", IsBool: true, Bool: m.settingsData.Mouse},
		{Label: "Reduced Motion", Key: "reduced_motion", IsBool: true, Bool: m.settingsData.ReducedMotion},
		{Label: "Codex Path", Key: "codex_path", Value: m.settingsData.CodexPath},
		{Label: "Claude Path", Key: "claude_path", Value: m.settingsData.ClaudePath},
		{Label: "Pi Path", Key: "pi_path", Value: m.settingsData.PiPath},
		{Label: "Run Root", Key: "run_root", Value: m.settingsData.RunRoot},
	}
	m.settings.SetFields(fields)
	m.settings.SetChanged(false)
}

func (m *Model) saveSettings() tea.Cmd {
	fields := m.settings.Fields()
	for _, f := range fields {
		switch f.Key {
		case "theme":
			m.settingsData.Theme = theme.Mode(f.Value)
		case "mouse":
			m.settingsData.Mouse = f.Bool
		case "reduced_motion":
			m.settingsData.ReducedMotion = f.Bool
		case "codex_path":
			m.settingsData.CodexPath = f.Value
		case "claude_path":
			m.settingsData.ClaudePath = f.Value
		case "pi_path":
			m.settingsData.PiPath = f.Value
		case "run_root":
			m.settingsData.RunRoot = f.Value
		}
	}
	m.settings.SetChanged(false)
	m.theme = theme.Default(m.settingsData.Theme)
	m.anim = animation.NewConfig(m.settingsData.ReducedMotion)
	if err := SaveTUISettings(m.settingsData); err != nil {
		return func() tea.Msg { return nil }
	}
	return nil
}
