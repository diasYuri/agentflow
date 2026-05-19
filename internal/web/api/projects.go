package api

import (
	"net/http"
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

func (s *Service) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
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
