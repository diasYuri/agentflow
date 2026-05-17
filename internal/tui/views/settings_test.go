package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestNewSettingsInitialState(t *testing.T) {
	s := NewSettings()
	if s.width != 0 {
		t.Fatalf("expected width 0, got %d", s.width)
	}
	if s.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", s.cursor)
	}
}

func TestSetFields(t *testing.T) {
	s := NewSettings()
	s.SetFields([]SettingField{
		{Label: "Theme", Key: "theme", Value: "dark"},
		{Label: "Mouse", Key: "mouse", IsBool: true, Bool: true},
	})
	if len(s.fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.fields))
	}
}

func TestSettingsCursorNavigation(t *testing.T) {
	s := NewSettings()
	s.SetFields([]SettingField{
		{Label: "A", Key: "a"},
		{Label: "B", Key: "b"},
		{Label: "C", Key: "c"},
	})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if s.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", s.cursor)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if s.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", s.cursor)
	}
}

func TestSettingsToggleBool(t *testing.T) {
	s := NewSettings()
	s.SetFields([]SettingField{{Label: "Mouse", Key: "mouse", IsBool: true, Bool: false}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if !s.fields[0].Bool {
		t.Fatal("expected bool toggled to true")
	}
	if !s.changed {
		t.Fatal("expected changed flag")
	}
}

func TestSettingsCycleOption(t *testing.T) {
	s := NewSettings()
	s.SetFields([]SettingField{{Label: "Theme", Key: "theme", Value: "dark", Options: []string{"auto", "dark", "light"}}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if s.fields[0].Value != "light" {
		t.Fatalf("expected cycled to light, got %s", s.fields[0].Value)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if s.fields[0].Value != "auto" {
		t.Fatalf("expected cycled to auto, got %s", s.fields[0].Value)
	}
}

func TestSettingsEditText(t *testing.T) {
	s := NewSettings()
	s.SetFields([]SettingField{{Label: "Path", Key: "path", Value: "/old"}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	if !s.editing {
		t.Fatal("expected editing mode")
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if s.fields[0].Value != "/oldab" {
		t.Fatalf("expected value /oldab, got %s", s.fields[0].Value)
	}
	if s.editing {
		t.Fatal("expected editing done")
	}
}

func TestSettingsViewRendersFields(t *testing.T) {
	s := NewSettings()
	s.SetSize(80, 24)
	s.SetFields([]SettingField{
		{Label: "Theme", Key: "theme", Value: "dark", Options: []string{"auto", "dark", "light"}},
		{Label: "Mouse", Key: "mouse", IsBool: true, Bool: true},
	})
	v := s.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Theme") {
		t.Fatal("expected Theme label")
	}
	if !strings.Contains(v, "Mouse") {
		t.Fatal("expected Mouse label")
	}
}

func TestSettingsSaveHint(t *testing.T) {
	s := NewSettings()
	s.SetSize(80, 24)
	s.SetFields([]SettingField{{Label: "Mouse", Key: "mouse", IsBool: true, Bool: false}})
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	v := s.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Unsaved changes") {
		t.Fatal("expected unsaved changes hint")
	}
}

func TestSettingsSetChanged(t *testing.T) {
	s := NewSettings()
	s.SetChanged(true)
	if !s.Changed() {
		t.Fatal("expected changed true")
	}
	s.SetChanged(false)
	if s.Changed() {
		t.Fatal("expected changed false")
	}
}
