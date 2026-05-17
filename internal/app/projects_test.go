package app

import (
	"path/filepath"
	"testing"
)

type memoryProjectStore struct {
	projects []Project
	err      error
}

func (m *memoryProjectStore) Load() ([]Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]Project, len(m.projects))
	copy(out, m.projects)
	return out, nil
}

func (m *memoryProjectStore) Save(projects []Project) error {
	if m.err != nil {
		return m.err
	}
	m.projects = make([]Project, len(projects))
	copy(m.projects, projects)
	return nil
}

func TestProjectRegistryAddNormalizesPath(t *testing.T) {
	tmp := t.TempDir()
	store := &memoryProjectStore{}
	registry := NewProjectRegistry(store)

	if err := registry.Add("alpha", filepath.Join(tmp, "workspace")); err != nil {
		t.Fatalf("add project: %v", err)
	}
	if len(store.projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(store.projects))
	}
	if !filepath.IsAbs(store.projects[0].Path) {
		t.Fatalf("expected absolute path, got %q", store.projects[0].Path)
	}
}

func TestProjectRegistryRejectsDuplicateNames(t *testing.T) {
	store := &memoryProjectStore{projects: []Project{{Name: "alpha", Path: "/one"}}}
	registry := NewProjectRegistry(store)

	if err := registry.Add("alpha", "/two"); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestProjectRegistryRemoveAndList(t *testing.T) {
	store := &memoryProjectStore{projects: []Project{
		{Name: "beta", Path: "/beta"},
		{Name: "alpha", Path: "/alpha"},
	}}
	registry := NewProjectRegistry(store)

	list, err := registry.List()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("expected sorted list, got %#v", list)
	}

	if err := registry.Remove("alpha"); err != nil {
		t.Fatalf("remove project: %v", err)
	}
	if len(store.projects) != 1 || store.projects[0].Name != "beta" {
		t.Fatalf("unexpected projects after remove: %#v", store.projects)
	}
}
