package app

// Route represents a TUI screen route.
type Route string

const (
	RouteDashboard Route = "dashboard"
	RouteWorkflows Route = "workflows"
	RouteRuns      Route = "runs"
	RouteLogs      Route = "logs"
	RouteArtifacts Route = "artifacts"
	RouteSettings  Route = "settings"
)

// Routes returns all available routes in order.
func Routes() []Route {
	return []Route{
		RouteDashboard,
		RouteWorkflows,
		RouteRuns,
		RouteLogs,
		RouteArtifacts,
		RouteSettings,
	}
}

// RouteIndex returns the index of a route in the ordered list.
func RouteIndex(r Route) int {
	for i, route := range Routes() {
		if route == r {
			return i
		}
	}
	return -1
}

// RouteFromIndex returns the route at the given index.
func RouteFromIndex(i int) Route {
	routes := Routes()
	if i < 0 {
		return routes[0]
	}
	if i >= len(routes) {
		return routes[len(routes)-1]
	}
	return routes[i]
}
