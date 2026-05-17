package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/components"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

const zoneDashRunPrefix = "dash_run_"

// Dashboard is the dashboard screen view.
type Dashboard struct {
	width, height int
	state         client.DaemonState
	runs          []client.RunSummary
	cursor        int
	zoneManager   *zone.Manager
}

// NewDashboard creates a new Dashboard view.
func NewDashboard(zm *zone.Manager) *Dashboard {
	return &Dashboard{zoneManager: zm}
}

// SetSize sets the view size.
func (d *Dashboard) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Update handles messages.
func (d *Dashboard) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		d.width = m.Width
		d.height = m.Height
	case tea.KeyMsg:
		switch m.String() {
		case "j", "down":
			if d.cursor < len(d.runs)-1 {
				d.cursor++
			}
		case "k", "up":
			if d.cursor > 0 {
				d.cursor--
			}
		}
	case tea.MouseMsg:
		if d.zoneManager == nil || m.Action != tea.MouseActionRelease || m.Button != tea.MouseButtonLeft {
			return
		}
		for i := range d.runs {
			zi := d.zoneManager.Get(fmt.Sprintf("%s%d", zoneDashRunPrefix, i))
			if zi != nil && zi.InBounds(m) {
				d.cursor = i
				return
			}
		}
	}
}

// View implements tea.Model.
func (d *Dashboard) View(t *theme.Theme) string {
	if d.width == 0 || d.height == 0 {
		return "Loading..."
	}
	var b strings.Builder

	b.WriteString(t.Title.Render("Dashboard") + "\n\n")
	b.WriteString(d.renderDaemonStatus(t) + "\n")
	b.WriteString(d.renderCounts(t) + "\n")
	b.WriteString(d.renderRecentRuns(t) + "\n")
	b.WriteString(t.Muted.Render("j/k navigate • enter open run • 3 runs"))

	style := lipgloss.NewStyle().Width(d.width).Height(d.height)
	return style.Render(b.String())
}

// SetDaemonState updates the displayed daemon state.
func (d *Dashboard) SetDaemonState(state client.DaemonState) {
	d.state = state
}

// SetRuns updates the displayed runs.
func (d *Dashboard) SetRuns(runs []client.RunSummary) {
	d.runs = runs
	if d.cursor >= len(d.runs) {
		d.cursor = max(0, len(d.runs)-1)
	}
}

// SelectedRun returns the currently selected run.
func (d *Dashboard) SelectedRun() (client.RunSummary, bool) {
	if d.cursor >= 0 && d.cursor < len(d.runs) {
		return d.runs[d.cursor], true
	}
	return client.RunSummary{}, false
}

func (d *Dashboard) renderDaemonStatus(t *theme.Theme) string {
	var b strings.Builder
	b.WriteString(t.Subtitle.Render("Daemon") + "\n")
	status := string(d.state.Status)
	switch d.state.Status {
	case client.DaemonAvailable:
		status = components.StatusBadge(t, "online")
	case client.DaemonUnavailable:
		status = components.StatusBadge(t, "offline")
	default:
		status = components.StatusBadge(t, status)
	}
	b.WriteString(fmt.Sprintf("Status: %s\n", status))
	if d.state.Status == client.DaemonAvailable {
		uptime := time.Since(d.state.StartedAt).Round(time.Second)
		b.WriteString(t.Muted.Render(fmt.Sprintf("PID: %d • Socket: %s • Uptime: %s • Runs: %d", d.state.PID, d.state.Socket, uptime, d.state.Runs)) + "\n")
	}
	return b.String()
}

func (d *Dashboard) renderCounts(t *theme.Theme) string {
	counts := make(map[string]int)
	for _, r := range d.runs {
		counts[strings.ToLower(r.Status)]++
	}
	var parts []string
	for _, s := range []string{"running", "created", "success", "failed", "paused", "cancelled"} {
		if c := counts[s]; c > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", components.StatusBadge(t, s), c))
		}
	}
	if len(parts) == 0 {
		return t.Subtitle.Render("Runs") + "\n" + t.Muted.Render("No runs")
	}
	chartHeight := 6
	if d.height > 20 && d.width > 40 {
		return components.StatusBarChart(t, d.width, chartHeight, counts)
	}
	return t.Subtitle.Render("Runs") + "\n" + strings.Join(parts, "  ")
}

func (d *Dashboard) renderRecentRuns(t *theme.Theme) string {
	var b strings.Builder
	b.WriteString(t.Subtitle.Render("Recent") + "\n")
	visible := d.height - 14
	if visible < 3 {
		visible = 3
	}
	start := 0
	if d.cursor >= visible {
		start = d.cursor - visible + 1
	}
	for i := start; i < len(d.runs) && i < start+visible; i++ {
		r := d.runs[i]
		prefix := "  "
		if i == d.cursor {
			prefix = t.Primary.Render("► ")
		}
		line := prefix + components.StatusBadge(t, r.Status) + " " + trunc(r.Workflow+" "+r.ID, d.width-30)
		if diag := r.DiagnosticSummary; diag != nil && diag.DurationMS > 0 {
			line += " " + t.Muted.Render((time.Duration(diag.DurationMS) * time.Millisecond).String())
		} else if !r.StartedAt.IsZero() {
			line += " " + t.Muted.Render(r.StartedAt.Format("15:04:05"))
		}
		if d.zoneManager != nil {
			line = d.zoneManager.Mark(fmt.Sprintf("%s%d", zoneDashRunPrefix, i), line)
		}
		b.WriteString(line + "\n")
		if diag := r.DiagnosticSummary; diag != nil {
			var hints []string
			if diag.FailedNodes > 0 {
				hints = append(hints, t.Danger.Render(fmt.Sprintf("failed:%d", diag.FailedNodes)))
			}
			if diag.Retries > 0 {
				hints = append(hints, t.Warning.Render(fmt.Sprintf("retries:%d", diag.Retries)))
			}
			if diag.AgentCalls > 0 {
				hints = append(hints, fmt.Sprintf("agents:%d", diag.AgentCalls))
			}
			if diag.BashCalls > 0 {
				hints = append(hints, fmt.Sprintf("bash:%d", diag.BashCalls))
			}
			if diag.ArtifactCount > 0 {
				hints = append(hints, fmt.Sprintf("artifacts:%d", diag.ArtifactCount))
			}
			if len(hints) > 0 {
				b.WriteString("      " + t.Muted.Render(strings.Join(hints, "  ")) + "\n")
			}
		}
	}
	if len(d.runs) == 0 {
		b.WriteString(t.Muted.Render("No recent runs") + "\n")
	}
	return b.String()
}
