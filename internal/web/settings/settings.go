// Package settings loads, merges, and exposes the configuration that powers
// the `agentflow web` server. The TOML file at ~/.agentflow/settings.toml
// is the source of truth for persistent state, while environment variables
// and CLI overrides apply in increasing precedence.
package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	SettingsFileName = "settings.toml"
	DefaultHost      = "127.0.0.1"
	DefaultPort      = 38080
	EnvPrefix        = "AGENTFLOW_WEB_"
	SlackEnvPrefix   = "AGENTFLOW_SLACK_"

	DaemonRequirementAuto     DaemonRequirement = "auto"
	DaemonRequirementRequired DaemonRequirement = "required"
	DaemonRequirementOff      DaemonRequirement = "off"
)

type DaemonRequirement string

func (d DaemonRequirement) IsValid() bool {
	switch d {
	case DaemonRequirementAuto, DaemonRequirementRequired, DaemonRequirementOff:
		return true
	}
	return false
}

type Server struct {
	Host        string            `toml:"host"`
	Port        int               `toml:"port"`
	OpenBrowser bool              `toml:"open_browser"`
	DevAssets   string            `toml:"dev_assets"`
	Daemon      DaemonRequirement `toml:"daemon"`
}

type Auth struct {
	TokenOverride string `toml:"token_override"`
}

type Paths struct {
	Root         string `toml:"root"`
	DaemonSocket string `toml:"daemon_socket"`
}

type ChatAgent struct {
	Provider     string                    `toml:"provider"`
	Model        string                    `toml:"model"`
	Timeout      string                    `toml:"timeout"`
	Sandbox      string                    `toml:"sandbox"`
	HistoryLimit int                       `toml:"history_limit"`
	Providers    map[string]ProviderConfig `toml:"-"`
}

type Slack struct {
	AppToken string `toml:"app_token"`
	BotToken string `toml:"bot_token"`
	Project  string `toml:"project"`
}

// ProviderConfig holds the connection and generation settings for one
// OpenAI-compatible backend.
type ProviderConfig struct {
	BaseURL     string            `toml:"base_url"`
	APIKey      string            `toml:"api_key"`
	APIKeyEnv   string            `toml:"api_key_env"`
	Headers     map[string]string `toml:"-"`
	Temperature float64           `toml:"temperature"`
	MaxTokens   int               `toml:"max_tokens"`
	TopP        float64           `toml:"top_p"`
}

type Settings struct {
	Server    Server    `toml:"web"`
	Auth      Auth      `toml:"web_auth"`
	Paths     Paths     `toml:"paths"`
	ChatAgent ChatAgent `toml:"chat_agent"`
	Slack     Slack     `toml:"slack"`
}

func Defaults() Settings {
	return Settings{
		Server: Server{
			Host:        DefaultHost,
			Port:        DefaultPort,
			OpenBrowser: true,
			Daemon:      DaemonRequirementAuto,
		},
		Paths: Paths{Root: defaultRoot()},
		ChatAgent: ChatAgent{
			Timeout:      "60s",
			Sandbox:      "read-only",
			HistoryLimit: 40,
			Providers:    map[string]ProviderConfig{},
		},
		Slack: Slack{},
	}
}

type Overrides struct {
	Host      *string
	Port      *int
	NoOpen    *bool
	DevAssets *string
	Daemon    *DaemonRequirement
	Root      *string
	Token     *string
}

func Load(root string, env Env, overrides Overrides) (Settings, error) {
	if env == nil {
		env = NewOSEnv()
	}
	resolvedRoot := chooseRoot(root, env, overrides)
	cfg := Defaults()
	cfg.Paths.Root = resolvedRoot

	if err := applyTOMLFile(&cfg, filepath.Join(resolvedRoot, SettingsFileName)); err != nil {
		return Settings{}, fmt.Errorf("read %s: %w", SettingsFileName, err)
	}
	applyEnv(&cfg, env)
	applyOverrides(&cfg, overrides)

	if err := validate(cfg); err != nil {
		return Settings{}, err
	}
	return cfg, nil
}

type Env interface {
	Lookup(name string) (string, bool)
}

func NewOSEnv() Env { return osEnv{} }

type osEnv struct{}

func (osEnv) Lookup(name string) (string, bool) { return os.LookupEnv(name) }

type MapEnv map[string]string

func (m MapEnv) Lookup(name string) (string, bool) {
	v, ok := m[name]
	return v, ok
}

func chooseRoot(root string, env Env, overrides Overrides) string {
	if overrides.Root != nil && strings.TrimSpace(*overrides.Root) != "" {
		return canonicalizePath(*overrides.Root)
	}
	if v, ok := env.Lookup(EnvPrefix + "ROOT"); ok && strings.TrimSpace(v) != "" {
		return canonicalizePath(v)
	}
	if root != "" {
		return canonicalizePath(root)
	}
	return defaultRoot()
}

func canonicalizePath(path string) string {
	expanded := expandHomeDir(path)
	if expanded == "" {
		return ""
	}
	if abs, err := filepath.Abs(expanded); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(expanded)
}

func expandHomeDir(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	default:
		return path
	}
}

func defaultRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agentflow")
	}
	return filepath.Join(".", ".agentflow")
}

func validate(cfg Settings) error {
	if cfg.Server.Host == "" {
		return fmt.Errorf("web.host must not be empty")
	}
	if cfg.Server.Port < 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("web.port %d out of range", cfg.Server.Port)
	}
	if !cfg.Server.Daemon.IsValid() {
		return fmt.Errorf("web.daemon %q must be one of auto, required, off", cfg.Server.Daemon)
	}
	return nil
}
