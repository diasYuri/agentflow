package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// SettingField represents an editable setting field.
type SettingField struct {
	Label   string
	Key     string
	Value   string
	Bool    bool
	IsBool  bool
	Options []string
}

// Settings is the settings screen view.
type Settings struct {
	width   int
	height  int
	fields  []SettingField
	cursor  int
	editing bool
	editBuf string
	changed bool
}

// NewSettings creates a new Settings view.
func NewSettings() *Settings {
	return &Settings{}
}

// SetFields updates the displayed fields.
func (s *Settings) SetFields(fields []SettingField) {
	s.fields = fields
	if s.cursor >= len(s.fields) {
		s.cursor = max(0, len(s.fields)-1)
	}
}

// Update handles messages.
func (s *Settings) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.height = m.Height
	case tea.KeyMsg:
		if s.editing {
			switch m.Type {
			case tea.KeyEsc:
				s.editing = false
				s.editBuf = ""
			case tea.KeyEnter:
				s.applyEdit()
			case tea.KeyBackspace:
				if len(s.editBuf) > 0 {
					s.editBuf = s.editBuf[:len(s.editBuf)-1]
				}
			case tea.KeyRunes:
				s.editBuf += string(m.Runes)
			}
			return
		}

		switch m.String() {
		case "j", "down":
			if s.cursor < len(s.fields)-1 {
				s.cursor++
			}
		case "k", "up":
			if s.cursor > 0 {
				s.cursor--
			}
		case "enter":
			if s.cursor >= 0 && s.cursor < len(s.fields) {
				f := &s.fields[s.cursor]
				if f.IsBool {
					f.Bool = !f.Bool
					s.changed = true
				} else if len(f.Options) > 0 {
					s.cycleOption(f)
					s.changed = true
				} else {
					s.editing = true
					s.editBuf = f.Value
				}
			}
		case "s":
			s.changed = true
		}
	}
}

// View implements tea.Model.
func (s *Settings) View(t *theme.Theme) string {
	if s.width == 0 || s.height == 0 {
		return "Loading..."
	}
	style := lipgloss.NewStyle().Width(s.width).Height(s.height)
	var b strings.Builder
	b.WriteString(t.Title.Render("Settings") + "\n\n")

	visible := s.height - 5
	start := 0
	if s.cursor >= visible {
		start = s.cursor - visible + 1
	}
	for i := start; i < len(s.fields) && i < start+visible; i++ {
		f := s.fields[i]
		prefix := "  "
		if i == s.cursor {
			prefix = t.Primary.Render("► ")
		}
		var value string
		if f.IsBool {
			if f.Bool {
				value = t.Success.Render("on")
			} else {
				value = t.Muted.Render("off")
			}
		} else {
			value = t.Body.Render(trunc(f.Value, s.width-30))
		}
		if s.editing && i == s.cursor {
			value = t.Primary.Render(trunc(s.editBuf, s.width-30) + "_")
		}
		line := fmt.Sprintf("%s%-20s %s", prefix, f.Label+":", value)
		b.WriteString(line + "\n")
	}

	if s.changed {
		b.WriteString("\n" + t.Warning.Render("Unsaved changes — press s to save") + "\n")
	} else {
		b.WriteString("\n" + t.Muted.Render("j/k navigate • enter edit/toggle • s save") + "\n")
	}
	return style.Render(b.String())
}

// SetSize sets the view size.
func (s *Settings) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// Changed returns whether settings have been modified.
func (s *Settings) Changed() bool {
	return s.changed
}

// SetChanged sets the changed flag.
func (s *Settings) SetChanged(v bool) {
	s.changed = v
}

// Fields returns the current fields.
func (s *Settings) Fields() []SettingField {
	return s.fields
}

func (s *Settings) applyEdit() {
	if s.cursor >= 0 && s.cursor < len(s.fields) {
		s.fields[s.cursor].Value = s.editBuf
		s.changed = true
	}
	s.editing = false
	s.editBuf = ""
}

func (s *Settings) cycleOption(f *SettingField) {
	if len(f.Options) == 0 {
		return
	}
	idx := 0
	for i, o := range f.Options {
		if o == f.Value {
			idx = i
			break
		}
	}
	idx++
	if idx >= len(f.Options) {
		idx = 0
	}
	f.Value = f.Options[idx]
}
