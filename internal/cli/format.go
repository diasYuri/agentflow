package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type cliFormat struct {
	colored bool

	titleStyle   lipgloss.Style
	sectionStyle lipgloss.Style
	keyStyle     lipgloss.Style
	mutedStyle   lipgloss.Style
	successStyle lipgloss.Style
	warningStyle lipgloss.Style
	dangerStyle  lipgloss.Style
	accentStyle  lipgloss.Style
}

func newCLIFormat(colored bool) cliFormat {
	if !colored {
		return cliFormat{}
	}

	return cliFormat{
		colored:      true,
		titleStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		sectionStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")),
		keyStyle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B0B0B0")),
		mutedStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")),
		warningStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#F4D03F")),
		dangerStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4672")),
		accentStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")),
	}
}

func (f cliFormat) title(text string) string {
	return f.render(f.titleStyle, text)
}

func (f cliFormat) section(text string) string {
	return f.render(f.sectionStyle, text)
}

func (f cliFormat) muted(text string) string {
	return f.render(f.mutedStyle, text)
}

func (f cliFormat) key(label string) string {
	return f.render(f.keyStyle, label)
}

func (f cliFormat) accent(text string) string {
	return f.render(f.accentStyle, text)
}

func (f cliFormat) value(text string) string {
	return text
}

func (f cliFormat) status(text string) string {
	switch normalizeStatus(text) {
	case "success":
		return f.render(f.successStyle, text)
	case "failed", "cancelled", "canceled", "timeout":
		return f.render(f.dangerStyle, text)
	case "running":
		return f.render(f.accentStyle, text)
	case "paused", "wait_approval":
		return f.render(f.warningStyle, text)
	default:
		return text
	}
}

func (f cliFormat) labelValue(label, value string) string {
	return fmt.Sprintf("%s: %s", f.key(label), f.value(value))
}

func (f cliFormat) note(text string) string {
	return f.muted(text)
}

func (f cliFormat) render(style lipgloss.Style, text string) string {
	if !f.colored {
		return text
	}
	return style.Render(text)
}

func (f cliFormat) table(headers []string, rows [][]string, widths []int) string {
	if len(headers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(f.tableBorder("┌", "┬", "┐", widths))
	b.WriteString("\n")
	b.WriteString(f.tableRow(headers, widths, true))
	b.WriteString("\n")
	b.WriteString(f.tableBorder("├", "┼", "┤", widths))
	b.WriteString("\n")
	for i, row := range rows {
		b.WriteString(f.tableRow(row, widths, false))
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(f.tableBorder("└", "┴", "┘", widths))
	return b.String()
}

func (f cliFormat) tableBorder(left, middle, right string, widths []int) string {
	var parts []string
	for _, width := range widths {
		if width < 1 {
			width = 1
		}
		parts = append(parts, strings.Repeat("─", width+2))
	}
	return left + strings.Join(parts, middle) + right
}

func (f cliFormat) tableRow(cells []string, widths []int, header bool) string {
	rendered := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		if header {
			rendered[i] = " " + padRendered(f.title(truncateVisual(cell, widths[i])), widths[i]) + " "
			continue
		}
		value := truncateVisual(cell, widths[i])
		if i == 3 {
			value = f.status(value)
		}
		rendered[i] = " " + padRendered(value, widths[i]) + " "
	}
	return "│" + strings.Join(rendered, "│") + "│"
}

func truncateVisual(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	current := 0
	for _, r := range value {
		rw := lipgloss.Width(string(r))
		if current+rw > width-1 {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	b.WriteString("…")
	return b.String()
}

func padRendered(value string, width int) string {
	if width <= 0 {
		return value
	}
	pad := width - lipgloss.Width(value)
	if pad <= 0 {
		return value
	}
	return value + strings.Repeat(" ", pad)
}

func (f cliFormat) keyValueLines(pairs [][2]string) []string {
	lines := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		lines = append(lines, f.labelValue(pair[0], pair[1]))
	}
	return lines
}

func (f cliFormat) block(title string, lines []string) string {
	var b strings.Builder
	b.WriteString(f.section(title))
	if len(lines) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(lines, "\n"))
	}
	return b.String()
}
