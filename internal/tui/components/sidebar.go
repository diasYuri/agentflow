package components

import (
	"strings"

	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

// SidebarItem represents a navigable item in the sidebar.
type SidebarItem struct {
	Label string
	Route string
	Icon  string
}

// Sidebar is the left navigation component.
type Sidebar struct {
	items       []SidebarItem
	activeRoute string
	width       int
	height      int
	zoneManager *zone.Manager
}

// NewSidebar creates a new Sidebar.
func NewSidebar(zoneManager *zone.Manager) *Sidebar {
	return &Sidebar{
		items: []SidebarItem{
			{Label: "Dashboard", Route: "dashboard", Icon: "📊"},
			{Label: "Workflows", Route: "workflows", Icon: "⚙️"},
			{Label: "Runs", Route: "runs", Icon: "▶️"},
			{Label: "Logs", Route: "logs", Icon: "📜"},
			{Label: "Artifacts", Route: "artifacts", Icon: "📦"},
			{Label: "Settings", Route: "settings", Icon: "🔧"},
		},
		zoneManager: zoneManager,
	}
}

// SetSize sets the sidebar dimensions.
func (s *Sidebar) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// SetActiveRoute updates the highlighted item.
func (s *Sidebar) SetActiveRoute(route string) {
	s.activeRoute = route
}

// ActiveRoute returns the currently active route.
func (s *Sidebar) ActiveRoute() string {
	return s.activeRoute
}

// View renders the sidebar.
func (s *Sidebar) View(t *theme.Theme) string {
	if s.width == 0 || s.height == 0 {
		return ""
	}
	var b strings.Builder
	for _, item := range s.items {
		label := item.Icon + " " + item.Label
		style := t.SidebarItem.Width(s.width - 2)
		if item.Route == s.activeRoute {
			style = t.SidebarActive.Width(s.width - 2)
		}
		if s.zoneManager != nil {
			zoneID := "sidebar_" + item.Route
			b.WriteString(s.zoneManager.Mark(zoneID, style.Render(label)))
		} else {
			b.WriteString(style.Render(label))
		}
		b.WriteString("\n")
	}
	bg := t.SidebarBg.Width(s.width).Height(s.height)
	return bg.Render(b.String())
}

// RouteAt returns the route associated with a zone ID.
func (s *Sidebar) RouteAt(zoneID string) string {
	for _, item := range s.items {
		if "sidebar_"+item.Route == zoneID {
			return item.Route
		}
	}
	return ""
}
