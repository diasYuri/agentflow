// Package web implements the local "agentflow web" server. It wraps a
// standard library HTTP server with local-only auth, configuration
// merging, and the route surface that Plan 1 and Plan 2 need (health,
// settings, HTML shell, embedded-asset placeholder, projects,
// sessions, messages, tool calls, approvals, diagnostics, SSE, and
// debug-bundle export).
//
// The package is intentionally thin: orchestration logic that spans
// command line, settings, and runtime lives in command.go so the
// server can be exercised under tests without spawning the binary.
package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/web/api"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

// Server is the running web service. It owns its listener, HTTP
// server, and the components routed at startup. All state is local
// to the struct so multiple servers may run side by side in tests.
type Server struct {
	cfg       settings.Settings
	logger    *slog.Logger
	auth      *Auth
	assets    AssetProvider
	apiSvc    *api.Service
	startedAt time.Time

	mu       sync.Mutex
	listener net.Listener
	http     *http.Server
	addr     string
}

// Options groups the dependencies needed to build a server.
type Options struct {
	Settings settings.Settings
	Logger   *slog.Logger
	Auth     *Auth
	Assets   AssetProvider
	API      *api.Service
}

// NewServer constructs a server but does not bind a socket yet.
func NewServer(opts Options) (*Server, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Auth == nil {
		return nil, errors.New("web: Auth is required")
	}
	if opts.Assets == nil {
		opts.Assets = NewEmbeddedAssets()
	}
	return &Server{
		cfg:    opts.Settings,
		logger: opts.Logger,
		auth:   opts.Auth,
		assets: opts.Assets,
		apiSvc: opts.API,
	}, nil
}

// Listen binds the configured host/port and starts the server in a
// background goroutine. The returned address always reflects the real
// port (important when port 0 is requested).
func (s *Server) Listen(ctx context.Context) (string, error) {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("listen %s: %w", addr, err)
	}
	s.mu.Lock()
	s.listener = listener
	s.addr = listener.Addr().String()
	s.startedAt = time.Now()
	s.http = &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Unlock()

	go func() {
		err := s.http.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("web server failed", "error", err)
		}
	}()
	return s.addr, nil
}

// Addr reports the resolved listen address (e.g. "127.0.0.1:38080").
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// StartedAt reports when Listen succeeded.
func (s *Server) StartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startedAt
}

// Shutdown closes the listener and waits for in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	server := s.http
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	registerRoutes(mux, s)
	if s.apiSvc != nil {
		s.apiSvc.Register(mux)
	}
	return s.auth.Middleware(mux)
}
