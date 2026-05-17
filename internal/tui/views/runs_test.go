package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/animation"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

func TestNewRunsInitialState(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	if r.width != 0 {
		t.Fatalf("expected width 0, got %d", r.width)
	}
	if r.selectedNode != -1 {
		t.Fatalf("expected selectedNode -1, got %d", r.selectedNode)
	}
}

func TestSetRun(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetRun(client.RunSummary{ID: "r1", Workflow: "wf1", Status: "running"})
	if r.run.ID != "r1" {
		t.Fatal("expected run set")
	}
}

func TestSetNodes(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{
		{NodeID: "n1", Status: "success"},
		{NodeID: "n2", Status: "failed"},
	})
	if len(r.nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(r.nodes))
	}
	if r.selectedNode != 0 {
		t.Fatalf("expected selectedNode 0, got %d", r.selectedNode)
	}
}

func TestSetNodesResetsSelection(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{{NodeID: "n1"}})
	r.selectedNode = 0
	r.SetNodes([]client.NodeSummary{})
	if r.selectedNode != -1 {
		t.Fatalf("expected selectedNode -1 after empty nodes, got %d", r.selectedNode)
	}
}

func TestRunsCursorNavigation(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{
		{NodeID: "n1", Status: "success"},
		{NodeID: "n2", Status: "failed"},
		{NodeID: "n3", Status: "running"},
	})
	r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if r.selectedNode != 1 {
		t.Fatalf("expected selectedNode 1, got %d", r.selectedNode)
	}
	r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if r.selectedNode != 0 {
		t.Fatalf("expected selectedNode 0, got %d", r.selectedNode)
	}
	r.selectedNode = 2
	r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if r.selectedNode != 2 {
		t.Fatalf("expected selectedNode clamped at 2, got %d", r.selectedNode)
	}
}

func TestRunsEscDeselectsNode(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{{NodeID: "n1"}})
	r.selectedNode = 0
	r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if r.selectedNode != -1 {
		t.Fatalf("expected selectedNode -1, got %d", r.selectedNode)
	}
}

func TestRunsSelectedNode(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "success"}})
	n, ok := r.SelectedNode()
	if !ok || n.NodeID != "n1" {
		t.Fatal("expected selected node")
	}
	r.selectedNode = -1
	_, ok = r.SelectedNode()
	if ok {
		t.Fatal("expected no selected node")
	}
}

func TestRunsViewRendersHeader(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Workflow: "wf1", Status: "running"})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "wf1") {
		t.Fatal("expected workflow in view")
	}
	if !strings.Contains(v, "r1") {
		t.Fatal("expected run id in view")
	}
}

func TestRunsViewRendersProgress(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Workflow: "wf1", Status: "running", TotalSteps: 5, CompletedSteps: []string{"a", "b"}})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Progress") {
		t.Fatal("expected progress in view")
	}
}

func TestRunsViewRendersNodeList(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "success"}})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "n1") {
		t.Fatal("expected node in view")
	}
}

func TestRunsViewRendersNodeDetail(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(120, 30)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "failed", Error: "oops", Stdout: "out", Stderr: "err", Duration: 1000, Attempts: 2}})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "n1") {
		t.Fatal("expected node id in detail")
	}
	if !strings.Contains(v, "oops") {
		t.Fatal("expected error in detail")
	}
	if !strings.Contains(v, "out") {
		t.Fatal("expected stdout in detail")
	}
	if !strings.Contains(v, "err") {
		t.Fatal("expected stderr in detail")
	}
}

func TestRunsViewRendersTimelineWhenNoNodeSelected(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(120, 30)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "success"}})
	r.selectedNode = -1
	r.SetEvents([]client.EventLine{{Type: "start", Message: "started"}})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Timeline") {
		t.Fatal("expected timeline in view")
	}
	if !strings.Contains(v, "started") {
		t.Fatal("expected event in timeline")
	}
}

func TestRunsViewRendersDiagnostics(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(120, 30)
	r.SetRun(client.RunSummary{
		ID:     "r1",
		Status: "failed",
		DiagnosticSummary: &client.RunDiagnosticSummary{
			DurationMS:    5000,
			FailedNodes:   2,
			Retries:       3,
			AgentCalls:    4,
			BashCalls:     5,
			ArtifactCount: 6,
			FirstError:    "boom",
			SlowestNodes:  []client.SlowestNode{{NodeID: "slow", DurationMS: 4000}},
			AgentUsage:    []client.AgentUsage{{Provider: "openai", TotalTokens: 100}},
		},
	})
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "success"}})
	r.selectedNode = -1
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Diagnostics") {
		t.Fatal("expected diagnostics header in view")
	}
	if !strings.Contains(v, "Duration:") {
		t.Fatal("expected duration in diagnostics")
	}
	if !strings.Contains(v, "Failed:") {
		t.Fatal("expected failed nodes in diagnostics")
	}
	if !strings.Contains(v, "Retries:") {
		t.Fatal("expected retries in diagnostics")
	}
}

func TestRunsViewRendersDiagnosticsEmpty(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(120, 30)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "success"}})
	r.selectedNode = -1
	v := r.View(theme.Default(theme.ModeDark))
	if strings.Contains(v, "Diagnostics") {
		t.Fatal("expected no diagnostics header when empty")
	}
}

func TestRunsConfirming(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetConfirming("cancel")
	if r.Confirming() != "cancel" {
		t.Fatal("expected confirming cancel")
	}
	r.SetConfirming("")
	if r.Confirming() != "" {
		t.Fatal("expected confirming empty")
	}
}

func TestRunsHintsShowControls(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "cancel") {
		t.Fatal("expected cancel hint")
	}
	if !strings.Contains(v, "pause") {
		t.Fatal("expected pause hint")
	}
}

func TestRunsHintsHideResumeWhenNotPaused(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	v := r.View(theme.Default(theme.ModeDark))
	if strings.Contains(v, "resume") {
		t.Fatal("expected no resume hint for running run")
	}
}

func TestRunsHintsShowResumeWhenPaused(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Status: "paused"})
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "resume") {
		t.Fatal("expected resume hint for paused run")
	}
}

func TestRunsViewRendersConfirmation(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetRun(client.RunSummary{ID: "r1", Status: "running"})
	r.SetConfirming("cancel")
	v := r.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Confirm cancel") {
		t.Fatal("expected confirmation prompt")
	}
}

func TestRunsMouseDisabled(t *testing.T) {
	r := NewRuns(nil, animation.NewConfig(true))
	r.SetNodes([]client.NodeSummary{{NodeID: "n1", Status: "running"}})
	prev := r.selectedNode
	r.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	if r.selectedNode != prev {
		t.Fatal("expected no selection change when mouse disabled")
	}
}

func TestRunsMouseEnabledClicksRow(t *testing.T) {
	zm := zone.New()
	r := NewRuns(zm, animation.NewConfig(true))
	r.SetSize(80, 24)
	r.SetNodes([]client.NodeSummary{
		{NodeID: "n1", Status: "running"},
		{NodeID: "n2", Status: "success"},
	})
	_ = r.View(theme.Default(theme.ModeDark))
	zm.Scan(strings.Repeat("\n", 30))
	r.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	// Should not panic.
}
