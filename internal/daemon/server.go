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
	"strconv"
	"strings"
	"time"

	"github.com/thejerf/suture/v4"

	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
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
	mux.HandleFunc("/v1/workflow-definitions", s.handleWorkflowDefinitions)
	mux.HandleFunc("/v1/workflow-definitions/", s.handleWorkflowDefinition)
	mux.HandleFunc("/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/v1/workflows/", s.handleWorkflow)
	registerDebugRoutes(mux)
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

func (s *Server) handleWorkflowDefinitions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		definitions, err := s.manager.ListWorkflowDefinitions(r.Context())
		if err != nil {
			writeError(w, statusForWorkflowDefinitionError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, WorkflowDefinitionsResponse{Definitions: definitions})
	case http.MethodPost:
		spec, err := decodeWorkflowSpecRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		definition, err := s.manager.CreateWorkflowDefinition(r.Context(), spec)
		if err != nil {
			writeError(w, statusForWorkflowDefinitionError(err), err.Error())
			return
		}
		w.Header().Set("Location", "/v1/workflow-definitions/"+definition.ID)
		writeJSON(w, http.StatusCreated, WorkflowDefinitionResponse{WorkflowDefinition: definition})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/workflow-definitions/"))
	id = strings.Trim(id, "/")
	if id == "" {
		writeError(w, http.StatusNotFound, "workflow definition not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		definition, err := s.manager.GetWorkflowDefinition(r.Context(), id)
		if err != nil {
			writeError(w, statusForWorkflowDefinitionError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, WorkflowDefinitionResponse{WorkflowDefinition: definition})
	case http.MethodPut:
		spec, err := decodeWorkflowSpecRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		definition, err := s.manager.UpdateWorkflowDefinition(r.Context(), id, spec)
		if err != nil {
			writeError(w, statusForWorkflowDefinitionError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, WorkflowDefinitionResponse{WorkflowDefinition: definition})
	case http.MethodDelete:
		if err := s.manager.DeleteWorkflowDefinition(r.Context(), id); err != nil {
			writeError(w, statusForWorkflowDefinitionError(err), err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		cursor, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = defaultEventLimit
		}
		if limit > maxEventLimit {
			limit = maxEventLimit
		}
		if cursor < 0 {
			cursor = 0
		}
		resp, err := s.manager.WorkflowEvents(runID, cursor, limit)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
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
	if len(parts) == 2 && parts[1] == "approve" && r.Method == http.MethodPost {
		run, err := s.manager.ApproveWorkflow(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ApproveWorkflowResponse{Run: run})
		return
	}
	if len(parts) == 2 && parts[1] == "reject" && r.Method == http.MethodPost {
		reason := strings.TrimSpace(r.URL.Query().Get("reason"))
		run, err := s.manager.RejectWorkflow(runID, reason)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, RejectWorkflowResponse{Run: run})
		return
	}
	if len(parts) == 2 && parts[1] == "artifacts" && r.Method == http.MethodGet {
		resp, err := s.manager.WorkflowArtifacts(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "artifact-path" && r.Method == http.MethodGet {
		artifactID := r.URL.Query().Get("artifact_id")
		if artifactID == "" {
			writeError(w, http.StatusBadRequest, "artifact_id is required")
			return
		}
		p, err := s.manager.WorkflowArtifactPath(runID, artifactID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"run_id": runID, "artifact_id": artifactID, "path": p})
		return
	}
	if len(parts) >= 3 && parts[1] == "artifacts" && r.Method == http.MethodGet {
		artifactID := strings.Join(parts[2:], "/")
		resp, err := s.manager.WorkflowArtifact(runID, artifactID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "nodes" && r.Method == http.MethodGet {
		resp, err := s.manager.WorkflowNodes(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) >= 3 && parts[1] == "nodes" && r.Method == http.MethodGet {
		nodeID := strings.Join(parts[2:], "/")
		resp, err := s.manager.WorkflowNode(runID, nodeID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "plan" && r.Method == http.MethodGet {
		resp, err := s.manager.WorkflowPlan(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "summary" && r.Method == http.MethodGet {
		resp, err := s.manager.WorkflowSummary(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "timeline" && r.Method == http.MethodGet {
		cursor, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		resp, err := s.manager.WorkflowTimeline(runID, cursor, limit)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(parts) == 2 && parts[1] == "inspect" && r.Method == http.MethodGet {
		resp, err := s.manager.WorkflowInspect(runID)
		if err != nil {
			writeError(w, statusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
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

func decodeWorkflowSpecRequest(r *http.Request) (coreworkflow.WorkflowSpec, error) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var spec coreworkflow.WorkflowSpec
	if err := decoder.Decode(&spec); err != nil {
		return coreworkflow.WorkflowSpec{}, err
	}
	return spec, nil
}

func statusForWorkflowDefinitionError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrWorkflowDefinitionConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, ErrWorkflowDefinitionInvalid) {
		return http.StatusBadRequest
	}
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not configured") {
			return http.StatusServiceUnavailable
		}
	}
	return http.StatusInternalServerError
}

func statusForError(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "only paused runs can be resumed") ||
			strings.Contains(msg, "only wait_approval runs can be approved") ||
			strings.Contains(msg, "only wait_approval runs can be rejected") ||
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
