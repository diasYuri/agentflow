package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// StatusBadge renders a status string as a colored badge.
func StatusBadge(t *theme.Theme, status string) string {
	s := strings.ToLower(status)
	var style lipgloss.Style
	switch s {
	case "running", "created":
		style = t.Primary
	case "success", "completed":
		style = t.Success
	case "failed", "error":
		style = t.Danger
	case "cancelled", "canceled":
		style = t.Muted
	case "paused":
		style = t.Warning
	default:
		style = t.Muted
	}
	return style.Render(" " + status + " ")
}
