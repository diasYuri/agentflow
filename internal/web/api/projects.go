package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/diasYuri/agentflow/internal/app"
)

// ProjectResponse is the wire shape of a project record. It mirrors
// app.Project without leaking the registry-specific tags.
type ProjectResponse struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type projectsListResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

type createProjectRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type pickFolderResponse struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (s *Service) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListProjects(w, r)
	case http.MethodPost:
		s.handleCreateProject(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleListProjects(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.Projects.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := projectsListResponse{Projects: make([]ProjectResponse, 0, len(projects))}
	for _, p := range projects {
		out.Projects = append(out.Projects, ProjectResponse{Name: p.Name, Path: p.Path})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	adder, ok := s.Projects.(projectAdder)
	if !ok {
		writeError(w, http.StatusNotImplemented, "project registry does not support creation")
		return
	}
	var req createProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := adder.Add(req.Name, req.Path); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	project, err := s.Projects.Resolve(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ProjectResponse{Name: project.Name, Path: project.Path})
}

func (s *Service) handlePickProjectFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.FolderPicker == nil {
		writeError(w, http.StatusNotImplemented, "folder picker is not configured")
		return
	}
	path, err := s.FolderPicker.PickFolder(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path = filepath.Clean(strings.TrimSpace(path))
	writeJSON(w, http.StatusOK, pickFolderResponse{Path: path, Name: filepath.Base(path)})
}

// handleProjectChild matches /api/v1/projects/{name}/...
func (s *Service) handleProjectChild(w http.ResponseWriter, r *http.Request) {
	name, sub := trimPrefixPath(r.URL.Path, "/api/v1/projects/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	project, err := s.Projects.Resolve(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, ProjectResponse{Name: project.Name, Path: project.Path})
	case strings.HasPrefix(sub, "sessions"):
		s.handleProjectSessions(w, r, project, strings.TrimPrefix(sub, "sessions"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Service) handleProjectSessions(w http.ResponseWriter, r *http.Request, project app.Project, rest string) {
	rest = strings.Trim(rest, "/")
	if rest != "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.Sessions.List(r.Context(), project.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case http.MethodPost:
		s.handleCreateSession(w, r, project.Name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
