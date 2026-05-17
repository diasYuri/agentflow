package components

import "github.com/charmbracelet/lipgloss"

// trunc truncates a string to a maximum visual width, adding an ellipsis if truncated.
func trunc(s string, max int) string {
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
