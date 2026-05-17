package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// KeyHelp renders the footer keybindings help.
type KeyHelp struct {
	bindings []Binding
}

// Binding represents a single keybinding description.
type Binding struct {
	Key         string
	Description string
}

// NewKeyHelp creates a new KeyHelp component.
func NewKeyHelp() *KeyHelp {
	return &KeyHelp{
		bindings: []Binding{
			{Key: "q", Description: "quit"},
			{Key: "tab/↑↓", Description: "navigate"},
			{Key: "1-6", Description: "jump"},
		},
	}
}

// SetBindings updates the displayed bindings.
func (kh *KeyHelp) SetBindings(bindings []Binding) {
	kh.bindings = bindings
}

// View renders the footer help line.
func (kh *KeyHelp) View(t *theme.Theme, width int) string {
	if width == 0 {
		return ""
	}
	var parts []string
	for _, b := range kh.bindings {
		key := t.Primary.Render(b.Key)
		desc := t.Muted.Render(b.Description)
		parts = append(parts, key+":"+desc)
	}
	content := strings.Join(parts, "  ")
	content = truncWidth(content, width)
	return t.FooterBg.Width(width).Render(content)
}

func truncWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	runes := []rune(s)
	w := 0
	for i, r := range runes {
		rw := lipgloss.Width(string(r))
		if w+rw > max-1 {
			return string(runes[:i]) + "…"
		}
		w += rw
	}
	return s
}
