package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/web/chatagent"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

type fakeDaemonChecker struct{ available bool }

func (f fakeDaemonChecker) Available(context.Context) bool { return f.available }

func TestCheckDaemonRequiredFailsWhenMissing(t *testing.T) {
	cfg := settings.Defaults()
	cfg.Server.Daemon = settings.DaemonRequirementRequired
	c := &Command{
		Settings: cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Daemon:   fakeDaemonChecker{available: false},
	}
	if err := c.checkDaemon(context.Background()); err == nil {
		t.Fatalf("expected error when daemon required but missing")
	}
}

func TestCheckDaemonAutoPassesWithWarning(t *testing.T) {
	cfg := settings.Defaults()
	cfg.Server.Daemon = settings.DaemonRequirementAuto
	c := &Command{
		Settings: cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Daemon:   fakeDaemonChecker{available: false},
	}
	if err := c.checkDaemon(context.Background()); err != nil {
		t.Fatalf("auto must not fail when daemon missing: %v", err)
	}
}

func TestCheckDaemonOffSkipsLookup(t *testing.T) {
	cfg := settings.Defaults()
	cfg.Server.Daemon = settings.DaemonRequirementOff
	c := &Command{
		Settings: cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Daemon:   fakeDaemonChecker{available: false},
	}
	if err := c.checkDaemon(context.Background()); err != nil {
		t.Fatalf("off must skip checks: %v", err)
	}
}

func TestBannerWritesTokenAndURL(t *testing.T) {
	var buf bytes.Buffer
	c := &Command{Stdout: &buf}
	c.banner("127.0.0.1:38080", "tok123", "http://127.0.0.1:38080/?token=tok123")
	out := buf.String()
	for _, want := range []string{"AgentFlow Web ready", "127.0.0.1:38080", "tok123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q: %s", want, out)
		}
	}
}

func TestAssetProviderChoosesDevWhenSet(t *testing.T) {
	cfg := settings.Defaults()
	cfg.Server.DevAssets = "/tmp/agentflow-frontend"
	c := &Command{Settings: cfg}
	if _, ok := c.assetProvider().(*devAssets); !ok {
		t.Fatalf("expected devAssets when DevAssets is set")
	}
}

func TestAssetProviderFallsBackToEmbedded(t *testing.T) {
	cfg := settings.Defaults()
	c := &Command{Settings: cfg}
	if _, ok := c.assetProvider().(*embeddedAssets); !ok {
		t.Fatalf("expected embeddedAssets when DevAssets empty")
	}
}

func TestBuildChatAgentFallsBackWhenUnconfigured(t *testing.T) {
	agent, timeout, reason := buildChatAgent(settings.ChatAgent{})
	if agent == nil {
		t.Fatal("expected fallback agent, got nil")
	}
	if timeout <= 0 {
		t.Fatalf("unexpected timeout: %s", timeout)
	}
	if reason == "" {
		t.Fatal("expected fallback reason")
	}
	resp, err := agent.Run(context.Background(), chatagent.RunRequest{
		ProjectName: "demo",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("run fallback agent: %v", err)
	}
	if resp.Metadata["provider"] != "fallback" {
		t.Fatalf("provider=%v", resp.Metadata["provider"])
	}
	if !strings.Contains(resp.Text, "chat agent is not configured") {
		t.Fatalf("fallback response did not explain the configuration gap: %q", resp.Text)
	}
}
