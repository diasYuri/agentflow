package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// FilterBar renders a filter input bar.
func FilterBar(t *theme.Theme, width int, label, value string, active bool) string {
	var content string
	if active {
		content = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Render("Filter ") + label + ": " + value + "_"
	} else {
		content = t.Muted.Render("Filter ") + label + ": " + value
	}
	style := lipgloss.NewStyle().Width(width)
	return style.Render(content)
}
