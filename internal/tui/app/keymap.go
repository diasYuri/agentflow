package app

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the global keybindings.
type KeyMap struct {
	Quit      key.Binding
	PrevRoute key.Binding
	NextRoute key.Binding
	Left      key.Binding
	Right     key.Binding
	Help      key.Binding
}

// DefaultKeyMap returns the default keymap.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		PrevRoute: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev route"),
		),
		NextRoute: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next route"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
	}
}
