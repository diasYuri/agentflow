package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/web/sse"
)

// handleSessionStream answers SSE requests for a single session and
// supports Last-Event-ID / since_sequence based replay from storage.
func (s *Service) handleSessionStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, err := s.Sessions.Get(r.Context(), sessionID); err != nil {
		if errors.Is(err, persistence.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sub := s.Broker.Subscribe(sessionID)
	defer sub.Close()
	// Set SSE headers before any replay so the response is correctly
	// typed as event-stream from the first byte.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if seqStr := r.URL.Query().Get("since_sequence"); seqStr != "" {
		if seq, err := strconv.ParseInt(seqStr, 10, 64); err == nil {
			s.replayMessages(r, w, sessionID, seq)
		}
	}
	if err := sse.Stream(r.Context(), w, sub, 15*time.Second); err != nil {
		if errors.Is(err, http.ErrAbortHandler) {
			panic(err)
		}
	}
}

// handleGlobalStream answers SSE requests that fan in every session.
func (s *Service) handleGlobalStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sub := s.Broker.Subscribe("")
	defer sub.Close()
	if err := sse.Stream(r.Context(), w, sub, 15*time.Second); err != nil {
		if errors.Is(err, http.ErrAbortHandler) {
			panic(err)
		}
	}
}

func (s *Service) replayMessages(r *http.Request, w http.ResponseWriter, sessionID string, since int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	msgs, err := s.Sessions.SinceSequence(r.Context(), sessionID, since)
	if err != nil {
		return
	}
	for _, msg := range msgs {
		ev := events.Event{
			ID:            uint64(msg.Sequence),
			SessionID:     sessionID,
			Kind:          events.KindMessage,
			CorrelationID: msg.CorrelationID,
			OccurredAt:    msg.CreatedAt,
			Payload:       msg,
		}
		_ = sse.WriteEvent(w, ev)
		flusher.Flush()
	}
}
