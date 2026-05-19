package session_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/web/persistence"
	"github.com/diasYuri/agentflow/internal/web/session"
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

func openSessions(t *testing.T, projects session.ProjectResolver) *session.Sessions {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "web.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc, err := session.NewSessions(session.Options{DB: db, Projects: projects})
	if err != nil {
		t.Fatalf("new sessions: %v", err)
	}
	return svc
}

func TestCreateSessionSnapshotsProjectRoot(t *testing.T) {
	projects := newStubProjects(app.Project{Name: "demo", Path: "/projects/demo"})
	svc := openSessions(t, projects)
	created, err := svc.Create(context.Background(), session.CreateInput{ProjectName: "demo", Title: "first"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ProjectPath != "/projects/demo" {
		t.Fatalf("expected snapshot of project path, got %q", created.ProjectPath)
	}
	// Mutate the registry to confirm the snapshot does not drift.
	projects.byName["demo"] = app.Project{Name: "demo", Path: "/projects/demo-renamed"}
	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ProjectPath != "/projects/demo" {
		t.Fatalf("snapshot drifted: %q", got.ProjectPath)
	}
}

func TestAssertProjectMatchesBlocksSwitch(t *testing.T) {
	projects := newStubProjects(app.Project{Name: "demo", Path: "/p"})
	svc := openSessions(t, projects)
	created, err := svc.Create(context.Background(), session.CreateInput{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AssertProjectMatches(context.Background(), created.ID, "demo"); err != nil {
		t.Fatalf("matching project should succeed: %v", err)
	}
	_, err = svc.AssertProjectMatches(context.Background(), created.ID, "other")
	if !errors.Is(err, session.ErrProjectSwitch) {
		t.Fatalf("expected ErrProjectSwitch, got %v", err)
	}
}

func TestAppendMessageOffloadsLargePayloads(t *testing.T) {
	projects := newStubProjects(app.Project{Name: "demo", Path: "/p"})
	svc := openSessions(t, projects)
	created, err := svc.Create(context.Background(), session.CreateInput{ProjectName: "demo"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	small, err := svc.AppendMessage(context.Background(), created.ID, session.AppendInput{
		Role:    persistence.MessageRoleUser,
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("append small: %v", err)
	}
	if small.PayloadRef != "" || small.Content != "hello" {
		t.Fatalf("small message should inline: %+v", small)
	}
	large := strings.Repeat("x", persistence.MaxInlineMessageBytes+100)
	big, err := svc.AppendMessage(context.Background(), created.ID, session.AppendInput{
		Role:    persistence.MessageRoleAssistant,
		Content: large,
	})
	if err != nil {
		t.Fatalf("append big: %v", err)
	}
	if big.PayloadRef == "" || big.Content != "" {
		t.Fatalf("large message should offload: %+v", big)
	}
	body, _, err := svc.ResolvePayload(context.Background(), big.PayloadRef)
	if err != nil {
		t.Fatalf("resolve payload: %v", err)
	}
	if string(body) != large {
		t.Fatalf("payload content mismatch")
	}
}

func TestAppendMessageRejectsEmptyContent(t *testing.T) {
	projects := newStubProjects(app.Project{Name: "demo", Path: "/p"})
	svc := openSessions(t, projects)
	created, _ := svc.Create(context.Background(), session.CreateInput{ProjectName: "demo"})
	_, err := svc.AppendMessage(context.Background(), created.ID, session.AppendInput{Role: persistence.MessageRoleUser, Content: " \t"})
	if !errors.Is(err, session.ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}
