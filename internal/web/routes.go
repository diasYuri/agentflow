package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/version"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

// registerRoutes wires the HTTP surface available to Plan 1. The route
// list intentionally stays small; future plans extend this surface
// rather than restructuring it.
func registerRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/settings", s.handleSettings)
	mux.HandleFunc("/assets/", s.handleAssets)
	mux.HandleFunc("/static/", s.handleStatic)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/", s.handleShell)
}

// HealthResponse is the public health payload. Future fields can be
// added without breaking the contract because the struct is encoded as
// JSON and clients should ignore unknown keys.
type HealthResponse struct {
	Status     string    `json:"status"`
	Version    string    `json:"version"`
	Commit     string    `json:"commit,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	DaemonMode string    `json:"daemon_mode"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:     "ok",
		Version:    version.Version,
		Commit:     version.Commit,
		StartedAt:  s.StartedAt(),
		DaemonMode: string(s.cfg.Server.Daemon),
	})
}

// SettingsResponse exposes the subset of the merged settings that is
// safe to ship to the browser. It deliberately omits the session token
// override: clients read the token from the page bootstrap instead.
type SettingsResponse struct {
	Web   SettingsWeb   `json:"web"`
	Paths SettingsPaths `json:"paths"`
}

type SettingsWeb struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	OpenBrowser bool   `json:"open_browser"`
	DevAssets   string `json:"dev_assets,omitempty"`
	Daemon      string `json:"daemon"`
}

type SettingsPaths struct {
	Root         string `json:"root"`
	DaemonSocket string `json:"daemon_socket,omitempty"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponseFor(s.cfg))
}

func settingsResponseFor(cfg settings.Settings) SettingsResponse {
	return SettingsResponse{
		Web: SettingsWeb{
			Host:        cfg.Server.Host,
			Port:        cfg.Server.Port,
			OpenBrowser: cfg.Server.OpenBrowser,
			DevAssets:   cfg.Server.DevAssets,
			Daemon:      string(cfg.Server.Daemon),
		},
		Paths: SettingsPaths{
			Root:         cfg.Paths.Root,
			DaemonSocket: cfg.Paths.DaemonSocket,
		},
	}
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	s.handleAssetPrefix(w, r, "/assets/")
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	s.handleAssetPrefix(w, r, "/static/")
}

func (s *Server) handleAssetPrefix(w http.ResponseWriter, r *http.Request, prefix string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	s.assets.ServeAsset(w, r, name)
}

func (s *Server) handleShell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.assets.ServeShell(w, r)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
