package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/web/api"
	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

// Command bundles every piece of the `agentflow web` runtime: the
// merged settings, the auth gate, the asset provider, and the daemon
// availability check. The CLI assembles a Command and calls Run.
type Command struct {
	Settings settings.Settings
	Logger   *slog.Logger
	Stdout   io.Writer
	OpenURL  func(url string) error
	Daemon   DaemonChecker
	// Projects, if set, replaces the default ProjectRegistry. Tests use
	// this to inject an in-memory registry.
	Projects session.ProjectResolver

	server *Server
	db     *persistence.DB
	api    *api.Service
	broker *events.Broker
}

// DaemonChecker reports whether the local agentflowd daemon is
// reachable. The default implementation calls daemon.Status; tests
// inject a fake to avoid spinning up real processes.
type DaemonChecker interface {
	Available(ctx context.Context) bool
}

// Run starts the web server and blocks until ctx is cancelled. It
// honours the daemon requirement, prepares the AgentFlow root
// directory, prints the local URL/token banner, and shuts the server
// down cleanly when interrupted.
func (c *Command) Run(ctx context.Context) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Daemon == nil {
		c.Daemon = NewDefaultDaemonChecker(c.Settings.Paths.DaemonSocket)
	}
	if err := os.MkdirAll(c.Settings.Paths.Root, 0o755); err != nil {
		return fmt.Errorf("ensure agentflow root: %w", err)
	}
	if err := c.checkDaemon(ctx); err != nil {
		return err
	}
	if err := c.openAPI(ctx); err != nil {
		return err
	}
	defer c.closeAPI()

	auth, err := NewAuth(c.Settings.Auth.TokenOverride)
	if err != nil {
		return err
	}
	server, err := NewServer(Options{
		Settings: c.Settings,
		Logger:   c.Logger,
		Auth:     auth,
		Assets:   c.assetProvider(),
		API:      c.api,
	})
	if err != nil {
		return err
	}
	c.server = server

	addr, err := server.Listen(ctx)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s/?token=%s", addr, auth.Token())
	c.banner(addr, auth.Token(), url)

	if c.Settings.Server.OpenBrowser && c.OpenURL != nil {
		if err := c.OpenURL(url); err != nil {
			c.Logger.Warn("failed to open browser", "error", err)
		}
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// Server exposes the underlying HTTP server, primarily so the CLI tests
// and integration tests can inspect the bound address.
func (c *Command) Server() *Server { return c.server }

// API exposes the optional API service so tests can verify routing.
func (c *Command) API() *api.Service { return c.api }

func (c *Command) checkDaemon(ctx context.Context) error {
	available := c.Daemon.Available(ctx)
	switch c.Settings.Server.Daemon {
	case settings.DaemonRequirementOff:
		return nil
	case settings.DaemonRequirementRequired:
		if !available {
			return errors.New("agentflowd is not running and daemon=required was requested")
		}
		return nil
	case settings.DaemonRequirementAuto:
		if !available {
			c.Logger.Warn("agentflowd is not running; web features that need it will be unavailable")
		}
		return nil
	}
	return fmt.Errorf("unknown daemon requirement %q", c.Settings.Server.Daemon)
}

func (c *Command) assetProvider() AssetProvider {
	if dir := strings.TrimSpace(c.Settings.Server.DevAssets); dir != "" {
		return NewDevAssets(dir)
	}
	return NewEmbeddedAssets()
}

func (c *Command) openAPI(ctx context.Context) error {
	db, err := persistence.Open(ctx, persistence.DefaultPath(c.Settings.Paths.Root))
	if err != nil {
		return fmt.Errorf("open web db: %w", err)
	}
	projects := c.Projects
	if projects == nil {
		projects = app.NewProjectRegistry(nil)
	}
	broker := events.NewBroker(64)
	svc, err := api.NewService(api.Options{DB: db, Projects: projects, Broker: broker})
	if err != nil {
		_ = db.Close()
		broker.Close()
		return err
	}
	c.db = db
	c.api = svc
	c.broker = broker
	return nil
}

func (c *Command) closeAPI() {
	if c.api != nil {
		c.api.Close()
	}
	if c.db != nil {
		_ = c.db.Close()
	}
}

func (c *Command) banner(addr, token, url string) {
	fmt.Fprintf(c.Stdout, "AgentFlow Web ready\n")
	fmt.Fprintf(c.Stdout, "  url:   %s\n", url)
	fmt.Fprintf(c.Stdout, "  addr:  %s\n", addr)
	fmt.Fprintf(c.Stdout, "  token: %s\n", token)
}

// NewDefaultDaemonChecker returns a checker that pings the unix socket
// for `agentflowd`. The optional socketPath overrides the default.
func NewDefaultDaemonChecker(socketPath string) DaemonChecker {
	if socketPath == "" {
		socketPath = daemon.DefaultConfig().SocketPath
	}
	return &socketChecker{socketPath: socketPath}
}

type socketChecker struct{ socketPath string }

func (s *socketChecker) Available(ctx context.Context) bool {
	if _, err := os.Stat(s.socketPath); err != nil {
		return false
	}
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	transport := &http.Transport{
		DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(c, "unix", s.socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://daemon/v1/daemon/status", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}
