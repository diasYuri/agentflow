package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/components"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// Logs is the logs screen view.
type Logs struct {
	width, height int
	lines         []string
	events        []client.EventLine
	showEvents    bool
	textFilter    string
	nodeFilter    string
	typeFilter    string
	filterField   int
	filtering     bool
	vp            viewport.Model
}

// NewLogs creates a new Logs view.
func NewLogs() *Logs {
	l := &Logs{}
	l.vp = viewport.New(0, 0)
	return l
}

// SetSize sets the view size.
func (l *Logs) SetSize(w, h int) {
	l.width = w
	l.height = h
	l.vp.Width = w
	l.vp.Height = h - 3 // reserve space for filter bar
	if l.vp.Height < 3 {
		l.vp.Height = 3
	}
}

// Update handles messages.
func (l *Logs) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = m.Width
		l.height = m.Height
		l.vp.Width = m.Width
		l.vp.Height = m.Height - 3
		if l.vp.Height < 3 {
			l.vp.Height = 3
		}
	case tea.KeyMsg:
		if l.filtering {
			switch m.Type {
			case tea.KeyEsc, tea.KeyEnter:
				l.filtering = false
			case tea.KeyTab:
				l.filterField = (l.filterField + 1) % 3
			case tea.KeyBackspace:
				switch l.filterField {
				case 0:
					if len(l.textFilter) > 0 {
						l.textFilter = l.textFilter[:len(l.textFilter)-1]
					}
				case 1:
					if len(l.nodeFilter) > 0 {
						l.nodeFilter = l.nodeFilter[:len(l.nodeFilter)-1]
					}
				case 2:
					if len(l.typeFilter) > 0 {
						l.typeFilter = l.typeFilter[:len(l.typeFilter)-1]
					}
				}
				l.rebuildContent()
			case tea.KeyRunes:
				switch l.filterField {
				case 0:
					l.textFilter += string(m.Runes)
				case 1:
					l.nodeFilter += string(m.Runes)
				case 2:
					l.typeFilter += string(m.Runes)
				}
				l.rebuildContent()
			}
			return
		}

		switch m.String() {
		case "/":
			l.filtering = true
		case "tab":
			l.filterField = (l.filterField + 1) % 3
		case "e":
			l.showEvents = !l.showEvents
			l.rebuildContent()
		case "up", "down":
			l.vp, _ = l.vp.Update(m)
		}
	}
}

// View implements tea.Model.
func (l *Logs) View(t *theme.Theme) string {
	if l.width == 0 || l.height == 0 {
		return "Loading..."
	}
	var b strings.Builder

	labels := []string{"text", "node", "type"}
	values := []string{l.textFilter, l.nodeFilter, l.typeFilter}
	var barParts []string
	for i := range labels {
		active := l.filtering && l.filterField == i
		barParts = append(barParts, components.FilterBar(t, 20, labels[i], values[i], active))
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, barParts...) + "\n")

	mode := "logs"
	if l.showEvents {
		mode = "events"
	}
	b.WriteString(t.Muted.Render(fmt.Sprintf("/ filter • e toggle %s • up/down scroll", mode)) + "\n")
	b.WriteString(l.vp.View())

	style := lipgloss.NewStyle().Width(l.width).Height(l.height)
	return style.Render(b.String())
}

// SetLines updates the displayed log lines.
func (l *Logs) SetLines(lines []string) {
	l.lines = lines
	l.rebuildContent()
}

// SetEvents updates the displayed events.
func (l *Logs) SetEvents(events []client.EventLine) {
	l.events = events
	l.rebuildContent()
}

func (l *Logs) rebuildContent() {
	var out []string
	if l.showEvents {
		for _, e := range l.events {
			if !l.matchesEventFilters(e) {
				continue
			}
			ts := e.Timestamp.Format(time.RFC3339)
			if len(ts) > 19 {
				ts = ts[:19]
			}
			line := fmt.Sprintf("%s [%s] %s", ts, e.Type, e.Message)
			if e.NodeID != "" {
				line += fmt.Sprintf(" (%s)", e.NodeID)
			}
			out = append(out, line)
		}
	} else {
		for _, line := range l.lines {
			if l.matchesTextFilter(line) {
				out = append(out, line)
			}
		}
	}
	l.vp.SetContent(strings.Join(out, "\n"))
}

func (l *Logs) matchesEventFilters(e client.EventLine) bool {
	if l.textFilter != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(l.textFilter)) {
		return false
	}
	if l.nodeFilter != "" && e.NodeID != l.nodeFilter {
		return false
	}
	if l.typeFilter != "" && e.Type != l.typeFilter {
		return false
	}
	return true
}

func (l *Logs) matchesTextFilter(line string) bool {
	if l.textFilter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(l.textFilter)) {
		return false
	}
	if l.nodeFilter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(l.nodeFilter)) {
		return false
	}
	if l.typeFilter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(l.typeFilter)) {
		return false
	}
	return true
}
