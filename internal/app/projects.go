package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Project describes a named project root used to resolve workflows.
type Project struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Active bool   `json:"-"`
}

// ProjectStore persists project entries locally.
type ProjectStore interface {
	Load() ([]Project, error)
	Save([]Project) error
}

// JSONProjectStore persists projects in a JSON file.
type JSONProjectStore struct {
	path string
	mu   sync.RWMutex
}

// NewJSONProjectStore creates a JSON-backed project store.
func NewJSONProjectStore(path string) *JSONProjectStore {
	return &JSONProjectStore{path: path}
}

// Load reads projects from disk or returns an empty list.
func (s *JSONProjectStore) Load() ([]Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}

	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return normalizeProjects(projects), nil
}

// Save writes projects to disk.
func (s *JSONProjectStore) Save(projects []Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(normalizeProjects(projects), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// DefaultProjectsPath returns the default local registry path.
func DefaultProjectsPath() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".agentflow", "projects.json")
	}
	return filepath.Join(".agentflow", "projects.json")
}

// ProjectRegistry provides CRUD and lookup operations for projects.
type ProjectRegistry struct {
	store ProjectStore
	mu    sync.RWMutex
}

// NewProjectRegistry creates a registry backed by the provided store.
func NewProjectRegistry(store ProjectStore) *ProjectRegistry {
	if store == nil {
		store = NewJSONProjectStore(DefaultProjectsPath())
	}
	return &ProjectRegistry{store: store}
}

// List returns all configured projects.
func (r *ProjectRegistry) List() ([]Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	projects, err := r.store.Load()
	if err != nil {
		return nil, err
	}
	return normalizeProjects(projects), nil
}

// Add inserts a new project after normalizing its path.
func (r *ProjectRegistry) Add(name, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	projects, err := r.store.Load()
	if err != nil {
		return err
	}
	name = normalizeProjectName(name)
	if err := validateProjectName(name); err != nil {
		return err
	}
	absPath, err := normalizeProjectPath(path)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if project.Name == name {
			return fmt.Errorf("project %q already exists", name)
		}
	}
	projects = append(projects, Project{Name: name, Path: absPath})
	return r.store.Save(projects)
}

// Remove deletes a project by name.
func (r *ProjectRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = normalizeProjectName(name)
	if err := validateProjectName(name); err != nil {
		return err
	}
	projects, err := r.store.Load()
	if err != nil {
		return err
	}
	filtered := projects[:0]
	removed := false
	for _, project := range projects {
		if project.Name == name {
			removed = true
			continue
		}
		filtered = append(filtered, project)
	}
	if !removed {
		return fmt.Errorf("project %q not found", name)
	}
	return r.store.Save(filtered)
}

// Resolve finds a project by name.
func (r *ProjectRegistry) Resolve(name string) (Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name = normalizeProjectName(name)
	if err := validateProjectName(name); err != nil {
		return Project{}, err
	}
	projects, err := r.store.Load()
	if err != nil {
		return Project{}, err
	}
	for _, project := range projects {
		if project.Name == name {
			return project, nil
		}
	}
	return Project{}, fmt.Errorf("project %q not found", name)
}

// ResolveWorkflowRef resolves a workflow reference inside a project when needed.
func ResolveWorkflowRef(project Project, workflowRef string) (string, error) {
	workflowRef = strings.TrimSpace(workflowRef)
	if workflowRef == "" {
		return "", fmt.Errorf("workflow ref is required")
	}
	if isWorkflowPath(workflowRef) {
		return workflowRef, nil
	}

	base := filepath.Join(project.Path, ".agentflow", "workflows")
	if filepath.Ext(workflowRef) != "" {
		candidate := filepath.Join(base, workflowRef)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		return "", fmt.Errorf("workflow %q not found in project %q", workflowRef, project.Name)
	}

	for _, ext := range []string{".yaml", ".yml"} {
		candidate := filepath.Join(base, workflowRef+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("workflow %q not found in project %q", workflowRef, project.Name)
}

func normalizeProjectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("project path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path %q: %w", path, err)
	}
	return filepath.Clean(abs), nil
}

func normalizeProjectName(name string) string {
	return strings.TrimSpace(name)
}

func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("project name %q cannot contain path separators", name)
	}
	return nil
}

func normalizeProjects(projects []Project) []Project {
	if len(projects) == 0 {
		return []Project{}
	}
	out := make([]Project, 0, len(projects))
	for _, project := range projects {
		name := normalizeProjectName(project.Name)
		path := filepath.Clean(strings.TrimSpace(project.Path))
		if name == "" || path == "." {
			continue
		}
		out = append(out, Project{Name: name, Path: path})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func isWorkflowPath(ref string) bool {
	ext := strings.ToLower(filepath.Ext(ref))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	if strings.Contains(ref, string(filepath.Separator)) || filepath.IsAbs(ref) {
		return true
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	return false
}
