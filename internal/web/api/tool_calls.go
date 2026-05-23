package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

// toolCallRequest is the body shape for POST tool-calls.
type toolCallRequest struct {
	Name          string `json:"name"`
	MessageID     string `json:"message_id,omitempty"`
	RequestRef    string `json:"request_ref,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Status        string `json:"status,omitempty"`
}

// toolCallUpdateRequest is the body shape for PATCH tool-calls.
type toolCallUpdateRequest struct {
	Status      string `json:"status"`
	ResponseRef string `json:"response_ref,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (s *Service) handleToolCalls(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		calls, err := s.Sessions.ListToolCalls(r.Context(), sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tool_calls": calls})
	case http.MethodPost:
		var req toolCallRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "tool name is required")
			return
		}
		call := persistence.ToolCall{
			SessionID:     sessionID,
			MessageID:     req.MessageID,
			Name:          req.Name,
			RequestRef:    req.RequestRef,
			CorrelationID: req.CorrelationID,
			Status:        persistence.ToolCallStatus(req.Status),
		}
		stored, err := s.Sessions.RecordToolCall(r.Context(), call)
		if err != nil {
			if errors.Is(err, persistence.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.Broker.Publish(sessionID, events.KindToolCall, stored, stored.CorrelationID)
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleToolCallChild matches /api/v1/tool-calls/{id} (PATCH only).
func (s *Service) handleToolCallChild(w http.ResponseWriter, r *http.Request) {
	id, sub := trimPrefixPath(r.URL.Path, "/api/v1/tool-calls/")
	if id == "" || sub != "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req toolCallUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := persistence.ToolCallStatus(req.Status)
	if status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}
	if err := s.Sessions.UpdateToolCallStatus(r.Context(), id, status, req.ResponseRef, req.Error); err != nil {
		if errors.Is(err, persistence.ErrToolCallNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated, err := s.Sessions.GetToolCall(r.Context(), id); err == nil {
		s.Broker.Publish(updated.SessionID, events.KindToolCall, updated, updated.CorrelationID)
	}
	w.WriteHeader(http.StatusNoContent)
}
