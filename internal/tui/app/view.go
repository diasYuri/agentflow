package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var b strings.Builder

	// Status bar.
	b.WriteString(m.statusBar.View(m.theme, m.width))
	b.WriteString("\n")

	// Main area.
	mainContent := m.currentView().View(m.theme)
	if m.width > 60 {
		sidebar := m.sidebar.View(m.theme)
		row := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainContent)
		b.WriteString(row)
	} else {
		b.WriteString(mainContent)
	}

	b.WriteString("\n")

	// Optional bottom panel (reserved for future plans).
	bpHeight := m.bottomPanelHeight()
	if bpHeight > 0 {
		bpStyle := lipgloss.NewStyle().Width(m.width).Height(bpHeight)
		b.WriteString(bpStyle.Render(m.theme.Muted.Render("")))
		b.WriteString("\n")
	}

	// Footer help.
	if m.showHelp {
		bindings := m.FullHelp()
		var parts []string
		for _, group := range bindings {
			var groupParts []string
			for _, kb := range group {
				groupParts = append(groupParts, kb.Help().Key+":"+kb.Help().Desc)
			}
			parts = append(parts, strings.Join(groupParts, "  "))
		}
		b.WriteString(m.theme.FooterBg.Width(m.width).Render(strings.Join(parts, " | ")))
	} else {
		b.WriteString(m.keyHelp.View(m.theme, m.width))
	}

	if m.zoneManager != nil {
		return m.zoneManager.Scan(b.String())
	}
	return b.String()
}
