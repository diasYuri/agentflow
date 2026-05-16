package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/thejerf/suture/v4"
)

type Server struct {
	cfg       Config
	manager   *Manager
	startedAt time.Time
	stop      context.CancelFunc
	logger    *slog.Logger
}

func NewServer(cfg Config, manager *Manager, startedAt time.Time, stop context.CancelFunc, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{cfg: cfg, manager: manager, startedAt: startedAt, stop: stop, logger: logger}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(parentDir(s.cfg.SocketPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(s.cfg.SocketPath)
	listener, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(s.cfg.SocketPath)
	}()
	if err := os.Chmod(s.cfg.SocketPath, 0o600); err != nil {
		return err
	}
	server := &http.Server{Handler: s.routes()}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return suture.ErrDoNotRestart
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return suture.ErrDoNotRestart
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/daemon/status", s.handleDaemonStatus)
	mux.HandleFunc("/v1/daemon/stop", s.handleDaemonStop)
	mux.HandleFunc("/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/v1/workflows/", s.handleWorkflow)
	return mux
}

func (s *Server) handleDaemonStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, DaemonStatus{
		Running:   true,
		PID:       os.Getpid(),
		StartedAt: s.startedAt,
		Socket:    s.cfg.SocketPath,
		Runs:      len(s.manager.ListWorkflows()),
	})
}

func (s *Server) handleDaemonStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer s.stop()
	writeJSON(w, http.StatusOK, StopResponse{Stopping: true})
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, ListWorkflowsResponse{Runs: s.manager.ListWorkflows()})
	case http.MethodPost:
		var req RunWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := s.manager.StartWorkflow(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, RunWorkflowResponse{Run: run})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/workflows/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	runID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		run, ok := s.manager.WorkflowStatus(runID)
		if !ok {
			writeError(w, http.StatusNotFound, "workflow run not found")
			return
		}
		writeJSON(w, http.StatusOK, RunWorkflowResponse{Run: run})
		return
	}
	if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
		lines, err := s.manager.WorkflowLogs(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, LogsResponse{RunID: runID, Lines: lines})
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		run, err := s.manager.CancelWorkflow(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, CancelWorkflowResponse{Run: run})
		return
	}
	if len(parts) == 2 && parts[1] == "pause" && r.Method == http.MethodPost {
		run, err := s.manager.PauseWorkflow(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, PauseWorkflowResponse{Run: run})
		return
	}
	if len(parts) == 2 && parts[1] == "resume" && r.Method == http.MethodPost {
		run, err := s.manager.ResumeWorkflow(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ResumeWorkflowResponse{Run: run})
		return
	}
	writeError(w, http.StatusNotFound, "endpoint not found")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func statusForError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "only paused runs can be resumed") ||
			strings.Contains(msg, "is not active in this daemon process") ||
			strings.Contains(msg, "has no persisted request") ||
			strings.Contains(msg, "is already success") ||
			strings.Contains(msg, "is already failed") ||
			strings.Contains(msg, "is already cancelled") {
			return http.StatusConflict
		}
	}
	return http.StatusInternalServerError
}

func parentDir(path string) string {
	if i := strings.LastIndex(path, string(os.PathSeparator)); i >= 0 {
		return path[:i]
	}
	return "."
}

func (s *Server) String() string {
	return fmt.Sprintf("agentflowd-rpc(%s)", s.cfg.SocketPath)
}
