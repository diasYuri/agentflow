package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreSafe(t *testing.T) {
	d := Defaults()
	if d.Server.Host != DefaultHost {
		t.Fatalf("default host=%s want %s", d.Server.Host, DefaultHost)
	}
	if d.Server.Port != DefaultPort {
		t.Fatalf("default port=%d want %d", d.Server.Port, DefaultPort)
	}
	if !d.Server.OpenBrowser {
		t.Fatalf("default open_browser=false")
	}
	if d.Server.Daemon != DaemonRequirementAuto {
		t.Fatalf("default daemon=%s want auto", d.Server.Daemon)
	}
}

func TestLoadWithoutFileReturnsDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := Load(tmp, MapEnv{}, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != DefaultHost {
		t.Fatalf("host=%s", cfg.Server.Host)
	}
	if cfg.Paths.Root != tmp {
		t.Fatalf("root=%s want %s", cfg.Paths.Root, tmp)
	}
}

func TestLoadAppliesTOMLValues(t *testing.T) {
	tmp := t.TempDir()
	body := []byte(
		"[web]\n" +
			"host = \"0.0.0.0\"\n" +
			"port = 12345\n" +
			"open_browser = false\n" +
			"dev_assets = \"/srv/agentflow-web\"\n" +
			"daemon = \"required\"\n" +
			"\n" +
			"[web.auth]\n" +
			"token_override = \"toml-token\"\n",
	)
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	cfg, err := Load(tmp, MapEnv{}, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("host=%s", cfg.Server.Host)
	}
	if cfg.Server.Port != 12345 {
		t.Fatalf("port=%d", cfg.Server.Port)
	}
	if cfg.Server.OpenBrowser {
		t.Fatalf("open_browser must be false")
	}
	if cfg.Server.DevAssets != "/srv/agentflow-web" {
		t.Fatalf("dev_assets=%s", cfg.Server.DevAssets)
	}
	if cfg.Server.Daemon != DaemonRequirementRequired {
		t.Fatalf("daemon=%s", cfg.Server.Daemon)
	}
	if cfg.Auth.TokenOverride != "toml-token" {
		t.Fatalf("token=%s", cfg.Auth.TokenOverride)
	}
}

func TestEnvOverridesTOMLValues(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("[web]\nport = 11111\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	env := MapEnv{
		EnvPrefix + "PORT":         "22222",
		EnvPrefix + "OPEN_BROWSER": "false",
		EnvPrefix + "DAEMON":       "off",
	}
	cfg, err := Load(tmp, env, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 22222 {
		t.Fatalf("port=%d", cfg.Server.Port)
	}
	if cfg.Server.OpenBrowser {
		t.Fatalf("open_browser must be false")
	}
	if cfg.Server.Daemon != DaemonRequirementOff {
		t.Fatalf("daemon=%s", cfg.Server.Daemon)
	}
}

func TestOverridesWinOverEnv(t *testing.T) {
	tmp := t.TempDir()
	port := 33333
	noOpen := true
	daemon := DaemonRequirementRequired
	host := "127.0.0.5"
	env := MapEnv{
		EnvPrefix + "PORT":         "22222",
		EnvPrefix + "OPEN_BROWSER": "true",
		EnvPrefix + "DAEMON":       "off",
	}
	cfg, err := Load(tmp, env, Overrides{
		Host:   &host,
		Port:   &port,
		NoOpen: &noOpen,
		Daemon: &daemon,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != host {
		t.Fatalf("host=%s", cfg.Server.Host)
	}
	if cfg.Server.Port != port {
		t.Fatalf("port=%d", cfg.Server.Port)
	}
	if cfg.Server.OpenBrowser {
		t.Fatalf("expected open_browser=false")
	}
	if cfg.Server.Daemon != daemon {
		t.Fatalf("daemon=%s", cfg.Server.Daemon)
	}
}

func TestEnvRootChoosesPath(t *testing.T) {
	tmp := t.TempDir()
	env := MapEnv{EnvPrefix + "ROOT": tmp}
	cfg, err := Load("", env, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Paths.Root != tmp {
		t.Fatalf("root=%s want %s", cfg.Paths.Root, tmp)
	}
}

func TestOverrideRootBeatsEnv(t *testing.T) {
	tmpA := t.TempDir()
	tmpB := t.TempDir()
	env := MapEnv{EnvPrefix + "ROOT": tmpA}
	cfg, err := Load("", env, Overrides{Root: &tmpB})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Paths.Root != tmpB {
		t.Fatalf("root=%s want %s", cfg.Paths.Root, tmpB)
	}
}

func TestRootPathExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := "~/.agentflow"
	cfg, err := Load("", MapEnv{}, Overrides{Root: &root})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := filepath.Join(home, ".agentflow")
	if cfg.Paths.Root != want {
		t.Fatalf("root=%s want %s", cfg.Paths.Root, want)
	}
}

func TestInvalidDaemonValueFails(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("[web]\ndaemon = \"nope\"\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	if _, err := Load(tmp, MapEnv{}, Overrides{}); err == nil {
		t.Fatalf("expected error for invalid daemon value")
	}
}

func TestInvalidPortFails(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("[web]\nport = 70000\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	if _, err := Load(tmp, MapEnv{}, Overrides{}); err == nil {
		t.Fatalf("expected port range error")
	}
}

func TestMalformedTOMLReturnsError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), []byte("not = toml = at all\n"), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	if _, err := Load(tmp, MapEnv{}, Overrides{}); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestCommentsAndBlankLinesAreIgnored(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("# leading comment\n\n[web] # inline comment\nport = 4444 # trailing\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	cfg, err := Load(tmp, MapEnv{}, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 4444 {
		t.Fatalf("port=%d", cfg.Server.Port)
	}
}

func TestLoadParsesChatAgentSettings(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("[chat_agent]\nprovider = \"openai\"\nmodel = \"gpt-4\"\ntimeout = \"30s\"\nsandbox = \"read-only\"\nhistory_limit = 20\n\n[chat_agent.providers.openai]\nbase_url = \"https://api.openai.com\"\napi_key_env = \"OPENAI_API_KEY\"\ntemperature = 0.2\nmax_tokens = 1000\ntop_p = 0.9\nX-Custom = \"header-value\"\n\n[chat_agent.providers.ollama]\nbase_url = \"http://localhost:11434\"\napi_key = \"ollama-key\"\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	cfg, err := Load(tmp, MapEnv{}, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ChatAgent.Provider != "openai" {
		t.Fatalf("provider=%s", cfg.ChatAgent.Provider)
	}
	if cfg.ChatAgent.Model != "gpt-4" {
		t.Fatalf("model=%s", cfg.ChatAgent.Model)
	}
	if cfg.ChatAgent.Timeout != "30s" {
		t.Fatalf("timeout=%s", cfg.ChatAgent.Timeout)
	}
	if cfg.ChatAgent.Sandbox != "read-only" {
		t.Fatalf("sandbox=%s", cfg.ChatAgent.Sandbox)
	}
	if cfg.ChatAgent.HistoryLimit != 20 {
		t.Fatalf("history_limit=%d", cfg.ChatAgent.HistoryLimit)
	}
	openai, ok := cfg.ChatAgent.Providers["openai"]
	if !ok {
		t.Fatal("missing openai provider")
	}
	if openai.BaseURL != "https://api.openai.com" {
		t.Fatalf("base_url=%s", openai.BaseURL)
	}
	if openai.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("api_key_env=%s", openai.APIKeyEnv)
	}
	if openai.Temperature != 0.2 {
		t.Fatalf("temperature=%f", openai.Temperature)
	}
	if openai.MaxTokens != 1000 {
		t.Fatalf("max_tokens=%d", openai.MaxTokens)
	}
	if openai.TopP != 0.9 {
		t.Fatalf("top_p=%f", openai.TopP)
	}
	if openai.Headers["X-Custom"] != "header-value" {
		t.Fatalf("header=%v", openai.Headers)
	}
	ollama, ok := cfg.ChatAgent.Providers["ollama"]
	if !ok {
		t.Fatal("missing ollama provider")
	}
	if ollama.APIKey != "ollama-key" {
		t.Fatalf("ollama api_key=%s", ollama.APIKey)
	}
}

func TestLoadChatAgentEnvOverridesTOML(t *testing.T) {
	tmp := t.TempDir()
	body := []byte("[chat_agent]\nprovider = \"openai\"\nmodel = \"gpt-4\"\n")
	if err := os.WriteFile(filepath.Join(tmp, SettingsFileName), body, 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	env := MapEnv{
		EnvPrefix + "CHAT_AGENT_PROVIDER": "anthropic",
		EnvPrefix + "CHAT_AGENT_MODEL":    "claude-3",
	}
	cfg, err := Load(tmp, env, Overrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ChatAgent.Provider != "anthropic" {
		t.Fatalf("provider=%s", cfg.ChatAgent.Provider)
	}
	if cfg.ChatAgent.Model != "claude-3" {
		t.Fatalf("model=%s", cfg.ChatAgent.Model)
	}
}

func TestDefaultsIncludeChatAgentDefaults(t *testing.T) {
	d := Defaults()
	if d.ChatAgent.Timeout != "60s" {
		t.Fatalf("timeout=%s", d.ChatAgent.Timeout)
	}
	if d.ChatAgent.Sandbox != "read-only" {
		t.Fatalf("sandbox=%s", d.ChatAgent.Sandbox)
	}
	if d.ChatAgent.HistoryLimit != 40 {
		t.Fatalf("history_limit=%d", d.ChatAgent.HistoryLimit)
	}
	if d.ChatAgent.Providers == nil {
		t.Fatal("providers map is nil")
	}
}

