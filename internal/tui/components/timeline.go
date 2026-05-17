package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// Timeline renders a textual timeline of workflow events.
func Timeline(t *theme.Theme, width int, events []client.EventLine, maxLines int) string {
	if len(events) == 0 {
		return t.Muted.Render("No events yet")
	}
	var b strings.Builder
	start := 0
	if len(events) > maxLines {
		start = len(events) - maxLines
	}
	for i := start; i < len(events); i++ {
		e := events[i]
		ts := e.Timestamp.Format(time.RFC3339)
		if len(ts) > 19 {
			ts = ts[:19]
		}
		line := fmt.Sprintf("%s [%s] %s", ts, e.Type, e.Message)
		if e.NodeID != "" {
			line += fmt.Sprintf(" (%s)", e.NodeID)
		}
		b.WriteString(trunc(line, width) + "\n")
	}
	return b.String()
}
