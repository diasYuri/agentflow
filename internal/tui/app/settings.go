package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diasYuri/agentflow/internal/tui/theme"
)

const settingsFileName = "tui-settings.json"

// TUISettings holds persisted TUI preferences.
type TUISettings struct {
	Theme         theme.Mode `json:"theme"`
	Mouse         bool       `json:"mouse"`
	ReducedMotion bool       `json:"reduced_motion"`
	CodexPath     string     `json:"codex_path,omitempty"`
	ClaudePath    string     `json:"claude_path,omitempty"`
	PiPath        string     `json:"pi_path,omitempty"`
	RunRoot       string     `json:"run_root,omitempty"`
}

// DefaultTUISettings returns default TUI settings.
func DefaultTUISettings() TUISettings {
	return TUISettings{
		Theme: theme.ModeAuto,
		Mouse: true,
	}
}

// settingsPath returns the path to the TUI settings file inside .agentflow.
func settingsPath() string {
	root := defaultAgentFlowRoot()
	return filepath.Join(root, settingsFileName)
}

// LoadTUISettings reads persisted TUI settings, falling back to defaults on error.
func LoadTUISettings() TUISettings {
	defaults := DefaultTUISettings()
	path := settingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var raw struct {
		Theme         *theme.Mode `json:"theme"`
		Mouse         *bool       `json:"mouse"`
		ReducedMotion *bool       `json:"reduced_motion"`
		CodexPath     string      `json:"codex_path,omitempty"`
		ClaudePath    string      `json:"claude_path,omitempty"`
		PiPath        string      `json:"pi_path,omitempty"`
		RunRoot       string      `json:"run_root,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return defaults
	}
	s := defaults
	if raw.Theme != nil {
		s.Theme = *raw.Theme
	}
	if raw.Mouse != nil {
		s.Mouse = *raw.Mouse
	}
	if raw.ReducedMotion != nil {
		s.ReducedMotion = *raw.ReducedMotion
	}
	s.CodexPath = raw.CodexPath
	s.ClaudePath = raw.ClaudePath
	s.PiPath = raw.PiPath
	s.RunRoot = raw.RunRoot
	if s.Theme == "" {
		s.Theme = defaults.Theme
	}
	return s
}

// ResolveStartupOptions merges persisted settings into CLI options while preserving explicit flags.
func ResolveStartupOptions(opts Options, settings TUISettings, noMouse bool) Options {
	if opts.Theme == "" {
		opts.Theme = settings.Theme
	}
	opts.ReducedMotion = settings.ReducedMotion
	if noMouse {
		opts.Mouse = false
	} else {
		opts.Mouse = settings.Mouse
	}
	return opts
}

// SaveTUISettings persists TUI settings to disk.
func SaveTUISettings(s TUISettings) error {
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

// defaultAgentFlowRoot is a variable so tests can override it.
var defaultAgentFlowRoot = func() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agentflow")
	}
	return filepath.Join(".", ".agentflow")
}
