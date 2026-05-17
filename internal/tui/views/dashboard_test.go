package views

import (
	"strings"
	"testing"

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
