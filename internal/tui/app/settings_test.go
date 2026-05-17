package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestDefaultTUISettings(t *testing.T) {
	s := DefaultTUISettings()
	if s.Theme != theme.ModeAuto {
		t.Fatalf("expected default theme auto, got %s", s.Theme)
	}
	if !s.Mouse {
		t.Fatal("expected default mouse true")
	}
	if s.ReducedMotion {
		t.Fatal("expected default reduced motion false")
	}
}

func TestLoadTUISettingsFallback(t *testing.T) {
	s := LoadTUISettings()
	if s.Theme != theme.ModeAuto {
		t.Fatalf("expected fallback theme auto, got %s", s.Theme)
	}
}

func TestSaveAndLoadTUISettings(t *testing.T) {
	tmp := t.TempDir()
	origRoot := defaultAgentFlowRoot
	defaultAgentFlowRoot = func() string { return tmp }
	defer func() { defaultAgentFlowRoot = origRoot }()

	saved := TUISettings{Theme: theme.ModeDark, Mouse: false, ReducedMotion: true, CodexPath: "/usr/bin/codex"}
	if err := SaveTUISettings(saved); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	loaded := LoadTUISettings()
	if loaded.Theme != theme.ModeDark {
		t.Fatalf("expected theme dark, got %s", loaded.Theme)
	}
	if loaded.Mouse {
		t.Fatal("expected mouse false")
	}
	if !loaded.ReducedMotion {
		t.Fatal("expected reduced motion true")
	}
	if loaded.CodexPath != "/usr/bin/codex" {
		t.Fatalf("expected codex path, got %s", loaded.CodexPath)
	}
}

func TestLoadTUISettingsMergesDefaults(t *testing.T) {
	tmp := t.TempDir()
	origRoot := defaultAgentFlowRoot
	defaultAgentFlowRoot = func() string { return tmp }
	defer func() { defaultAgentFlowRoot = origRoot }()

	path := filepath.Join(tmp, settingsFileName)
	data := []byte(`{"mouse": false}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write partial settings: %v", err)
	}

	loaded := LoadTUISettings()
	if loaded.Theme != theme.ModeAuto {
		t.Fatalf("expected default theme merged, got %s", loaded.Theme)
	}
	if loaded.Mouse {
		t.Fatal("expected mouse false from file")
	}
	if loaded.ReducedMotion {
		t.Fatal("expected reduced motion default false when omitted")
	}
}

func TestResolveStartupOptionsUsesSettings(t *testing.T) {
	opts := Options{}
	resolved := ResolveStartupOptions(opts, TUISettings{
		Theme:         theme.ModeDark,
		Mouse:         false,
		ReducedMotion: true,
	}, false)
	if resolved.Theme != theme.ModeDark {
		t.Fatalf("expected theme from settings, got %s", resolved.Theme)
	}
	if resolved.Mouse {
		t.Fatal("expected mouse false from settings")
	}
	if !resolved.ReducedMotion {
		t.Fatal("expected reduced motion from settings")
	}
}

func TestResolveStartupOptionsHonorsNoMouse(t *testing.T) {
	resolved := ResolveStartupOptions(Options{Mouse: true}, TUISettings{Mouse: true}, true)
	if resolved.Mouse {
		t.Fatal("expected mouse disabled when no-mouse is set")
	}
}
