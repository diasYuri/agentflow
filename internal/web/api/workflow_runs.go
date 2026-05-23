package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/diasYuri/agentflow/internal/daemon"
)

const (
	defaultWorkflowCursor = 0
	defaultWorkflowLimit  = 100
	maxWorkflowLimit      = 1000
)

func (s *Service) handleWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	if s.WorkflowRuns == nil {
		writeError(w, http.StatusServiceUnavailable, "workflow runs require agentflowd")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := s.WorkflowRuns.ListWorkflows(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req daemon.RunWorkflowRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := s.WorkflowRuns.RunWorkflow(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleWorkflowRun(w http.ResponseWriter, r *http.Request) {
	if s.WorkflowRuns == nil {
		writeError(w, http.StatusServiceUnavailable, "workflow runs require agentflowd")
		return
	}
	runID, child := trimPrefixPath(r.URL.Path, "/api/v1/workflows/")
	if runID == "" {
		writeError(w, http.StatusNotFound, "workflow run not found")
		return
	}
	switch {
	case child == "" && r.Method == http.MethodGet:
		resp, err := s.WorkflowRuns.WorkflowStatus(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "inspect" && r.Method == http.MethodGet:
		resp, err := s.WorkflowRuns.WorkflowInspect(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "nodes" && r.Method == http.MethodGet:
		resp, err := s.WorkflowRuns.WorkflowNodes(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "summary" && r.Method == http.MethodGet:
		resp, err := s.WorkflowRuns.WorkflowSummary(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "artifacts" && r.Method == http.MethodGet:
		resp, err := s.WorkflowRuns.WorkflowArtifacts(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "events" && r.Method == http.MethodGet:
		cursor, limit := workflowCursorAndLimit(r)
		resp, err := s.WorkflowRuns.WorkflowEvents(r.Context(), runID, cursor, limit)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "timeline" && r.Method == http.MethodGet:
		cursor, limit := workflowCursorAndLimit(r)
		resp, err := s.WorkflowRuns.WorkflowTimeline(r.Context(), runID, cursor, limit)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "cancel" && r.Method == http.MethodPost:
		resp, err := s.WorkflowRuns.CancelWorkflow(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "pause" && r.Method == http.MethodPost:
		resp, err := s.WorkflowRuns.PauseWorkflow(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "resume" && r.Method == http.MethodPost:
		resp, err := s.WorkflowRuns.ResumeWorkflow(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "approve" && r.Method == http.MethodPost:
		resp, err := s.WorkflowRuns.ApproveWorkflow(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	case child == "reject" && r.Method == http.MethodPost:
		resp, err := s.WorkflowRuns.RejectWorkflow(r.Context(), runID)
		writeWorkflowRunProxyResponse(w, resp, err)
	default:
		if strings.Contains(child, "/") {
			writeError(w, http.StatusNotFound, "workflow run endpoint not found")
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeWorkflowRunProxyResponse(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func workflowCursorAndLimit(r *http.Request) (int, int) {
	q := r.URL.Query()
	cursor := defaultWorkflowCursor
	if value := q.Get("cursor"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			cursor = parsed
		}
	}
	limit := defaultWorkflowLimit
	if value := q.Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxWorkflowLimit {
		limit = maxWorkflowLimit
	}
	return cursor, limit
}
