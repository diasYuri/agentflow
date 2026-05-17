package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/version"
)

var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
	buildBy      = ""
)

func init() {
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate
	version.BuiltBy = buildBy
}

func debugLevel() slog.Level {
	if os.Getenv("AGENTFLOWD_DEBUG") == "1" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func main() {
	cfg := daemon.DefaultConfig()
	if value := os.Getenv("AGENTFLOWD_SOCKET"); value != "" {
		cfg.SocketPath = value
	}
	if value := os.Getenv("AGENTFLOWD_RUN_ROOT"); value != "" {
		cfg.RunRoot = value
	}
	if value := os.Getenv("AGENTFLOWD_DB"); value != "" {
		cfg.DBPath = value
	}
	if value := os.Getenv("AGENTFLOW_CODEX_PATH"); value != "" {
		cfg.CodexPath = value
	}
	if value := os.Getenv("AGENTFLOW_CLAUDE_PATH"); value != "" {
		cfg.ClaudePath = value
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: debugLevel()}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, cfg, logger); err != nil {
		logger.Error("agentflowd exited with error", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
