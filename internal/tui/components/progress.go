package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// ProgressBar renders a textual progress bar.
func ProgressBar(t *theme.Theme, width, completed, total int) string {
	if total <= 0 {
		return t.Muted.Render("No steps")
	}
	if width < 10 {
		width = 10
	}
	filled := completed * width / total
	if filled > width {
		filled = width
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).Render(strings.Repeat("░", width-filled))
	pct := completed * 100 / total
	return fmt.Sprintf("%s %d%% (%d/%d)", bar, pct, completed, total)
}
