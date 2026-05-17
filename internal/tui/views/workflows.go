package views

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

type workflowTab int

const (
	tabOverview workflowTab = iota
	tabGraph
	tabDryRun
)

func (t workflowTab) String() string {
	switch t {
	case tabOverview:
		return "Overview"
	case tabGraph:
		return "Graph"
	case tabDryRun:
		return "Dry-run"
	}
	return ""
}

func (t workflowTab) zoneID() string {
	switch t {
	case tabOverview:
		return "wf_tab_overview"
	case tabGraph:
		return "wf_tab_graph"
	case tabDryRun:
		return "wf_tab_dryrun"
	}
	return ""
}

const zoneWfRowPrefix = "wf_row_"

// Workflows is the workflows screen view.
type Workflows struct {
	width, height int

	workflows []client.LocalWorkflow
	filtered  []client.LocalWorkflow
	cursor    int
	filter    string
	filtering bool

	selected  client.LocalWorkflow
	activeTab workflowTab

	loading bool
	listErr error

	validation map[string]error
	graphs     map[string]string
	dryRuns    map[string]string
	dryRunData map[string]dryRunResult

	graphVP  viewport.Model
	dryRunVP viewport.Model

	zoneManager *zone.Manager
}

type dryRunResult struct {
	Workflow string
	Inputs   map[string]any
	Order    []string
	Nodes    map[string]dryRunNode
}

type dryRunNode struct {
	ID        string
	Kind      string
	DependsOn []string
}

// NewWorkflows creates a new Workflows view.
func NewWorkflows(zm *zone.Manager) *Workflows {
	return &Workflows{
		validation:  make(map[string]error),
		graphs:      make(map[string]string),
		dryRuns:     make(map[string]string),
		zoneManager: zm,
	}
}

// SetSize sets the view size.
func (w *Workflows) SetSize(width, height int) {
	w.width = width
	w.height = height
	w.syncVPSizes()
}

func (w *Workflows) syncVPSizes() {
	dw := w.detailWidth()
	if dw > 0 {
		w.graphVP.Width = dw - 2
		w.graphVP.Height = w.height - 6
		w.dryRunVP.Width = dw - 2
		w.dryRunVP.Height = w.height - 6
	} else {
		w.graphVP.Width = w.width - 2
		w.graphVP.Height = w.height - 6
		w.dryRunVP.Width = w.width - 2
		w.dryRunVP.Height = w.height - 6
	}
}

func (w *Workflows) listWidth() int {
	if w.selected.Path != "" && w.width > 60 {
		return min(30, w.width/3)
	}
	return w.width
}

func (w *Workflows) detailWidth() int {
	if w.selected.Path != "" && w.width > 60 {
		return w.width - w.listWidth() - 1
	}
	return 0
}

// Update handles messages.
func (w *Workflows) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = m.Width
		w.height = m.Height
		w.syncVPSizes()

	case tea.KeyMsg:
		if w.filtering {
			switch m.Type {
			case tea.KeyEsc, tea.KeyEnter:
				w.filtering = false
			case tea.KeyBackspace:
				if len(w.filter) > 0 {
					w.filter = w.filter[:len(w.filter)-1]
					w.applyFilter()
				}
			case tea.KeyRunes:
				w.filter += string(m.Runes)
				w.applyFilter()
			}
			return
		}

		switch m.String() {
		case "/":
			w.filtering = true
			w.filter = ""
			w.applyFilter()
		case "j":
			w.moveCursor(1)
		case "k":
			w.moveCursor(-1)
		case "down":
			if w.selected.Path != "" && w.isScrollableTab() {
				w.graphVP, _ = w.graphVP.Update(m)
				w.dryRunVP, _ = w.dryRunVP.Update(m)
			} else {
				w.moveCursor(1)
			}
		case "up":
			if w.selected.Path != "" && w.isScrollableTab() {
				w.graphVP, _ = w.graphVP.Update(m)
				w.dryRunVP, _ = w.dryRunVP.Update(m)
			} else {
				w.moveCursor(-1)
			}
		case "enter":
			if len(w.filtered) > 0 && w.cursor >= 0 && w.cursor < len(w.filtered) {
				w.selected = w.filtered[w.cursor]
				w.activeTab = tabOverview
				w.syncVPSizes()
			}
		case "esc", "b":
			w.selected = client.LocalWorkflow{}
			w.syncVPSizes()
		case "right", "l", "tab":
			if w.selected.Path != "" {
				w.nextTab()
			}
		case "left", "h", "shift+tab":
			if w.selected.Path != "" {
				w.prevTab()
			}
		}

	case tea.MouseMsg:
		if w.zoneManager == nil || m.Action != tea.MouseActionRelease || m.Button != tea.MouseButtonLeft {
			return
		}
		for i := range w.filtered {
			zi := w.zoneManager.Get(fmt.Sprintf("%s%d", zoneWfRowPrefix, i))
			if zi != nil && zi.InBounds(m) {
				w.cursor = i
				w.selected = w.filtered[i]
				w.activeTab = tabOverview
				w.syncVPSizes()
				return
			}
		}
		if w.selected.Path != "" {
			for _, tab := range []workflowTab{tabOverview, tabGraph, tabDryRun} {
				zi := w.zoneManager.Get(tab.zoneID())
				if zi != nil && zi.InBounds(m) {
					w.activeTab = tab
					return
				}
			}
		}
	}
}

func (w *Workflows) isScrollableTab() bool {
	return w.activeTab == tabGraph || w.activeTab == tabDryRun
}

func (w *Workflows) applyFilter() {
	w.filtered = w.filtered[:0]
	term := strings.ToLower(w.filter)
	for _, wf := range w.workflows {
		if term == "" || strings.Contains(strings.ToLower(wf.Name), term) || strings.Contains(strings.ToLower(wf.Path), term) {
			w.filtered = append(w.filtered, wf)
		}
	}
	w.cursor = max(0, min(w.cursor, len(w.filtered)-1))
}

func (w *Workflows) moveCursor(delta int) {
	if len(w.filtered) == 0 {
		return
	}
	w.cursor += delta
	if w.cursor < 0 {
		w.cursor = 0
	}
	if w.cursor >= len(w.filtered) {
		w.cursor = len(w.filtered) - 1
	}
}

func (w *Workflows) nextTab() {
	w.activeTab++
	if w.activeTab > tabDryRun {
		w.activeTab = tabOverview
	}
}

func (w *Workflows) prevTab() {
	w.activeTab--
	if w.activeTab < tabOverview {
		w.activeTab = tabDryRun
	}
}

// SetLoading sets the loading state for the workflow list.
func (w *Workflows) SetLoading(v bool) {
	w.loading = v
}

// SetListError sets the error state for the workflow list.
func (w *Workflows) SetListError(err error) {
	w.listErr = err
}

// SetWorkflows updates the displayed workflows.
func (w *Workflows) SetWorkflows(workflows []client.LocalWorkflow) {
	w.workflows = workflows
	w.loading = false
	w.listErr = nil
	w.applyFilter()
}

// SetValidationResult stores a validation result for a workflow.
func (w *Workflows) SetValidationResult(ref string, err error) {
	if w.validation == nil {
		w.validation = make(map[string]error)
	}
	w.validation[ref] = err
}

// SetGraphResult stores graph output for a workflow.
func (w *Workflows) SetGraphResult(ref, output string) {
	if w.graphs == nil {
		w.graphs = make(map[string]string)
	}
	w.graphs[ref] = output
	w.graphVP.SetContent(output)
	w.graphVP.GotoTop()
}

// SetDryRunResult stores dry-run output for a workflow.
func (w *Workflows) SetDryRunResult(ref, output string) {
	if w.dryRuns == nil {
		w.dryRuns = make(map[string]string)
	}
	if w.dryRunData == nil {
		w.dryRunData = make(map[string]dryRunResult)
	}
	w.dryRuns[ref] = output
	var raw struct {
		Workflow string         `json:"workflow"`
		Inputs   map[string]any `json:"inputs"`
		Order    []string       `json:"order"`
		Nodes    map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(output), &raw); err == nil {
		data := dryRunResult{Workflow: raw.Workflow, Inputs: raw.Inputs, Order: raw.Order, Nodes: make(map[string]dryRunNode)}
		for id, n := range raw.Nodes {
			if nodeMap, ok := n.(map[string]any); ok {
				var drn dryRunNode
				drn.ID = id
				if spec, ok := nodeMap["spec"].(map[string]any); ok {
					if k, ok := spec["kind"].(string); ok {
						drn.Kind = k
					}
				}
				if deps, ok := nodeMap["dependencies"].([]any); ok {
					for _, d := range deps {
						if s, ok := d.(string); ok {
							drn.DependsOn = append(drn.DependsOn, s)
						}
					}
				}
				data.Nodes[id] = drn
			}
		}
		w.dryRunData[ref] = data
		w.dryRunVP.SetContent(w.formatDryRun(data, w.dryRunVP.Width))
	} else {
		w.dryRunVP.SetContent(output)
	}
	w.dryRunVP.GotoTop()
}

func (w *Workflows) formatDryRun(data dryRunResult, width int) string {
	var b strings.Builder
	b.WriteString("Workflow: " + data.Workflow + "\n\n")
	if len(data.Inputs) > 0 {
		b.WriteString("Resolved Inputs:\n")
		for k, v := range data.Inputs {
			b.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
		b.WriteString("\n")
	}
	b.WriteString("Execution Order:\n")
	for i, id := range data.Order {
		node := data.Nodes[id]
		deps := ""
		if len(node.DependsOn) > 0 {
			deps = " (deps: " + strings.Join(node.DependsOn, ", ") + ")"
		}
		b.WriteString(fmt.Sprintf("  %d. %s [%s]%s\n", i+1, id, node.Kind, deps))
	}
	return b.String()
}

// SelectByIndex selects a workflow by its index in the filtered list.
func (w *Workflows) SelectByIndex(index int) {
	if index >= 0 && index < len(w.filtered) {
		w.cursor = index
		w.selected = w.filtered[index]
		w.activeTab = tabOverview
		w.syncVPSizes()
	}
}

// SelectedWorkflow returns the currently selected workflow.
func (w *Workflows) SelectedWorkflow() (client.LocalWorkflow, bool) {
	if w.selected.Path == "" {
		return client.LocalWorkflow{}, false
	}
	return w.selected, true
}

// ActiveTab returns the name of the active tab.
func (w *Workflows) ActiveTab() string {
	return w.activeTab.String()
}

// IsLoading returns whether the workflow list is loading.
func (w *Workflows) IsLoading() bool {
	return w.loading
}

// ListError returns the current list error, if any.
func (w *Workflows) ListError() error {
	return w.listErr
}

// HasValidationResult reports whether a validation result exists for the given ref.
func (w *Workflows) HasValidationResult(ref string) bool {
	_, ok := w.validation[ref]
	return ok
}

// GraphResult returns the stored graph result for a ref.
func (w *Workflows) GraphResult(ref string) string {
	return w.graphs[ref]
}

// DryRunResult returns the stored raw dry-run result for a ref.
func (w *Workflows) DryRunResult(ref string) string {
	return w.dryRuns[ref]
}

// View implements tea.Model.
func (w *Workflows) View(t *theme.Theme) string {
	if w.width == 0 || w.height == 0 {
		return "Loading..."
	}
	list := w.renderList(t)
	if w.selected.Path == "" || w.width <= 60 {
		return list
	}
	detail := w.renderDetail(t)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
}

func (w *Workflows) renderList(t *theme.Theme) string {
	width := w.listWidth()
	style := lipgloss.NewStyle().Width(width).Height(w.height)
	var b strings.Builder

	if w.loading {
		b.WriteString(t.Muted.Render("Loading workflows...") + "\n")
		return style.Render(b.String())
	}
	if w.listErr != nil {
		b.WriteString(t.Danger.Render("Error: "+trunc(w.listErr.Error(), width-2)) + "\n")
		b.WriteString(t.Muted.Render("Press r to retry") + "\n")
		return style.Render(b.String())
	}

	if w.filtering {
		b.WriteString(t.Primary.Render("filter: "+w.filter+"_") + "\n")
	} else if w.filter != "" {
		b.WriteString(t.Muted.Render("filter: "+w.filter) + " " + t.Primary.Render("[esc]") + "\n")
	} else {
		b.WriteString(t.Subtitle.Render("Workflows") + "\n")
	}

	visible := w.height - 3
	start := 0
	if w.cursor >= visible {
		start = w.cursor - visible + 1
	}
	for i := start; i < len(w.filtered) && i < start+visible; i++ {
		wf := w.filtered[i]
		prefix := "  "
		if i == w.cursor {
			prefix = t.Primary.Render("► ")
		}
		name := t.Body.Render(trunc(wf.Name, width-4))
		row := prefix + name
		if wf.Description != "" {
			row += "\n" + prefix + "  " + t.Muted.Render(trunc(wf.Description, width-6))
		}
		if w.zoneManager != nil {
			row = w.zoneManager.Mark(fmt.Sprintf("%s%d", zoneWfRowPrefix, i), row)
		}
		b.WriteString(row + "\n")
	}

	if len(w.filtered) == 0 {
		b.WriteString(t.Muted.Render("No workflows found") + "\n")
	}
	b.WriteString("\n" + t.Muted.Render("j/k navigate • enter select • / filter"))
	return style.Render(b.String())
}

func (w *Workflows) renderDetail(t *theme.Theme) string {
	width := w.detailWidth()
	if width <= 0 {
		return ""
	}
	style := lipgloss.NewStyle().Width(width).Height(w.height).PaddingLeft(1)
	var b strings.Builder

	b.WriteString(t.Title.Render(trunc(w.selected.Name, width)) + "\n")
	b.WriteString(t.Muted.Render(trunc(w.selected.Path, width)) + "\n\n")
	b.WriteString(w.renderTabs(t, width) + "\n\n")

	switch w.activeTab {
	case tabOverview:
		b.WriteString(w.renderOverview(t, width))
	case tabGraph:
		b.WriteString(w.renderGraph(t, width))
	case tabDryRun:
		b.WriteString(w.renderDryRun(t, width))
	}

	b.WriteString("\n" + t.Muted.Render("v validate • g graph • d dry-run • h/l tabs • b back"))
	return style.Render(b.String())
}

func (w *Workflows) renderTabs(t *theme.Theme, width int) string {
	var parts []string
	for _, tab := range []workflowTab{tabOverview, tabGraph, tabDryRun} {
		label := " " + tab.String() + " "
		if tab == w.activeTab {
			label = t.Primary.Render(label)
		} else {
			label = t.Muted.Render(label)
		}
		if w.zoneManager != nil {
			label = w.zoneManager.Mark(tab.zoneID(), label)
		}
		parts = append(parts, label)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (w *Workflows) renderOverview(t *theme.Theme, width int) string {
	var b strings.Builder
	if w.selected.Description != "" {
		b.WriteString(t.Body.Render(trunc(w.selected.Description, width)) + "\n\n")
	}
	b.WriteString(t.Muted.Render(fmt.Sprintf("Nodes: %d", w.selected.NodeCount)) + "\n")
	if len(w.selected.Inputs) > 0 {
		b.WriteString(t.Muted.Render(fmt.Sprintf("Inputs: %d", len(w.selected.Inputs))) + "\n")
		for name, in := range w.selected.Inputs {
			req := ""
			if in.Required {
				req = " required"
			}
			b.WriteString(t.Muted.Render(fmt.Sprintf("  • %s (%s)%s", name, in.Type, req)) + "\n")
		}
		b.WriteString("\n")
	}
	if err, ok := w.validation[w.selected.Path]; ok {
		if err != nil {
			b.WriteString(t.Danger.Render("✗ Invalid") + "\n")
			b.WriteString(t.Danger.Render(trunc(err.Error(), width)) + "\n")
		} else {
			b.WriteString(t.Success.Render("✓ Valid") + "\n")
		}
	} else {
		b.WriteString(t.Muted.Render("Press v to validate") + "\n")
	}
	return b.String()
}

func (w *Workflows) renderGraph(t *theme.Theme, width int) string {
	out, ok := w.graphs[w.selected.Path]
	if !ok || out == "" {
		w.graphVP.SetContent("")
		return t.Muted.Render("Press g to generate graph") + "\n"
	}
	w.graphVP.SetContent(out)
	w.graphVP.GotoTop()
	return w.graphVP.View()
}

func (w *Workflows) renderDryRun(t *theme.Theme, width int) string {
	out, ok := w.dryRuns[w.selected.Path]
	if !ok || out == "" {
		w.dryRunVP.SetContent("")
		return t.Muted.Render("Press d to run dry-run") + "\n"
	}
	w.dryRunVP.SetContent(w.dryRunContentFor(w.selected.Path))
	w.dryRunVP.GotoTop()
	return w.dryRunVP.View()
}

func (w *Workflows) dryRunContentFor(ref string) string {
	if data, ok := w.dryRunData[ref]; ok {
		return w.formatDryRun(data, w.dryRunVP.Width)
	}
	return w.dryRuns[ref]
}

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
