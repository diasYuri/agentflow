package views

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/animation"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/components"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

const zoneRunNodePrefix = "run_node_"

// Runs is the run detail screen view.
type Runs struct {
	width, height int
	run           client.RunSummary
	nodes         []client.NodeSummary
	events        []client.EventLine
	selectedNode  int
	zoneManager   *zone.Manager
	confirming    string
	progressAnim  animation.ProgressModel
}

// NewRuns creates a new Runs view.
func NewRuns(zm *zone.Manager, anim animation.Config) *Runs {
	r := &Runs{zoneManager: zm, selectedNode: -1, progressAnim: anim.NewProgressModel()}
	r.progressAnim.SetTarget(0)
	return r
}

// SetSize sets the view size.
func (r *Runs) SetSize(w, h int) {
	r.width = w
	r.height = h
}

// Update handles messages.
func (r *Runs) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = m.Width
		r.height = m.Height
	case tea.KeyMsg:
		switch m.String() {
		case "j", "down":
			if r.selectedNode < len(r.nodes)-1 {
				r.selectedNode++
			}
		case "k", "up":
			if r.selectedNode > 0 {
				r.selectedNode--
			}
		case "enter", "right", "l":
			// selection is implicit via selectedNode
		case "esc", "left", "h":
			r.selectedNode = -1
		}
	case tea.MouseMsg:
		if r.zoneManager == nil || m.Action != tea.MouseActionRelease || m.Button != tea.MouseButtonLeft {
			return
		}
		for i := range r.nodes {
			zi := r.zoneManager.Get(fmt.Sprintf("%s%d", zoneRunNodePrefix, i))
			if zi != nil && zi.InBounds(m) {
				r.selectedNode = i
				return
			}
		}
	}
}

// View implements tea.Model.
func (r *Runs) View(t *theme.Theme) string {
	if r.width == 0 || r.height == 0 {
		return "Loading..."
	}
	var b strings.Builder

	b.WriteString(r.renderHeader(t) + "\n")
	b.WriteString(r.renderProgress(t) + "\n")

	headerLines := 6
	hintLines := 2
	if r.confirming != "" {
		hintLines = 3
	}
	midHeight := r.height - headerLines - hintLines
	if midHeight < 5 {
		midHeight = 5
	}

	mid := r.renderMid(t, midHeight)
	b.WriteString(mid + "\n")
	b.WriteString(r.renderHints(t) + "\n")

	style := lipgloss.NewStyle().Width(r.width).Height(r.height)
	return style.Render(b.String())
}

// SetRun updates the displayed run.
func (r *Runs) SetRun(run client.RunSummary) {
	r.run = run
}

// SetNodes updates the displayed nodes.
func (r *Runs) SetNodes(nodes []client.NodeSummary) {
	r.nodes = nodes
	if r.selectedNode >= len(r.nodes) {
		r.selectedNode = -1
	}
	if r.selectedNode == -1 && len(r.nodes) > 0 {
		r.selectedNode = 0
	}
}

// SetEvents updates the displayed events.
func (r *Runs) SetEvents(events []client.EventLine) {
	r.events = events
}

// SelectedNode returns the currently selected node.
func (r *Runs) SelectedNode() (client.NodeSummary, bool) {
	if r.selectedNode >= 0 && r.selectedNode < len(r.nodes) {
		return r.nodes[r.selectedNode], true
	}
	return client.NodeSummary{}, false
}

// SetConfirming sets the action awaiting confirmation.
func (r *Runs) SetConfirming(action string) {
	r.confirming = action
}

// Confirming returns the action awaiting confirmation.
func (r *Runs) Confirming() string {
	return r.confirming
}

func (r *Runs) renderHeader(t *theme.Theme) string {
	var b strings.Builder
	if r.run.ID == "" {
		b.WriteString(t.Title.Render("Run Detail") + "\n")
		b.WriteString(t.Muted.Render("Select a run from the dashboard (1) or workflows (2)") + "\n")
		return b.String()
	}
	b.WriteString(t.Title.Render(trunc(r.run.Workflow+" "+r.run.ID, r.width)) + "\n")
	b.WriteString(fmt.Sprintf("Status: %s", components.StatusBadge(t, r.run.Status)) + "\n")
	if r.run.RunDir != "" {
		b.WriteString(t.Muted.Render("Dir: "+trunc(r.run.RunDir, r.width-6)) + "\n")
	}
	if !r.run.StartedAt.IsZero() {
		duration := time.Since(r.run.StartedAt).Round(time.Second)
		if !r.run.FinishedAt.IsZero() {
			duration = r.run.FinishedAt.Sub(r.run.StartedAt).Round(time.Second)
		}
		b.WriteString(t.Muted.Render(fmt.Sprintf("Started: %s • Duration: %s", r.run.StartedAt.Format(time.RFC3339), duration)) + "\n")
	}
	return b.String()
}

func (r *Runs) renderProgress(t *theme.Theme) string {
	if r.run.TotalSteps <= 0 {
		return t.Muted.Render("Progress: unknown")
	}
	completed := len(r.run.CompletedSteps)
	r.progressAnim.SetTarget(float64(completed) / float64(r.run.TotalSteps))
	r.progressAnim.Update()
	barWidth := r.width - 20
	if barWidth < 10 {
		barWidth = 10
	}
	animatedCompleted := int(math.Round(r.progressAnim.Value() * float64(r.run.TotalSteps)))
	bar := components.ProgressBar(t, barWidth, animatedCompleted, r.run.TotalSteps)
	var extra string
	if r.run.CurrentStep != "" {
		extra = " • Current: " + trunc(r.run.CurrentStep, r.width-30)
	}
	return t.Subtitle.Render("Progress") + "\n" + bar + extra
}

func (r *Runs) renderMid(t *theme.Theme, height int) string {
	if len(r.nodes) == 0 {
		return t.Muted.Render("No nodes")
	}
	listWidth := r.width
	detailWidth := 0
	if r.width > 80 {
		listWidth = min(35, r.width/3)
		detailWidth = r.width - listWidth - 1
	}

	list := r.renderNodeList(t, listWidth, height)
	if detailWidth <= 0 {
		return list
	}
	detail := r.renderDetail(t, detailWidth, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
}

func (r *Runs) renderNodeList(t *theme.Theme, width, height int) string {
	style := lipgloss.NewStyle().Width(width).Height(height)
	var b strings.Builder

	visible := height - 2
	start := 0
	if r.selectedNode >= visible {
		start = r.selectedNode - visible + 1
	}
	for i := start; i < len(r.nodes) && i < start+visible; i++ {
		n := r.nodes[i]
		prefix := "  "
		if i == r.selectedNode {
			prefix = t.Primary.Render("► ")
		}
		dur := time.Duration(n.Duration) * time.Millisecond
		line := prefix + components.StatusBadge(t, n.Status) + " " + trunc(n.NodeID, width-16)
		if n.Duration > 0 {
			line += " " + t.Muted.Render(dur.String())
		}
		if r.zoneManager != nil {
			line = r.zoneManager.Mark(fmt.Sprintf("%s%d", zoneRunNodePrefix, i), line)
		}
		b.WriteString(line + "\n")
	}
	return style.Render(b.String())
}

func (r *Runs) renderDetail(t *theme.Theme, width, height int) string {
	style := lipgloss.NewStyle().Width(width).Height(height).PaddingLeft(1)
	var b strings.Builder

	if r.selectedNode >= 0 && r.selectedNode < len(r.nodes) {
		n := r.nodes[r.selectedNode]
		b.WriteString(t.Subtitle.Render(trunc(n.NodeID, width)) + "\n")
		b.WriteString(fmt.Sprintf("Status: %s", components.StatusBadge(t, n.Status)) + "\n")
		if n.Duration > 0 {
			b.WriteString(t.Muted.Render(fmt.Sprintf("Duration: %s • Attempts: %d", time.Duration(n.Duration)*time.Millisecond, n.Attempts)) + "\n")
		}
		if n.ExitCode != nil {
			b.WriteString(t.Muted.Render(fmt.Sprintf("Exit code: %d", *n.ExitCode)) + "\n")
		}
		if n.Error != "" {
			b.WriteString(t.Danger.Render("Error: "+trunc(n.Error, width-8)) + "\n")
		}
		if n.Stdout != "" {
			b.WriteString(t.Muted.Render("Stdout:") + "\n" + trunc(n.Stdout, width-2) + "\n")
		}
		if n.Stderr != "" {
			b.WriteString(t.Danger.Render("Stderr:") + "\n" + trunc(n.Stderr, width-2) + "\n")
		}
	} else {
		b.WriteString(t.Subtitle.Render("Timeline") + "\n")
		b.WriteString(components.Timeline(t, width, r.events, height-2) + "\n")
		if len(r.nodes) > 0 && width > 30 && height > 10 {
			labels := make([]string, 0, len(r.nodes))
			values := make([]float64, 0, len(r.nodes))
			for _, n := range r.nodes {
				labels = append(labels, n.NodeID)
				values = append(values, float64(n.Duration))
			}
			b.WriteString(components.BarChart(t, width, 6, labels, values, "Duration (ms)"))
		}
	}
	return style.Render(b.String())
}

func (r *Runs) renderHints(t *theme.Theme) string {
	if r.confirming != "" {
		return t.Warning.Render(fmt.Sprintf("Confirm %s: y/n", r.confirming))
	}
	var parts []string
	parts = append(parts, "j/k nodes • enter select")
	if canCancelRun(r.run.Status) {
		parts = append(parts, "c cancel")
	}
	if canPauseRun(r.run.Status) {
		parts = append(parts, "p pause")
	}
	if canResumeRun(r.run.Status) {
		parts = append(parts, "r resume")
	}
	return t.Muted.Render(strings.Join(parts, " • "))
}

func canCancelRun(status string) bool {
	s := strings.ToLower(status)
	return s == "running" || s == "created" || s == "paused"
}

func canPauseRun(status string) bool {
	s := strings.ToLower(status)
	return s == "running" || s == "created"
}

func canResumeRun(status string) bool {
	s := strings.ToLower(status)
	return s == "paused"
}
