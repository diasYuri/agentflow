package app

import "github.com/diasYuri/agentflow/internal/tui/theme"

// Options holds configuration for the TUI application.
type Options struct {
	Workflow      string
	Run           string
	Daemon        bool
	Mouse         bool
	Theme         theme.Mode
	ReducedMotion bool
}
