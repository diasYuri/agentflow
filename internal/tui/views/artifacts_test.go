package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/diasYuri/agentflow/internal/tui/client"
	"github.com/diasYuri/agentflow/internal/tui/theme"
	zone "github.com/lrstanley/bubblezone"
)

func TestNewArtifactsInitialState(t *testing.T) {
	a := NewArtifacts(nil)
	if a.width != 0 {
		t.Fatalf("expected width 0, got %d", a.width)
	}
	if a.cursor != -1 {
		t.Fatalf("expected cursor -1, got %d", a.cursor)
	}
}

func TestSetArtifacts(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetArtifacts([]client.ArtifactSummary{
		{ID: "a1", Name: "file1.txt"},
		{ID: "a2", Name: "file2.txt"},
	})
	if len(a.artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(a.artifacts))
	}
	if a.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", a.cursor)
	}
}

func TestSetArtifactsResetsCursor(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "file1.txt"}})
	a.cursor = 0
	a.SetArtifacts(nil)
	if a.cursor != -1 {
		t.Fatalf("expected cursor -1, got %d", a.cursor)
	}
}

func TestArtifactsCursorNavigation(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetArtifacts([]client.ArtifactSummary{
		{ID: "a1", Name: "file1.txt"},
		{ID: "a2", Name: "file2.txt"},
		{ID: "a3", Name: "file3.txt"},
	})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if a.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", a.cursor)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if a.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", a.cursor)
	}
}

func TestArtifactsSelectedArtifact(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "file1.txt"}})
	art, ok := a.SelectedArtifact()
	if !ok || art.ID != "a1" {
		t.Fatal("expected selected artifact")
	}
	a.cursor = -1
	_, ok = a.SelectedArtifact()
	if ok {
		t.Fatal("expected no selected artifact")
	}
}

func TestArtifactsViewRendersList(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetSize(80, 24)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "file1.txt", Size: 1024}})
	v := a.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "file1.txt") {
		t.Fatal("expected artifact name in view")
	}
	if !strings.Contains(v, "1.0 KB") {
		t.Fatal("expected formatted size")
	}
}

func TestArtifactsViewRendersMetadata(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetSize(120, 30)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "file1.txt", Path: "out/file1.txt", Size: 256, ContentType: "text/plain"}})
	a.SetPreview(client.ArtifactSummary{ID: "a1", Name: "file1.txt", Path: "out/file1.txt", Size: 256, ContentType: "text/plain", Content: "hello world"}, nil)
	v := a.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "out/file1.txt") {
		t.Fatal("expected path in view")
	}
	if !strings.Contains(v, "text/plain") {
		t.Fatal("expected content type in view")
	}
	if !strings.Contains(v, "hello world") {
		t.Fatal("expected preview content")
	}
}

func TestArtifactsViewShowsBinaryFallback(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetSize(120, 30)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "bin.dat"}})
	a.SetPreview(client.ArtifactSummary{ID: "a1", Name: "bin.dat", Content: "\xff\xfe"}, nil)
	v := a.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "Binary content") {
		t.Fatal("expected binary fallback")
	}
}

func TestArtifactsViewShowsBase64Decode(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetSize(120, 30)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "encoded.txt"}})
	a.SetPreview(client.ArtifactSummary{ID: "a1", Name: "encoded.txt", Encoding: "base64", Content: "aGVsbG8gd29ybGQ="}, nil)
	v := a.View(theme.Default(theme.ModeDark))
	if !strings.Contains(v, "hello world") {
		t.Fatal("expected decoded base64 content")
	}
}

func TestArtifactsSafePath(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"out/file.txt", "out/file.txt"},
		{"../../../etc/passwd", "passwd"},
		{"../secret", "secret"},
	}
	for _, c := range cases {
		got := safePath(c.input)
		if got != c.expected {
			t.Fatalf("safePath(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestArtifactsMouseDisabled(t *testing.T) {
	a := NewArtifacts(nil)
	a.SetArtifacts([]client.ArtifactSummary{{ID: "a1", Name: "file1.txt"}})
	prev := a.cursor
	a.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	if a.cursor != prev {
		t.Fatal("expected no cursor change when mouse disabled")
	}
}

func TestArtifactsMouseEnabledClicksRow(t *testing.T) {
	zm := zone.New()
	a := NewArtifacts(zm)
	a.SetSize(80, 24)
	a.SetArtifacts([]client.ArtifactSummary{
		{ID: "a1", Name: "file1.txt"},
		{ID: "a2", Name: "file2.txt"},
	})
	_ = a.View(theme.Default(theme.ModeDark))
	zm.Scan(strings.Repeat("\n", 30))
	a.Update(tea.MouseMsg{X: 5, Y: 5, Type: tea.MouseLeft, Action: tea.MouseActionRelease})
	// Should not panic.
}
