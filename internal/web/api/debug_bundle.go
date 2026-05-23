package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/diagnostics"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

// handleDebugBundle streams a zip archive containing redacted
// JSONL, TOML metadata, and README files for offline debugging.
func (s *Service) handleDebugBundle(w http.ResponseWriter, r *http.Request, sessionID string) {
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
	includePayloads, _ := strconv.ParseBool(r.URL.Query().Get("include_payloads"))
	maxPayloadBytes, _ := strconv.ParseInt(r.URL.Query().Get("max_payload_bytes"), 10, 64)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="agentflow-debug-bundle.zip"`)
	if err := s.Bundler.Write(r.Context(), w, diagnostics.BundleOptions{
		SessionID:       sessionID,
		IncludePayloads: includePayloads,
		MaxPayloadBytes: maxPayloadBytes,
		MaxDiagnostics:  parsedLimit(r, 500),
		GeneratedAt:     time.Now().UTC(),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
}
