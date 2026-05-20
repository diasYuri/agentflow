package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type workflowDefinitionRequest struct {
	YAML string `json:"yaml"`
}

func (s *Service) handleWorkflowDefinitions(w http.ResponseWriter, r *http.Request) {
	if s.WorkflowDefinitions == nil {
		writeError(w, http.StatusServiceUnavailable, "workflow definitions require agentflowd")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := s.WorkflowDefinitions.ListWorkflowDefinitions(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		spec, err := decodeWorkflowDefinitionSpec(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := s.WorkflowDefinitions.CreateWorkflowDefinition(r.Context(), spec)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, resp)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	if s.WorkflowDefinitions == nil {
		writeError(w, http.StatusServiceUnavailable, "workflow definitions require agentflowd")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/workflow-definitions/"), "/")
	if id == "" {
		writeError(w, http.StatusNotFound, "workflow definition not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := s.WorkflowDefinitions.WorkflowDefinition(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPut:
		spec, err := decodeWorkflowDefinitionSpec(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := s.WorkflowDefinitions.UpdateWorkflowDefinition(r.Context(), id, spec)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodDelete:
		if err := s.WorkflowDefinitions.DeleteWorkflowDefinition(r.Context(), id); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeWorkflowDefinitionSpec(r *http.Request) (workflow.WorkflowSpec, error) {
	var raw json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		return workflow.WorkflowSpec{}, err
	}
	var yamlReq workflowDefinitionRequest
	if err := json.Unmarshal(raw, &yamlReq); err == nil && strings.TrimSpace(yamlReq.YAML) != "" {
		var spec workflow.WorkflowSpec
		if err := yaml.Unmarshal([]byte(yamlReq.YAML), &spec); err != nil {
			return workflow.WorkflowSpec{}, err
		}
		return spec, nil
	}
	var spec workflow.WorkflowSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return workflow.WorkflowSpec{}, err
	}
	return spec, nil
}
