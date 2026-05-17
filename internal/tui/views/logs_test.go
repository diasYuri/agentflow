package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestNewLogsInitialState(t *testing.T) {
	l := NewLogs()
	if l.width != 0 {
		t.Fatalf("expected width 0, got %d", l.width)
	}
}

func TestSetLines(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.SetLines([]string{"line1", "line2"})
	if len(l.lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(l.lines))
	}
	if !strings.Contains(l.vp.View(), "line1") {
		t.Fatal("expected line1 in viewport")
	}
}

func TestSetEvents(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.showEvents = true
	l.SetEvents([]client.EventLine{
		{Timestamp: time.Now(), Type: "start", Message: "hello", NodeID: "n1"},
	})
	if len(l.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(l.events))
	}
	if !strings.Contains(l.vp.View(), "hello") {
		t.Fatal("expected event message in viewport")
	}
}

func TestToggleEvents(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.SetLines([]string{"raw log"})
	l.SetEvents([]client.EventLine{{Timestamp: time.Now(), Type: "start", Message: "event msg"}})

	if !strings.Contains(l.vp.View(), "raw log") {
		t.Fatal("expected raw log initially")
	}

	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !strings.Contains(l.vp.View(), "event msg") {
		t.Fatal("expected event msg after toggle")
	}
}

func TestTextFilter(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.SetLines([]string{"alpha", "beta"})
	l.textFilter = "alp"
	l.rebuildContent()
	if strings.Contains(l.vp.View(), "beta") {
		t.Fatal("expected beta filtered out")
	}
	if !strings.Contains(l.vp.View(), "alpha") {
		t.Fatal("expected alpha to remain")
	}
}

func TestNodeFilter(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.SetEvents([]client.EventLine{
		{Timestamp: time.Now(), Type: "start", Message: "m1", NodeID: "n1"},
		{Timestamp: time.Now(), Type: "end", Message: "m2", NodeID: "n2"},
	})
	l.showEvents = true
	l.nodeFilter = "n1"
	l.rebuildContent()
	if strings.Contains(l.vp.View(), "m2") {
		t.Fatal("expected m2 filtered out")
	}
	if !strings.Contains(l.vp.View(), "m1") {
		t.Fatal("expected m1 to remain")
	}
}

func TestTypeFilter(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.SetEvents([]client.EventLine{
		{Timestamp: time.Now(), Type: "start", Message: "m1"},
		{Timestamp: time.Now(), Type: "end", Message: "m2"},
	})
	l.showEvents = true
	l.typeFilter = "end"
	l.rebuildContent()
	if strings.Contains(l.vp.View(), "m1") {
		t.Fatal("expected m1 filtered out")
	}
	if !strings.Contains(l.vp.View(), "m2") {
		t.Fatal("expected m2 to remain")
	}
}

func TestLogsFilterMode(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !l.filtering {
		t.Fatal("expected filtering mode")
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if l.textFilter != "x" {
		t.Fatalf("expected text filter 'x', got %s", l.textFilter)
	}
	l.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if l.filtering {
		t.Fatal("expected filtering to stop")
	}
}

func TestFilterFieldSwitch(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if l.textFilter != "a" {
		t.Fatalf("expected text filter 'a', got %s", l.textFilter)
	}
	l.Update(tea.KeyMsg{Type: tea.KeyTab})
	if l.filterField != 1 {
		t.Fatalf("expected filterField 1, got %d", l.filterField)
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if l.nodeFilter != "b" {
		t.Fatalf("expected node filter 'b', got %s", l.nodeFilter)
	}
}

func TestLogsViewRendersFilterBar(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	v := l.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Filter") {
		t.Fatal("expected filter bar in view")
	}
}

func TestLogsViewRendersModeHint(t *testing.T) {
	l := NewLogs()
	l.SetSize(80, 24)
	v := l.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "logs") {
		t.Fatal("expected mode hint in view")
	}
}
