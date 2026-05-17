package views

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

const (
	zoneArtifactPrefix = "artifact_"
	maxPreviewSize     = 4096
	maxPreviewLines    = 40
)

// Artifacts is the artifacts screen view.
type Artifacts struct {
	width       int
	height      int
	artifacts   []client.ArtifactSummary
	cursor      int
	preview     client.ArtifactSummary
	previewErr  error
	zoneManager *zone.Manager
}

// NewArtifacts creates a new Artifacts view.
func NewArtifacts(zm *zone.Manager) *Artifacts {
	return &Artifacts{zoneManager: zm, cursor: -1}
}

// Update handles messages.
func (a *Artifacts) Update(msg tea.Msg) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
	case tea.KeyMsg:
		switch m.String() {
		case "j", "down":
			if a.cursor < len(a.artifacts)-1 {
				a.cursor++
			}
		case "k", "up":
			if a.cursor > 0 {
				a.cursor--
			}
		case "enter":
			if a.cursor >= 0 && a.cursor < len(a.artifacts) {
				// Selection triggers preview request handled by parent model.
			}
		case "esc":
			a.preview = client.ArtifactSummary{}
			a.previewErr = nil
		}
	case tea.MouseMsg:
		if a.zoneManager == nil || m.Action != tea.MouseActionRelease || m.Button != tea.MouseButtonLeft {
			return
		}
		for i := range a.artifacts {
			zi := a.zoneManager.Get(fmt.Sprintf("%s%d", zoneArtifactPrefix, i))
			if zi != nil && zi.InBounds(m) {
				a.cursor = i
				return
			}
		}
	}
}

// View implements tea.Model.
func (a *Artifacts) View(t *theme.Theme) string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}
	listWidth := a.width
	detailWidth := 0
	if a.width > 70 {
		listWidth = min(35, a.width/3)
		detailWidth = a.width - listWidth - 1
	}

	list := a.renderList(t, listWidth)
	if detailWidth <= 0 {
		style := lipgloss.NewStyle().Width(a.width).Height(a.height)
		return style.Render(list)
	}
	detail := a.renderDetail(t, detailWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
}

// SetSize sets the view size.
func (a *Artifacts) SetSize(w, h int) {
	a.width = w
	a.height = h
}

// SetArtifacts updates the displayed artifacts.
func (a *Artifacts) SetArtifacts(artifacts []client.ArtifactSummary) {
	a.artifacts = artifacts
	if len(a.artifacts) == 0 {
		a.cursor = -1
		return
	}
	if a.cursor >= len(a.artifacts) {
		a.cursor = len(a.artifacts) - 1
	}
	if a.cursor < 0 {
		a.cursor = 0
	}
}

// SetPreview updates the previewed artifact content.
func (a *Artifacts) SetPreview(artifact client.ArtifactSummary, err error) {
	a.preview = artifact
	a.previewErr = err
}

// SelectedArtifact returns the currently selected artifact.
func (a *Artifacts) SelectedArtifact() (client.ArtifactSummary, bool) {
	if a.cursor >= 0 && a.cursor < len(a.artifacts) {
		return a.artifacts[a.cursor], true
	}
	return client.ArtifactSummary{}, false
}

// Preview returns the currently previewed artifact and any error.
func (a *Artifacts) Preview() (client.ArtifactSummary, error) {
	return a.preview, a.previewErr
}

func (a *Artifacts) renderList(t *theme.Theme, width int) string {
	style := lipgloss.NewStyle().Width(width).Height(a.height)
	var b strings.Builder
	b.WriteString(t.Subtitle.Render("Artifacts") + "\n")
	if len(a.artifacts) == 0 {
		b.WriteString(t.Muted.Render("No artifacts") + "\n")
		return style.Render(b.String())
	}

	visible := a.height - 3
	start := 0
	if a.cursor >= visible {
		start = a.cursor - visible + 1
	}
	for i := start; i < len(a.artifacts) && i < start+visible; i++ {
		art := a.artifacts[i]
		prefix := "  "
		if i == a.cursor {
			prefix = t.Primary.Render("► ")
		}
		name := trunc(art.Name, width-4)
		line := prefix + name
		if art.Size > 0 {
			line += " " + t.Muted.Render(formatSize(art.Size))
		}
		if a.zoneManager != nil {
			line = a.zoneManager.Mark(fmt.Sprintf("%s%d", zoneArtifactPrefix, i), line)
		}
		b.WriteString(line + "\n")
	}
	return style.Render(b.String())
}

func (a *Artifacts) renderDetail(t *theme.Theme, width int) string {
	style := lipgloss.NewStyle().Width(width).Height(a.height).PaddingLeft(1)
	var b strings.Builder

	if a.previewErr != nil {
		b.WriteString(t.Danger.Render("Error: "+trunc(a.previewErr.Error(), width-2)) + "\n")
		return style.Render(b.String())
	}

	if a.preview.ID == "" {
		if a.cursor >= 0 && a.cursor < len(a.artifacts) {
			art := a.artifacts[a.cursor]
			b.WriteString(a.renderMetadata(t, width, art) + "\n")
			b.WriteString(t.Muted.Render("Press enter to load preview") + "\n")
		} else {
			b.WriteString(t.Muted.Render("Select an artifact") + "\n")
		}
		return style.Render(b.String())
	}

	b.WriteString(a.renderMetadata(t, width, a.preview) + "\n")
	b.WriteString(a.renderPreview(t, width) + "\n")
	return style.Render(b.String())
}

func (a *Artifacts) renderMetadata(t *theme.Theme, width int, art client.ArtifactSummary) string {
	var b strings.Builder
	b.WriteString(t.Title.Render(trunc(art.Name, width)) + "\n")
	if art.Path != "" {
		b.WriteString(t.Muted.Render("Path: "+safePath(art.Path)) + "\n")
	}
	if art.Size > 0 {
		b.WriteString(t.Muted.Render("Size: "+formatSize(art.Size)) + "\n")
	}
	if art.ContentType != "" {
		b.WriteString(t.Muted.Render("Type: "+art.ContentType) + "\n")
	}
	if !art.ModifiedAt.IsZero() {
		b.WriteString(t.Muted.Render("Modified: "+art.ModifiedAt.Format("2006-01-02 15:04:05")) + "\n")
	}
	return b.String()
}

func (a *Artifacts) renderPreview(t *theme.Theme, width int) string {
	content := a.preview.Content
	if a.preview.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err == nil {
			content = string(decoded)
		} else {
			return t.Warning.Render("Base64 decode error: "+trunc(err.Error(), width-2)) + "\n"
		}
	}

	if isBinary(content) {
		return t.Warning.Render("Binary content — preview unavailable") + "\n"
	}

	lines := strings.Split(content, "\n")
	if len(lines) > maxPreviewLines {
		lines = lines[:maxPreviewLines]
		lines = append(lines, "...")
	}
	var out []string
	for _, line := range lines {
		if len(line) > maxPreviewSize {
			line = line[:maxPreviewSize]
		}
		out = append(out, trunc(line, width-2))
	}
	return t.Body.Render(strings.Join(out, "\n"))
}

func safePath(p string) string {
	clean := filepath.Clean(p)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, "..") {
		return filepath.Base(clean)
	}
	return clean
}

func isBinary(s string) bool {
	if len(s) == 0 {
		return false
	}
	sample := s
	if len(sample) > 1024 {
		sample = sample[:1024]
	}
	return !utf8.ValidString(sample)
}

func formatSize(size int64) string {
	switch {
	case size >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	case size >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
