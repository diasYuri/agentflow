package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// StatusBar renders the top status bar.
type StatusBar struct {
	title string
}

// NewStatusBar creates a new StatusBar.
func NewStatusBar(title string) *StatusBar {
	return &StatusBar{title: title}
}

// SetTitle updates the status bar title.
func (sb *StatusBar) SetTitle(title string) {
	sb.title = title
}

// View renders the status bar.
func (sb *StatusBar) View(t *theme.Theme, width int) string {
	if width == 0 {
		return ""
	}
	left := t.HeaderBg.Render(" " + sb.title + " ")
	rightText := "agentflow tui"
	right := t.HeaderBg.Render(" " + rightText + " ")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	mid := t.HeaderBg.Render(strings.Repeat(" ", gap))
	return left + mid + right
}
