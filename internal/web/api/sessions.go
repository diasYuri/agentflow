package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
)

// createSessionRequest is the body shape for POSTing a new session.
type createSessionRequest struct {
	Title    string         `json:"title,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Model    string         `json:"model,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// updateSessionRequest is the body shape for PATCH /sessions/{id}.
type updateSessionRequest struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

// handleSessions answers /api/v1/sessions (currently only GET to list).
func (s *Service) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	sessions, err := s.Sessions.List(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// handleSessionChild handles /api/v1/sessions/{id}/...
func (s *Service) handleSessionChild(w http.ResponseWriter, r *http.Request) {
	sessionID, sub := trimPrefixPath(r.URL.Path, "/api/v1/sessions/")
	if sessionID == "" {
		http.NotFound(w, r)
		return
	}
	switch {
	case sub == "":
		s.handleSessionRoot(w, r, sessionID)
	case sub == "messages":
		s.handleMessages(w, r, sessionID)
	case sub == "tool-calls":
		s.handleToolCalls(w, r, sessionID)
	case sub == "approvals":
		s.handleApprovals(w, r, sessionID)
	case sub == "diagnostics":
		s.handleDiagnostics(w, r, sessionID)
	case sub == "frontend-events":
		s.handleFrontendEvents(w, r, sessionID)
	case sub == "stream":
		s.handleSessionStream(w, r, sessionID)
	case sub == "debug-bundle":
		s.handleDebugBundle(w, r, sessionID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) handleSessionRoot(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSession(w, r, sessionID)
	case http.MethodPatch:
		s.handleUpdateSession(w, r, sessionID)
	case http.MethodDelete:
		s.handleDeleteSession(w, r, sessionID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleCreateSession(w http.ResponseWriter, r *http.Request, projectName string) {
	var req createSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.Sessions.Create(r.Context(), session.CreateInput{
		ProjectName: projectName,
		Title:       req.Title,
		Provider:    req.Provider,
		Model:       req.Model,
		Metadata:    req.Metadata,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Broker.Publish(created.ID, events.KindSessionInfo, created, "")
	writeJSON(w, http.StatusCreated, created)
}

func (s *Service) handleGetSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	got, err := s.Sessions.Get(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, persistence.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Service) handleUpdateSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req updateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title != nil {
		if err := s.Sessions.SetTitle(r.Context(), sessionID, *req.Title); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Status != nil {
		status := persistence.SessionStatus(*req.Status)
		switch status {
		case persistence.SessionStatusOpen, persistence.SessionStatusArchived:
			if err := s.Sessions.SetStatus(r.Context(), sessionID, status); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "unsupported status")
			return
		}
	}
	got, err := s.Sessions.Get(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Service) handleDeleteSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := s.Sessions.Delete(r.Context(), sessionID); err != nil {
		if errors.Is(err, persistence.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parsedLimit reads ?limit= safely.
func parsedLimit(r *http.Request, def int) int {
	q := r.URL.Query().Get("limit")
	if q == "" {
		return def
	}
	n, err := strconv.Atoi(q)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
