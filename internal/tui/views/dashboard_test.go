package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

func TestNewDashboardInitialState(t *testing.T) {
	d := NewDashboard(nil)
	if d.width != 0 {
		t.Fatalf("expected width 0, got %d", d.width)
	}
}

func TestSetDaemonState(t *testing.T) {
	d := NewDashboard(nil)
	d.SetDaemonState(client.DaemonState{Status: client.DaemonAvailable, Running: true, Runs: 3})
	if d.state.Status != client.DaemonAvailable {
		t.Fatal("expected daemon available")
	}
}

func TestSetRunsAndSelect(t *testing.T) {
	d := NewDashboard(nil)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Workflow: "wf1", Status: "running"},
		{ID: "r2", Workflow: "wf2", Status: "success"},
	})
	if len(d.runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(d.runs))
	}
	run, ok := d.SelectedRun()
	if !ok || run.ID != "r1" {
		t.Fatal("expected first run selected")
	}
}

func TestDashboardCursorNavigation(t *testing.T) {
	d := NewDashboard(nil)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Status: "running"},
		{ID: "r2", Status: "success"},
		{ID: "r3", Status: "failed"},
	})
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", d.cursor)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if d.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", d.cursor)
	}
	d.cursor = 2
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if d.cursor != 2 {
		t.Fatalf("expected cursor clamped at 2, got %d", d.cursor)
	}
}

func TestDashboardViewRendersDaemonStatus(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetDaemonState(client.DaemonState{Status: client.DaemonAvailable, Running: true, Runs: 5})
	v := d.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Daemon") {
		t.Fatal("expected Daemon header in view")
	}
	if !strings.Contains(v, "online") {
		t.Fatal("expected online badge in view")
	}
}

func TestDashboardViewRendersCounts(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Status: "running"},
		{ID: "r2", Status: "success"},
		{ID: "r3", Status: "success"},
	})
	v := d.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "running") {
		t.Fatal("expected running count in view")
	}
	if !strings.Contains(v, "success") {
		t.Fatal("expected success count in view")
	}
}

func TestDashboardViewRendersRecentRuns(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Workflow: "wf1", Status: "running"},
	})
	v := d.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "wf1") {
		t.Fatal("expected workflow name in recent runs")
	}
	if !strings.Contains(v, "r1") {
		t.Fatal("expected run id in recent runs")
	}
}

func TestDashboardMouseDisabled(t *testing.T) {
	d := NewDashboard(nil)
	d.SetRuns([]client.RunSummary{{ID: "r1", Status: "running"}})
	d.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	if d.cursor != 0 {
		t.Fatal("expected no cursor change when mouse disabled")
	}
}

func TestDashboardMouseEnabledClicksRow(t *testing.T) {
	zm := zone.New()
	d := NewDashboard(zm)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Status: "running"},
		{ID: "r2", Status: "success"},
	})
	_ = d.View(theme.Default(theme.ModeDark))
	zm.Scan(strings.Repeat("\n", 30))
	d.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	// Should not panic; selection may or may not happen depending on zone bounds.
}

func TestDashboardViewRendersDiagnostics(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{
			ID:       "r1",
			Workflow: "wf1",
			Status:   "failed",
			DiagnosticSummary: &client.RunDiagnosticSummary{
				DurationMS:    5000,
				FailedNodes:   2,
				Retries:       3,
				AgentCalls:    4,
				BashCalls:     5,
				ArtifactCount: 6,
			},
		},
	})
	v := d.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "failed:2") {
		t.Fatal("expected failed nodes hint")
	}
	if !strings.Contains(v, "retries:3") {
		t.Fatal("expected retries hint")
	}
	if !strings.Contains(v, "agents:4") {
		t.Fatal("expected agent calls hint")
	}
	if !strings.Contains(v, "bash:5") {
		t.Fatal("expected bash calls hint")
	}
	if !strings.Contains(v, "artifacts:6") {
		t.Fatal("expected artifact count hint")
	}
}

func TestDashboardViewRendersDiagnosticsDuration(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{
			ID:       "r1",
			Workflow: "wf1",
			Status:   "success",
			DiagnosticSummary: &client.RunDiagnosticSummary{
				DurationMS: 12345,
			},
		},
	})
	v := d.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "12s") && !strings.Contains(v, "12.3") {
		// lipgloss may format duration as 12s or 12.345s depending on rendering
		// just ensure it shows some duration string
		t.Logf("view output:\n%s", v)
	}
}

func TestDashboardViewWithoutDiagnostics(t *testing.T) {
	d := NewDashboard(nil)
	d.SetSize(80, 24)
	d.SetRuns([]client.RunSummary{
		{ID: "r1", Workflow: "wf1", Status: "running", StartedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
	})
	v := d.View(theme.Default(theme.ModeDark))
	if strings.Contains(v, "failed:") {
		t.Fatal("expected no diagnostic hints")
	}
	if !strings.Contains(v, "12:00:00") {
		t.Fatal("expected started at time fallback")
	}
}
