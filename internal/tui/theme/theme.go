package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Mode represents the theme mode.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeLight Mode = "light"
	ModeDark  Mode = "dark"
)

// Theme holds styled lipgloss definitions.
type Theme struct {
	Mode Mode

	Primary   lipgloss.Style
	Secondary lipgloss.Style
	Success   lipgloss.Style
	Danger    lipgloss.Style
	Warning   lipgloss.Style
	Muted     lipgloss.Style

	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Body     lipgloss.Style

	SidebarBg     lipgloss.Style
	SidebarItem   lipgloss.Style
	SidebarActive lipgloss.Style

	HeaderBg lipgloss.Style
	FooterBg lipgloss.Style

	Border lipgloss.Border
}

// Default returns a theme based on the mode and environment.
func Default(m Mode) *Theme {
	if m == ModeAuto {
		m = detectMode()
	}
	switch m {
	case ModeLight:
		return newLightTheme()
	default:
		return newDarkTheme()
	}
}

func detectMode() Mode {
	if strings.Contains(strings.ToLower(os.Getenv("COLORFGBG")), "15;0") {
		return ModeDark
	}
	if os.Getenv("TERM") == "xterm-256color" && os.Getenv("BACKGROUND") == "light" {
		return ModeLight
	}
	return ModeDark
}

func newDarkTheme() *Theme {
	return &Theme{
		Mode:          ModeDark,
		Primary:       lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")),
		Secondary:     lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0B0")),
		Success:       lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")),
		Danger:        lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4672")),
		Warning:       lipgloss.NewStyle().Foreground(lipgloss.Color("#F4D03F")),
		Muted:         lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")),
		Title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		Subtitle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0B0")),
		Body:          lipgloss.NewStyle().Foreground(lipgloss.Color("#E0E0E0")),
		SidebarBg:     lipgloss.NewStyle().Background(lipgloss.Color("#1A1A1A")),
		SidebarItem:   lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0B0")).PaddingLeft(1),
		SidebarActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#7D56F4")).PaddingLeft(1).Bold(true),
		HeaderBg:      lipgloss.NewStyle().Background(lipgloss.Color("#252525")).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1),
		FooterBg:      lipgloss.NewStyle().Background(lipgloss.Color("#252525")).Foreground(lipgloss.Color("#B0B0B0")).Padding(0, 1),
		Border:        lipgloss.RoundedBorder(),
	}
}

func newLightTheme() *Theme {
	return &Theme{
		Mode:          ModeLight,
		Primary:       lipgloss.NewStyle().Foreground(lipgloss.Color("#5B21B6")),
		Secondary:     lipgloss.NewStyle().Foreground(lipgloss.Color("#525252")),
		Success:       lipgloss.NewStyle().Foreground(lipgloss.Color("#047857")),
		Danger:        lipgloss.NewStyle().Foreground(lipgloss.Color("#BE123C")),
		Warning:       lipgloss.NewStyle().Foreground(lipgloss.Color("#B45309")),
		Muted:         lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3")),
		Title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#171717")),
		Subtitle:      lipgloss.NewStyle().Foreground(lipgloss.Color("#525252")),
		Body:          lipgloss.NewStyle().Foreground(lipgloss.Color("#404040")),
		SidebarBg:     lipgloss.NewStyle().Background(lipgloss.Color("#F5F5F5")),
		SidebarItem:   lipgloss.NewStyle().Foreground(lipgloss.Color("#525252")).PaddingLeft(1),
		SidebarActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#5B21B6")).PaddingLeft(1).Bold(true),
		HeaderBg:      lipgloss.NewStyle().Background(lipgloss.Color("#E5E5E5")).Foreground(lipgloss.Color("#171717")).Padding(0, 1),
		FooterBg:      lipgloss.NewStyle().Background(lipgloss.Color("#E5E5E5")).Foreground(lipgloss.Color("#525252")).Padding(0, 1),
		Border:        lipgloss.RoundedBorder(),
	}
}
