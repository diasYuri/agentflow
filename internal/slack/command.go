package slack

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

	"github.com/diasYuri/agentflow/internal/agentchannel"
	"github.com/diasYuri/agentflow/internal/agentchannel/chatagent"
	"github.com/diasYuri/agentflow/internal/agentchannel/diagnostics"
	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/daemon"
	websettings "github.com/diasYuri/agentflow/internal/web/settings"
)

const (
	defaultChatAgentTimeout    = 60 * time.Second
	defaultChatAgentHistoryLen = 40
)

type DaemonChecker interface {
	Available(context.Context) bool
}

type Command struct {
	Settings    websettings.Settings
	AppToken    string
	BotToken    string
	ProjectName string
	Logger      *slog.Logger
	Stdout      io.Writer
	Daemon      DaemonChecker
	Projects    session.ProjectResolver

	db        *persistence.DB
	broker    *events.Broker
	sessions  *session.Sessions
	channel   *agentchannel.Service
	botUserID string
	teamID    string
}

func (c *Command) Run(ctx context.Context) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if strings.TrimSpace(c.AppToken) == "" {
		return errors.New("slack app token is required")
	}
	if strings.TrimSpace(c.BotToken) == "" {
		return errors.New("slack bot token is required")
	}
	if c.Projects == nil {
		c.Projects = app.NewProjectRegistry(nil)
	}
	if strings.TrimSpace(c.ProjectName) != "" {
		if _, err := c.Projects.Resolve(c.ProjectName); err != nil {
			return fmt.Errorf("resolve project %q: %w", c.ProjectName, err)
		}
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
	if err := c.openCore(ctx); err != nil {
		return err
	}
	defer c.closeCore()

	client := newClient(c.AppToken, c.BotToken)
	auth, err := client.AuthTest(ctx)
	if err != nil {
		return fmt.Errorf("slack auth test: %w", err)
	}
	c.teamID = strings.TrimSpace(auth.TeamID)
	c.botUserID = strings.TrimSpace(auth.UserID)

	fmt.Fprintln(c.Stdout, "AgentFlow Slack ready")
	fmt.Fprintf(c.Stdout, "  team: %s\n", c.teamID)
	if strings.TrimSpace(c.ProjectName) == "" {
		fmt.Fprintln(c.Stdout, "  project: select in conversation")
	} else {
		fmt.Fprintf(c.Stdout, "  project: %s\n", c.ProjectName)
	}
	fmt.Fprintf(c.Stdout, "  bot: %s\n", c.botUserID)

	responder := newResponder(c.sessions, c.broker, client, c.Logger)
	go responder.Run(ctx)

	runner := newSocketRunner(client, &processor{
		submitter:   c.channel,
		projectName: c.ProjectName,
		teamIDFn: func(payloadTeamID string) string {
			if trimmed := strings.TrimSpace(payloadTeamID); trimmed != "" {
				return trimmed
			}
			return c.teamID
		},
		botUserID: c.botUserID,
	}, c.Logger)
	return runner.Run(ctx)
}

func (c *Command) openCore(ctx context.Context) error {
	db, err := persistence.Open(ctx, persistence.DefaultPath(c.Settings.Paths.Root))
	if err != nil {
		return fmt.Errorf("open slack db: %w", err)
	}
	projects := c.Projects
	if projects == nil {
		projects = app.NewProjectRegistry(nil)
	}
	broker := events.NewBroker(64)
	agent, agentTimeout, fallbackReason := buildChatAgent(c.Settings.ChatAgent)
	if fallbackReason != "" {
		c.Logger.Warn("chat agent fallback enabled", "reason", fallbackReason)
	} else {
		c.Logger.Info(
			"chat agent configured",
			"provider", c.Settings.ChatAgent.Provider,
			"model", c.Settings.ChatAgent.Model,
			"timeout", agentTimeout.String(),
			"history_limit", c.Settings.ChatAgent.HistoryLimit,
		)
	}
	sessions, err := session.NewSessions(session.Options{DB: db, Projects: projects})
	if err != nil {
		_ = db.Close()
		broker.Close()
		return err
	}
	recorder, err := diagnostics.NewRecorder(diagnostics.Options{
		DB: db,
		Publish: func(diag persistence.Diagnostic) {
			broker.Publish(diag.SessionID, events.KindDiagnostic, diag, diag.CorrelationID)
		},
	})
	if err != nil {
		_ = db.Close()
		broker.Close()
		return err
	}
	channelSvc, err := agentchannel.NewService(agentchannel.Options{
		Sessions:              sessions,
		Projects:              projects,
		Diagnostics:           recorder,
		Events:                broker,
		WorkflowDefinitions:   daemon.NewClient(c.Settings.Paths.DaemonSocket),
		WorkflowRuns:          daemon.NewClient(c.Settings.Paths.DaemonSocket),
		ChatAgent:             agent,
		ChatAgentTimeout:      agentTimeout,
		ChatAgentHistoryLimit: c.settingsHistoryLimit(),
		Logger:                c.Logger,
	})
	if err != nil {
		_ = db.Close()
		broker.Close()
		return err
	}
	c.db = db
	c.broker = broker
	c.sessions = sessions
	c.channel = channelSvc
	return nil
}

func (c *Command) checkDaemon(ctx context.Context) error {
	available := c.Daemon.Available(ctx)
	switch c.Settings.Server.Daemon {
	case websettings.DaemonRequirementOff:
		return nil
	case websettings.DaemonRequirementRequired:
		if !available {
			return errors.New("agentflowd is not running and daemon=required was requested")
		}
		return nil
	case websettings.DaemonRequirementAuto:
		if !available {
			c.Logger.Warn("agentflowd is not running; slack features that need it will be unavailable")
		}
		return nil
	default:
		return fmt.Errorf("unknown daemon requirement %q", c.Settings.Server.Daemon)
	}
}

func (c *Command) settingsHistoryLimit() int {
	if c.Settings.ChatAgent.HistoryLimit > 0 {
		return c.Settings.ChatAgent.HistoryLimit
	}
	return defaultChatAgentHistoryLen
}

func buildChatAgent(cfg websettings.ChatAgent) (agentchannel.ChatAgent, time.Duration, string) {
	timeout := defaultChatAgentTimeout
	if strings.TrimSpace(cfg.Timeout) != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	if cfg.Provider == "" || cfg.Model == "" {
		reason := "chat agent is not configured"
		return chatagent.NewFallbackAgent(reason), timeout, reason
	}
	providers := map[string]chatagent.ProviderConfig{}
	for name, pc := range cfg.Providers {
		providers[name] = chatagent.ProviderConfig{
			BaseURL:     pc.BaseURL,
			APIKey:      pc.APIKey,
			APIKeyEnv:   pc.APIKeyEnv,
			Headers:     pc.Headers,
			Temperature: pc.Temperature,
			MaxTokens:   pc.MaxTokens,
			TopP:        pc.TopP,
		}
	}
	agent, err := chatagent.NewGenkitAgent(cfg.Provider, cfg.Model, providers, timeout)
	if err != nil {
		reason := err.Error()
		return chatagent.NewFallbackAgent(reason), timeout, reason
	}
	return agent, timeout, ""
}

func (c *Command) closeCore() {
	if c.broker != nil {
		c.broker.Close()
	}
	if c.db != nil {
		_ = c.db.Close()
	}
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
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://unix/status", nil)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}
