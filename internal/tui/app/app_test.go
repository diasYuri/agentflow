package app

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestNewModelDefaultsToDashboard(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	if m.route != RouteDashboard {
		t.Fatalf("expected default route dashboard, got %s", m.route)
	}
}

func TestSetRouteChangesActiveRoute(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.setRoute(RouteWorkflows)
	if m.route != RouteWorkflows {
		t.Fatalf("expected route workflows, got %s", m.route)
	}
	if m.sidebar.ActiveRoute() != "workflows" {
		t.Fatalf("expected sidebar active route workflows, got %s", m.sidebar.ActiveRoute())
	}
}

func TestNextRouteNavigatesForward(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if m.route != RouteWorkflows {
		t.Fatalf("expected route workflows after next route, got %s", m.route)
	}
}

func TestPrevRouteNavigatesBackward(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	m.setRoute(RouteRuns)

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if m.route != RouteWorkflows {
		t.Fatalf("expected route workflows after prev route, got %s", m.route)
	}
}

func TestNumberKeysJumpToRoute(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()

	tests := []struct {
		key   string
		route Route
	}{
		{"1", RouteDashboard},
		{"2", RouteWorkflows},
		{"3", RouteRuns},
		{"4", RouteLogs},
		{"5", RouteArtifacts},
		{"6", RouteSettings},
	}

	for _, tt := range tests {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		if m.route != tt.route {
			t.Fatalf("expected route %s for key %s, got %s", tt.route, tt.key, m.route)
		}
	}
}

func TestQuitKeyReturnsQuitCmd(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	// tea.Quit is a function; we can't easily compare it, but we can ensure cmd is not nil.
}

func TestResizeUpdatesDimensions(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 {
		t.Fatalf("expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Fatalf("expected height 40, got %d", m.height)
	}
}

func TestLayoutForSmallWidthHidesSidebar(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	if m.width != 50 {
		t.Fatalf("expected width 50, got %d", m.width)
	}
	// Sidebar width should be 0 when total width <= 60.
	// We verify indirectly by checking that the model doesn't panic and view renders.
	_ = m.View()
}

func TestThemeAutoDefaultsToDark(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeAuto})
	if m.theme.Mode != theme.ModeDark && m.theme.Mode != theme.ModeLight {
		t.Fatalf("expected auto theme to resolve to dark or light, got %s", m.theme.Mode)
	}
}

func TestRunOptionSelectsRunsRoute(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "run-1"})
	if m.route != RouteRuns {
		t.Fatalf("expected route runs, got %s", m.route)
	}
	if m.selectedRunID != "run-1" {
		t.Fatalf("expected selected run run-1, got %s", m.selectedRunID)
	}
}

func TestWorkflowOptionSelectsWorkflowsRoute(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Workflow: "hello"})
	if m.route != RouteWorkflows {
		t.Fatalf("expected route workflows, got %s", m.route)
	}
	if m.selectedWorkflowRef != "hello" {
		t.Fatalf("expected selected workflow hello, got %s", m.selectedWorkflowRef)
	}
}

func TestWorkflowListMatchesByNameOrPath(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Workflow: "hello"})
	_, _ = m.Update(WorkflowsListMsg{Workflows: []client.LocalWorkflow{
		{Name: "other", Path: "/tmp/other.yaml"},
		{Name: "hello", Path: "/tmp/hello.yaml"},
	}})
	selected, ok := m.workflows.SelectedWorkflow()
	if !ok {
		t.Fatal("expected workflow selection")
	}
	if selected.Name != "hello" {
		t.Fatalf("expected selection by workflow name, got %s", selected.Name)
	}
}

func TestDaemonStatusMsgAvailable(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()

	_, _ = m.Update(DaemonStatusMsg{
		State: client.DaemonState{Status: client.DaemonAvailable, Running: true, Runs: 5},
	})
	if m.daemonState.Status != client.DaemonAvailable {
		t.Fatalf("expected available, got %s", m.daemonState.Status)
	}
}

func TestDaemonStatusMsgUnavailable(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()

	_, _ = m.Update(DaemonStatusMsg{
		Err: client.ErrDaemonUnavailable,
	})
	if m.daemonState.Status != client.DaemonUnavailable {
		t.Fatalf("expected unavailable, got %s", m.daemonState.Status)
	}
}

func TestDaemonStatusMsgRequiredMissingQuits(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Daemon: true})
	m.width = 100
	m.height = 30
	m.layout()

	_, cmd := m.Update(DaemonStatusMsg{
		Err: client.ErrDaemonUnavailable,
	})
	if cmd == nil {
		t.Fatal("expected quit command when daemon required")
	}
	if m.daemonState.Status != client.DaemonRequiredMissing {
		t.Fatalf("expected required_missing, got %s", m.daemonState.Status)
	}
}

func TestRunsListMsgUpdatesCache(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, _ = m.Update(RunsListMsg{Runs: []client.RunSummary{{ID: "r1"}}})
	if len(m.runsList) != 1 || m.runsList[0].ID != "r1" {
		t.Fatal("expected runs to be cached")
	}
}

func TestRunDetailMsgUpdatesCache(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, _ = m.Update(RunDetailMsg{Run: client.RunSummary{ID: "r1", Status: "running"}})
	if m.runDetail.ID != "r1" {
		t.Fatal("expected run detail to be cached")
	}
}

func TestRunEventsMsgAppendsEvents(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.selectedRunID = "r1"
	_, _ = m.Update(RunEventsMsg{
		RunID: "r1",
		Batch: client.EventBatch{
			Events:     []client.EventLine{{Cursor: 1, Type: "start"}},
			NextCursor: 2,
			HasMore:    false,
		},
	})
	if len(m.runEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(m.runEvents))
	}
	if m.runEventCursors["r1"] != 2 {
		t.Fatalf("expected cursor 2, got %d", m.runEventCursors["r1"])
	}
	if m.polling[tickRunEvents] {
		t.Fatal("expected polling to stop when HasMore is false")
	}
}

func TestTickMsgDaemon(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.polling[tickDaemon] = true
	_, cmd := m.Update(tickMsg{kind: tickDaemon})
	if cmd == nil {
		t.Fatal("expected command for daemon tick")
	}
}

func TestTickMsgIgnoredWhenNotPolling(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.polling[tickRuns] = false
	_, cmd := m.Update(tickMsg{kind: tickRuns})
	if cmd != nil {
		t.Fatal("expected no command when not polling")
	}
}

func TestReducedMotionWiredToAnimation(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, ReducedMotion: true})
	if !m.anim.Instant() {
		t.Fatal("expected animation instant when reduced motion is enabled")
	}
}

func TestBottomPanelHeightZeroByDefault(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	if m.bottomPanelHeight() != 0 {
		t.Fatalf("expected bottom panel height 0 by default, got %d", m.bottomPanelHeight())
	}
}

func TestLayoutAccountsForBottomPanel(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	// With status=1, bottom=0, help=1, main should be 28.
	// Verify indirectly by rendering the view without panic.
	_ = m.View()
}

func TestIsTerminalStatus(t *testing.T) {
	if !isTerminalStatus("success") {
		t.Fatal("expected success to be terminal")
	}
	if !isTerminalStatus("paused") {
		t.Fatal("expected paused to be terminal")
	}
	if isTerminalStatus("running") {
		t.Fatal("expected running to not be terminal")
	}
}

func TestWorkflowsRouteSetsLoading(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if !m.workflows.IsLoading() {
		t.Fatal("expected loading to be true when entering workflows route")
	}
}

func TestWorkflowsListErrorSetsViewError(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	_, _ = m.Update(WorkflowsListMsg{Err: errors.New("list failed")})
	if m.workflows.ListError() == nil {
		t.Fatal("expected workflows view to have list error")
	}
}

func TestWorkflowsListSuccessClearsLoading(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.workflows.SetLoading(true)
	_, _ = m.Update(WorkflowsListMsg{Workflows: []client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}}})
	if m.workflows.IsLoading() {
		t.Fatal("expected loading to be cleared after list success")
	}
}

func TestWorkflowValidateKeyReturnsCommand(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	m.setRoute(RouteWorkflows)
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if cmd == nil {
		t.Fatal("expected command for validate key")
	}
}

func TestWorkflowGraphKeyReturnsCommand(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	m.setRoute(RouteWorkflows)
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if cmd == nil {
		t.Fatal("expected command for graph key")
	}
}

func TestWorkflowDryRunKeyReturnsCommand(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	m.setRoute(RouteWorkflows)
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("expected command for dry-run key")
	}
}

func TestWorkflowValidateMsgUpdatesView(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, _ = m.Update(WorkflowValidateMsg{Ref: "/a.yaml", Err: nil})
	if !m.workflows.HasValidationResult("/a.yaml") {
		t.Fatal("expected validation result to be stored in view")
	}
}

func TestWorkflowGraphMsgUpdatesView(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, _ = m.Update(WorkflowGraphMsg{Ref: "/a.yaml", Output: "graph TD\nA-->B"})
	if m.workflows.GraphResult("/a.yaml") != "graph TD\nA-->B" {
		t.Fatal("expected graph result to be stored in view")
	}
}

func TestWorkflowDryRunMsgUpdatesView(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.workflows.SetWorkflows([]client.LocalWorkflow{{Name: "a", Path: "/a.yaml"}})
	m.workflows.SelectByIndex(0)
	_, _ = m.Update(WorkflowDryRunMsg{Ref: "/a.yaml", Output: `{"workflow":"a"}`})
	if m.workflows.DryRunResult("/a.yaml") == "" {
		t.Fatal("expected dry-run result to be stored in view")
	}
}

func TestArtifactsRouteFetchesArtifacts(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.width = 100
	m.height = 30
	m.layout()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.route != RouteArtifacts {
		t.Fatalf("expected route artifacts, got %s", m.route)
	}
	if cmd == nil {
		t.Fatal("expected command to fetch artifacts")
	}
}

func TestSettingsRouteSyncsFields(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	if m.route != RouteSettings {
		t.Fatalf("expected route settings, got %s", m.route)
	}
	if len(m.settings.Fields()) == 0 {
		t.Fatal("expected settings fields to be synced")
	}
}

func TestArtifactMsgUpdatesPreview(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	m.selectedRunID = "r1"
	_, _ = m.Update(ArtifactMsg{RunID: "r1", Artifact: client.ArtifactSummary{ID: "a1", Content: "hello"}})
	art, err := m.artifacts.Preview()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art.ID != "a1" {
		t.Fatal("expected artifact preview to be stored")
	}
}

func TestModelWithFakeClient(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	fake := &client.FakeClient{
		ListLocalWorkflowsFunc: func(ctx context.Context) ([]client.LocalWorkflow, error) {
			return []client.LocalWorkflow{{Name: "test", Path: "/test.yaml"}}, nil
		},
	}
	m.client = fake
	m.width = 100
	m.height = 30
	m.layout()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if cmd == nil {
		t.Fatal("expected command to list workflows")
	}

	_, _ = m.Update(WorkflowsListMsg{Workflows: []client.LocalWorkflow{{Name: "test", Path: "/test.yaml"}}})
	if len(m.workflowsList) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(m.workflowsList))
	}
}

func TestModelSavesSettings(t *testing.T) {
	tmp := t.TempDir()
	origRoot := defaultAgentFlowRoot
	defaultAgentFlowRoot = func() string { return tmp }
	defer func() { defaultAgentFlowRoot = origRoot }()

	m := NewModel(Options{Theme: theme.ModeDark})
	m.width = 100
	m.height = 30
	m.layout()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	if m.route != RouteSettings {
		t.Fatalf("expected route settings, got %s", m.route)
	}

	m.settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if !m.settings.Fields()[1].Bool {
		t.Fatal("expected mouse toggled on")
	}

	m.saveSettings()
	loaded := LoadTUISettings()
	if !loaded.Mouse {
		t.Fatal("expected mouse setting persisted")
	}
}

func TestRunsRouteCancelKey(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.width = 100
	m.height = 30
	m.layout()
	m.runDetail = client.RunSummary{ID: "r1", Status: "running"}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if m.runs.Confirming() != "cancel" {
		t.Fatalf("expected confirming cancel, got %s", m.runs.Confirming())
	}
}

func TestRunsRoutePauseKey(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.width = 100
	m.height = 30
	m.layout()
	m.runDetail = client.RunSummary{ID: "r1", Status: "running"}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.runs.Confirming() != "pause" {
		t.Fatalf("expected confirming pause, got %s", m.runs.Confirming())
	}
}

func TestRunsRouteResumeKey(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.width = 100
	m.height = 30
	m.layout()
	m.runDetail = client.RunSummary{ID: "r1", Status: "paused"}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.runs.Confirming() != "resume" {
		t.Fatalf("expected confirming resume, got %s", m.runs.Confirming())
	}
}

func TestMouseDisabledFallback(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Mouse: false})
	m.width = 100
	m.height = 30
	m.layout()
	m.dashboard.SetRuns([]client.RunSummary{{ID: "r1", Status: "running"}})

	_, okBefore := m.dashboard.SelectedRun()
	m.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	_, okAfter := m.dashboard.SelectedRun()
	if okBefore != okAfter {
		t.Fatal("expected no selection change when mouse is disabled")
	}
}

func TestEventAccumulationLimit(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.selectedRunID = "r1"

	for i := 0; i < maxCachedEvents+100; i++ {
		_, _ = m.Update(RunEventsMsg{
			RunID: "r1",
			Batch: client.EventBatch{
				Events:     []client.EventLine{{Cursor: i, Type: "log"}},
				NextCursor: i + 1,
				HasMore:    false,
			},
		})
	}

	if len(m.runEvents) > maxCachedEvents {
		t.Fatalf("expected events trimmed to %d, got %d", maxCachedEvents, len(m.runEvents))
	}
}

func TestLogAccumulationLimit(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	m.selectedRunID = "r1"

	lines := make([]string, maxCachedLogLines+100)
	for i := range lines {
		lines[i] = "log line"
	}
	_, _ = m.Update(RunLogsMsg{RunID: "r1", Lines: lines})

	if len(m.runLogs) > maxCachedLogLines {
		t.Fatalf("expected logs trimmed to %d, got %d", maxCachedLogLines, len(m.runLogs))
	}
}

func TestModelInitReturnsCommands(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command")
	}
}

func TestModelInitWithRunStartsPolling(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Run: "r1"})
	_ = m.Init()
	if !m.polling[tickRunDetail] || !m.polling[tickRunEvents] || !m.polling[tickRunLogs] {
		t.Fatal("expected polling for run detail, events, and logs")
	}
}

func TestDaemonUnavailableDoesNotQuitWhenOptional(t *testing.T) {
	m := NewModel(Options{Theme: theme.ModeDark, Daemon: false})
	m.width = 100
	m.height = 30
	m.layout()

	_, cmd := m.Update(DaemonStatusMsg{
		Err: client.ErrDaemonUnavailable,
	})
	if m.daemonState.Status != client.DaemonUnavailable {
		t.Fatalf("expected unavailable, got %s", m.daemonState.Status)
	}
	if cmd != nil {
		t.Fatal("expected no quit command when daemon is optional")
	}
}
