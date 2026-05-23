package api

import (
	"net/http"
	"strings"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

// recordDiagnosticRequest is the body shape for POST diagnostics.
type recordDiagnosticRequest struct {
	Level         string         `json:"level"`
	Source        string         `json:"source"`
	Code          string         `json:"code,omitempty"`
	Message       string         `json:"message"`
	Context       map[string]any `json:"context,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
}

// recordFrontendEventRequest is the body shape for POST frontend-events.
type recordFrontendEventRequest struct {
	Kind          string         `json:"kind"`
	Content       string         `json:"content,omitempty"`
	PayloadRef    string         `json:"payload_ref,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (s *Service) handleDiagnostics(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		limit := parsedLimit(r, 100)
		list, err := s.Diagnostics.ListBySession(r.Context(), sessionID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"diagnostics": list})
	case http.MethodPost:
		var req recordDiagnosticRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		level := persistence.DiagnosticLevel(strings.TrimSpace(req.Level))
		if level == "" {
			level = persistence.DiagnosticLevelInfo
		}
		diag := persistence.Diagnostic{
			SessionID:     sessionID,
			Level:         level,
			Source:        strings.TrimSpace(req.Source),
			Code:          strings.TrimSpace(req.Code),
			Message:       strings.TrimSpace(req.Message),
			Context:       req.Context,
			CorrelationID: req.CorrelationID,
		}
		stored, err := s.Diagnostics.Record(r.Context(), diag)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleRecentDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := parsedLimit(r, 100)
	list, err := s.Diagnostics.ListRecent(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": list})
}

func (s *Service) handleFrontendEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		limit := parsedLimit(r, 200)
		list, err := s.Diagnostics.ListEvents(r.Context(), sessionID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"frontend_events": list})
	case http.MethodPost:
		var req recordFrontendEventRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		event := persistence.FrontendEvent{
			SessionID:     sessionID,
			Kind:          strings.TrimSpace(req.Kind),
			Content:       req.Content,
			PayloadRef:    strings.TrimSpace(req.PayloadRef),
			CorrelationID: req.CorrelationID,
			Metadata:      req.Metadata,
		}
		stored, err := s.Diagnostics.RecordFrontendEvent(r.Context(), event)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Surface frontend events to live SSE consumers so the UI can
		// debug user interactions across panes.
		s.Broker.Publish(sessionID, events.KindHeartbeat, stored, stored.CorrelationID)
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
