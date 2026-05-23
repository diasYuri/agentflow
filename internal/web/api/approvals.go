package api

import (
	"errors"
	"net/http"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

// createApprovalRequest is the body shape for POST approvals.
type createApprovalRequest struct {
	ToolCallID string `json:"tool_call_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// decideApprovalRequest is the body shape for POST /approvals/{id}/decide.
type decideApprovalRequest struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
}

func (s *Service) handleApprovals(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		approvals, err := s.Sessions.ListApprovals(r.Context(), sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
	case http.MethodPost:
		var req createApprovalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		approval := persistence.Approval{
			SessionID:  sessionID,
			ToolCallID: req.ToolCallID,
			Reason:     req.Reason,
		}
		stored, err := s.Sessions.CreateApproval(r.Context(), approval)
		if err != nil {
			if errors.Is(err, persistence.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.Broker.Publish(sessionID, events.KindApproval, stored, "")
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleApprovalChild handles /api/v1/approvals/{id}/decide.
func (s *Service) handleApprovalChild(w http.ResponseWriter, r *http.Request) {
	id, sub := trimPrefixPath(r.URL.Path, "/api/v1/approvals/")
	if id == "" || sub != "decide" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req decideApprovalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := persistence.ApprovalStatus(req.Status)
	switch status {
	case persistence.ApprovalStatusApproved, persistence.ApprovalStatusRejected:
	default:
		writeError(w, http.StatusBadRequest, "status must be approved or rejected")
		return
	}
	if err := s.Sessions.DecideApproval(r.Context(), id, status, req.Reason, req.DecidedBy); err != nil {
		if errors.Is(err, persistence.ErrApprovalNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
