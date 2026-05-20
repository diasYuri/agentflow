package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/web/api"
	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
)

type stubProjects struct {
	byName map[string]app.Project
}

func newStubProjects(projects ...app.Project) *stubProjects {
	out := &stubProjects{byName: make(map[string]app.Project)}
	for _, project := range projects {
		out.byName[project.Name] = project
	}
	return out
}

func (s *stubProjects) Resolve(name string) (app.Project, error) {
	if p, ok := s.byName[name]; ok {
		return p, nil
	}
	return app.Project{}, fmt.Errorf("project %q not found", name)
}

func (s *stubProjects) List() ([]app.Project, error) {
	out := make([]app.Project, 0, len(s.byName))
	for _, p := range s.byName {
		out = append(out, p)
	}
	return out, nil
}

func (s *stubProjects) Add(name, path string) error {
	if _, ok := s.byName[name]; ok {
		return fmt.Errorf("project %q already exists", name)
	}
	s.byName[name] = app.Project{Name: name, Path: path}
	return nil
}

func newTestService(t *testing.T) (*api.Service, *http.ServeMux, *events.Broker) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "web.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	projects := newStubProjects(app.Project{Name: "demo", Path: "/p"})
	broker := events.NewBroker(8)
	svc, err := api.NewService(api.Options{DB: db, Projects: projects, Broker: broker})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	mux := http.NewServeMux()
	svc.Register(mux)
	t.Cleanup(func() { svc.Close() })
	return svc, mux, broker
}

func doReq(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewBuffer(data)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) persistence.Session {
	t.Helper()
	var s persistence.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return s
}
