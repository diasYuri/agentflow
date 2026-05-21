package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/thejerf/suture/v4"
)

func Run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxConcurrentRuns <= 0 {
		cfg.MaxConcurrentRuns = defaultMaxConcurrentRuns
	}
	if cfg.SocketPath == "" || cfg.PIDPath == "" || cfg.LogPath == "" || cfg.RunRoot == "" || cfg.DBPath == "" {
		defaults := DefaultConfig()
		if cfg.SocketPath == "" {
			cfg.SocketPath = defaults.SocketPath
		}
		if cfg.PIDPath == "" {
			cfg.PIDPath = defaults.PIDPath
		}
		if cfg.LogPath == "" {
			cfg.LogPath = defaults.LogPath
		}
		if cfg.RunRoot == "" {
			cfg.RunRoot = defaults.RunRoot
		}
		if cfg.DBPath == "" {
			cfg.DBPath = defaults.DBPath
		}
	}
	if err := os.MkdirAll(filepath.Dir(cfg.PIDPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.RunRoot, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.PIDPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(cfg.PIDPath)
	}()

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	startedAt := time.Now()
	runSupervisor := suture.NewSimple("agentflowd-workflows")
	store, err := OpenSQLiteRunStore(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	manager := NewManagerWithStore(cfg, runSupervisor, logger, store)
	server := NewServer(cfg, manager, startedAt, stop, logger)

	root := suture.New("agentflowd", suture.Spec{
		EventHook: func(event suture.Event) {
			logger.Info("suture event", "event", fmt.Sprint(event))
		},
		Timeout: 10 * time.Second,
	})
	root.Add(runSupervisor)
	root.Add(manager)
	root.Add(server)
	logger.Info("agentflowd starting", "socket", cfg.SocketPath, "run_root", cfg.RunRoot)
	err = root.Serve(ctx)
	if err == nil || errors.Is(err, suture.ErrDoNotRestart) {
		logger.Info("agentflowd stopped")
		return nil
	}
	return err
}
