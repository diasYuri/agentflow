package settings

import (
	"strconv"
	"strings"
)

// applyEnv overlays environment variables prefixed with EnvPrefix onto
// cfg. Unknown variables are ignored so a hosting environment can set
// extra variables without breaking startup.
func applyEnv(cfg *Settings, env Env) {
	if v, ok := lookupNonEmpty(env, EnvPrefix+"HOST"); ok {
		cfg.Server.Host = v
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"PORT"); ok {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"OPEN_BROWSER"); ok {
		if b, err := parseBool(v); err == nil {
			cfg.Server.OpenBrowser = b
		}
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"DEV_ASSETS"); ok {
		cfg.Server.DevAssets = v
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"DAEMON"); ok {
		cfg.Server.Daemon = DaemonRequirement(v)
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"TOKEN"); ok {
		cfg.Auth.TokenOverride = v
	}
	if v, ok := lookupNonEmpty(env, EnvPrefix+"DAEMON_SOCKET"); ok {
		cfg.Paths.DaemonSocket = v
	}
}

// applyOverrides overlays the CLI flag values, which always win because
// the user typed them this run.
func applyOverrides(cfg *Settings, overrides Overrides) {
	if overrides.Host != nil && strings.TrimSpace(*overrides.Host) != "" {
		cfg.Server.Host = *overrides.Host
	}
	if overrides.Port != nil {
		cfg.Server.Port = *overrides.Port
	}
	if overrides.NoOpen != nil && *overrides.NoOpen {
		cfg.Server.OpenBrowser = false
	}
	if overrides.DevAssets != nil {
		cfg.Server.DevAssets = *overrides.DevAssets
	}
	if overrides.Daemon != nil {
		cfg.Server.Daemon = *overrides.Daemon
	}
	if overrides.Token != nil && strings.TrimSpace(*overrides.Token) != "" {
		cfg.Auth.TokenOverride = *overrides.Token
	}
}

func lookupNonEmpty(env Env, key string) (string, bool) {
	v, ok := env.Lookup(key)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}
