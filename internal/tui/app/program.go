package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// NewProgram creates a tea.Program from options.
func NewProgram(opts Options) *tea.Program {
	m := NewModel(opts)
	pOpts := []tea.ProgramOption{}
	if opts.Mouse {
		pOpts = append(pOpts, tea.WithMouseCellMotion())
	}
	return tea.NewProgram(m, pOpts...)
}
