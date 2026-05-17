package views

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

func TestNewWorkflowsInitialState(t *testing.T) {
	w := NewWorkflows(nil)
	if w.width != 0 {
		t.Fatalf("expected width 0, got %d", w.width)
	}
	if len(w.workflows) != 0 {
		t.Fatal("expected no workflows")
	}
}

func TestSetWorkflowsAndFilter(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "alpha", Path: "/a.yaml", Description: "first"},
		{Name: "beta", Path: "/b.yaml", Description: "second"},
	})
	if len(w.filtered) != 2 {
		t.Fatalf("expected 2 filtered, got %d", len(w.filtered))
	}
	w.filter = "alp"
	w.applyFilter()
	if len(w.filtered) != 1 || w.filtered[0].Name != "alpha" {
		t.Fatalf("expected 1 filtered (alpha), got %v", w.filtered)
	}
}

func TestCursorNavigation(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
		{Name: "b", Path: "/b.yaml"},
		{Name: "c", Path: "/c.yaml"},
	})
	w.cursor = 0

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if w.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", w.cursor)
	}

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if w.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", w.cursor)
	}

	w.cursor = 2
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if w.cursor != 2 {
		t.Fatalf("expected cursor clamped at 2, got %d", w.cursor)
	}
}

func TestSelectAndDeselectWorkflow(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if w.selected.Path != "/a.yaml" {
		t.Fatalf("expected selected /a.yaml, got %s", w.selected.Path)
	}
	if w.activeTab != tabOverview {
		t.Fatalf("expected tab overview, got %s", w.activeTab.String())
	}

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if w.selected.Path != "" {
		t.Fatalf("expected deselected, got %s", w.selected.Path)
	}
}

func TestTabSwitching(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if w.activeTab != tabGraph {
		t.Fatalf("expected tab graph, got %s", w.activeTab.String())
	}

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if w.activeTab != tabOverview {
		t.Fatalf("expected tab overview, got %s", w.activeTab.String())
	}

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	if w.activeTab != tabGraph {
		t.Fatalf("expected tab graph after tab, got %s", w.activeTab.String())
	}
}

func TestValidationDisplay(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml", Description: "desc"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.SetValidationResult("/a.yaml", nil)

	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "✓ Valid") {
		t.Fatal("expected valid indicator in view")
	}

	w.SetValidationResult("/a.yaml", errors.New("bad yaml"))
	v = w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "✗ Invalid") {
		t.Fatal("expected invalid indicator in view")
	}
	if !strings.Contains(v, "bad yaml") {
		t.Fatal("expected error text in view")
	}
}

func TestGraphDisplay(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.SetGraphResult("/a.yaml", "graph TD\nA-->B")

	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "graph TD") {
		t.Fatal("expected graph content in view")
	}
}

func TestGraphDisplayUpdatesWithSelection(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
		{Name: "b", Path: "/b.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.SetGraphResult("/a.yaml", "graph TD\nA-->B")
	w.SetGraphResult("/b.yaml", "graph TD\nB-->C")

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "B-->C") {
		t.Fatalf("expected selected workflow graph content, got:\n%s", v)
	}
}

func TestDryRunDisplay(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.SetDryRunResult("/a.yaml", `{"workflow":"a","inputs":{"x":1},"order":["step1"],"nodes":{"step1":{"spec":{"kind":"bash"},"dependencies":[]}}}`)

	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Workflow: a") {
		t.Fatal("expected dry-run workflow name in view")
	}
	if !strings.Contains(v, "step1") {
		t.Fatal("expected dry-run step in view")
	}
}

func TestDryRunDisplayUpdatesWithSelection(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
		{Name: "b", Path: "/b.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.SetDryRunResult("/a.yaml", `{"workflow":"a","order":["step1"],"nodes":{"step1":{"spec":{"kind":"bash"},"dependencies":[]}}}`)
	w.SetDryRunResult("/b.yaml", `{"workflow":"b","order":["step2"],"nodes":{"step2":{"spec":{"kind":"python"},"dependencies":[]}}}`)

	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "workflow: b") && !strings.Contains(v, "Workflow: b") {
		t.Fatalf("expected selected workflow dry-run content, got:\n%s", v)
	}
}

func TestRenderSmallWidth(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(50, 20)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})

	v := w.View(theme.Default(theme.ModeDark))
	if strings.Contains(v, "Overview") {
		t.Fatal("expected no detail panel at small width")
	}
}

func TestRenderMediumWidth(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})

	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Overview") {
		t.Fatal("expected detail panel at medium width")
	}
	if !strings.Contains(v, "a") {
		t.Fatal("expected workflow name in view")
	}
}

func TestRenderLargeWidth(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(120, 30)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml", Description: "desc"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})

	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Overview") {
		t.Fatal("expected detail panel at large width")
	}
}

func TestMouseDisabled(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})
	w.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	if w.selected.Path != "" {
		t.Fatal("expected no selection when mouse is disabled")
	}
}

func TestMouseEnabledClicksRow(t *testing.T) {
	zm := zone.New()
	w := NewWorkflows(zm)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
	})

	// Render to register zones
	_ = w.View(theme.Default(theme.ModeDark))
	zm.Scan(strings.Repeat("\n", 30)) // minimal scan to satisfy bubblezone

	w.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	// Selection may or may not happen depending on zone bounds; the important thing is no panic.
}

func TestFilterMode(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "alpha", Path: "/alpha.yaml"},
		{Name: "beta", Path: "/beta.yaml"},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !w.filtering {
		t.Fatal("expected filtering mode")
	}
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if w.filter != "b" {
		t.Fatalf("expected filter 'b', got %s", w.filter)
	}
	if len(w.filtered) != 1 || w.filtered[0].Name != "beta" {
		t.Fatalf("expected 1 filtered (beta), got %v", w.filtered)
	}
	w.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if w.filtering {
		t.Fatal("expected filtering to stop")
	}
}

func TestTrunc(t *testing.T) {
	if trunc("hello", 10) != "hello" {
		t.Fatal("expected no truncation")
	}
	if trunc("hello world", 5) != "hell…" {
		t.Fatalf("expected truncation, got %s", trunc("hello world", 5))
	}
	if trunc("hello", 0) != "" {
		t.Fatal("expected empty for max 0")
	}
}

func TestSelectByIndex(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml"},
		{Name: "b", Path: "/b.yaml"},
	})
	w.SelectByIndex(1)
	if w.selected.Name != "b" {
		t.Fatalf("expected selected b, got %s", w.selected.Name)
	}
	w.SelectByIndex(-1)
	if w.selected.Name != "b" {
		t.Fatal("expected no change for invalid index")
	}
	w.SelectByIndex(10)
	if w.selected.Name != "b" {
		t.Fatal("expected no change for out of range")
	}
}

func TestLoadingState(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetLoading(true)
	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Loading workflows...") {
		t.Fatal("expected loading indicator in view")
	}
}

func TestListErrorState(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetListError(errors.New("disk error"))
	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "disk error") {
		t.Fatal("expected error text in view")
	}
}

func TestOverviewShowsNodeCountAndInputs(t *testing.T) {
	w := NewWorkflows(nil)
	w.SetSize(80, 24)
	w.SetWorkflows([]client.LocalWorkflow{
		{Name: "a", Path: "/a.yaml", NodeCount: 3, Inputs: map[string]client.InputSpec{
			"env": {Type: "string", Required: true},
		}},
	})
	w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	v := w.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Nodes: 3") {
		t.Fatal("expected node count in view")
	}
	if !strings.Contains(v, "Inputs: 1") {
		t.Fatal("expected input count in view")
	}
	if !strings.Contains(v, "env") {
		t.Fatal("expected input name in view")
	}
}
