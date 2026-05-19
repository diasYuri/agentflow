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
