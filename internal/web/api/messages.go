package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/diasYuri/agentflow/internal/agentchannel"
	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
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
	if persistence.MessageRole(req.Role) == persistence.MessageRoleUser && s.Channel != nil {
		result, err := s.Channel.SubmitUserMessage(r.Context(), agentchannel.UserMessageInput{
			SessionID:     sessionID,
			Content:       req.Content,
			CorrelationID: req.CorrelationID,
			Metadata:      req.Metadata,
			Async:         true,
		})
		if err != nil {
			writeAppendMessageError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result.Message)
		return
	}
	stored, err := s.Sessions.AppendMessage(r.Context(), sessionID, session.AppendInput{
		Role:          persistence.MessageRole(req.Role),
		Content:       req.Content,
		CorrelationID: req.CorrelationID,
		Metadata:      req.Metadata,
	})
	if err != nil {
		writeAppendMessageError(w, err)
		return
	}
	if s.Channel != nil {
		s.Channel.PublishMessage(stored)
	} else {
		s.Broker.Publish(sessionID, events.KindMessage, stored, stored.CorrelationID)
	}
	writeJSON(w, http.StatusCreated, stored)
}

func writeAppendMessageError(w http.ResponseWriter, err error) {
	if errors.Is(err, persistence.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, session.ErrEmptyContent) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
