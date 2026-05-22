package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
)

// appendMessageRequest is the body shape for POST messages.
type appendMessageRequest struct {
	Role          string         `json:"role"`
	Content       string         `json:"content"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (s *Service) handleMessages(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		s.handleListMessages(w, r, sessionID)
	case http.MethodPost:
		s.handleAppendMessage(w, r, sessionID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleListMessages(w http.ResponseWriter, r *http.Request, sessionID string) {
	if since := r.URL.Query().Get("since_sequence"); since != "" {
		seq, err := strconv.ParseInt(since, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since_sequence must be int64")
			return
		}
		msgs, err := s.Sessions.SinceSequence(r.Context(), sessionID, seq)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
		return
	}
	limit := parsedLimit(r, 0)
	msgs, err := s.Sessions.ListMessages(r.Context(), sessionID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Service) handleAppendMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	var req appendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, err := s.Sessions.AppendMessage(r.Context(), sessionID, session.AppendInput{
		Role:          persistence.MessageRole(req.Role),
		Content:       req.Content,
		CorrelationID: req.CorrelationID,
		Metadata:      req.Metadata,
	})
	if err != nil {
		if errors.Is(err, persistence.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, session.ErrEmptyContent) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Broker.Publish(sessionID, events.KindMessage, stored, stored.CorrelationID)
	writeJSON(w, http.StatusCreated, stored)
	if stored.Role == persistence.MessageRoleUser {
		s.scheduleChatAgent(sessionID, stored)
	}
}
