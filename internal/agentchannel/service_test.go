package agentchannel_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/diasYuri/agentflow/internal/agentchannel"
	"github.com/diasYuri/agentflow/internal/agentchannel/chatagent"
	"github.com/diasYuri/agentflow/internal/agentchannel/diagnostics"
	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
	"github.com/diasYuri/agentflow/internal/app"
)

type stubProjects struct {
	byName map[string]app.Project
}

func (s stubProjects) Resolve(name string) (app.Project, error) {
	if p, ok := s.byName[name]; ok {
		return p, nil
	}
	return app.Project{}, fmt.Errorf("project %q not found", name)
}

func (s stubProjects) List() ([]app.Project, error) {
	out := make([]app.Project, 0, len(s.byName))
	for _, p := range s.byName {
		out = append(out, p)
	}
	return out, nil
}

type fakeAgent struct {
	resp chatagent.RunResponse
	err  error
	req  chatagent.RunRequest
	runs int
}

func (f *fakeAgent) Run(_ context.Context, req chatagent.RunRequest) (chatagent.RunResponse, error) {
	f.req = req
	f.runs++
	return f.resp, f.err
}

func TestSubmitUserMessagePersistsAssistantReply(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := persistence.Open(ctx, filepath.Join(dir, "agentchannel.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	sessions, err := session.NewSessions(session.Options{
		DB:       db,
		Projects: stubProjects{byName: map[string]app.Project{"demo": {Name: "demo", Path: "/p"}}},
	})
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	rec, err := diagnostics.NewRecorder(diagnostics.Options{DB: db, Policy: diagnostics.DefaultPolicy()})
	if err != nil {
		t.Fatalf("diagnostics: %v", err)
	}
	agent := &fakeAgent{resp: chatagent.RunResponse{Text: "assistant reply"}}
	svc, err := agentchannel.NewService(agentchannel.Options{
		Sessions:    sessions,
		Projects:    stubProjects{byName: map[string]app.Project{"demo": {Name: "demo", Path: "/p"}}},
		Diagnostics: rec,
		Events:      events.NewBroker(8),
		ChatAgent:   agent,
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}

	result, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			ProjectName: "demo",
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if result.Session.Source != "slack" || result.Session.ExternalKey == "" {
		t.Fatalf("unexpected session: %+v", result.Session)
	}
	messages, err := sessions.ListMessages(ctx, result.Session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != persistence.MessageRoleUser || messages[1].Content != "assistant reply" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
	if agent.req.ProjectPath != "/p" || agent.req.UserMessage != "hello" {
		t.Fatalf("agent request was not channel-neutral: %+v", agent.req)
	}
}

func TestSubmitUserMessageWithoutProjectRequestsSelection(t *testing.T) {
	ctx := context.Background()
	projects := stubProjects{byName: map[string]app.Project{
		"demo": {Name: "demo", Path: "/p"},
		"app":  {Name: "app", Path: "/app"},
	}}
	svc, sessions := openChannelService(t, projects, &fakeAgent{resp: chatagent.RunResponse{Text: "unused"}})
	result, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "rode o workflow build",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if result.Session.ProjectName != "" {
		t.Fatalf("expected pending session, got %+v", result.Session)
	}
	messages, err := sessions.ListMessages(ctx, result.Session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[1].Role != persistence.MessageRoleAssistant || messages[1].Content != "Escolha um project para esta conversa: app, demo" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestSubmitUserMessageValidProjectSelectionBindsSession(t *testing.T) {
	ctx := context.Background()
	agent := &fakeAgent{resp: chatagent.RunResponse{Text: "unused"}}
	svc, sessions := openChannelService(t, stubProjects{byName: map[string]app.Project{
		"demo": {Name: "demo", Path: "/p"},
	}}, agent)
	first, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "demo",
	})
	if err != nil {
		t.Fatalf("selection submit: %v", err)
	}
	if second.Session.ID != first.Session.ID || second.Session.ProjectName != "demo" || second.Session.ProjectPath != "/p" {
		t.Fatalf("unexpected bound session: %+v", second.Session)
	}
	if agent.runs != 0 {
		t.Fatalf("chat agent should not run during project selection")
	}
	messages, err := sessions.ListMessages(ctx, first.Session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if got := messages[len(messages)-1].Content; got != "Project `demo` vinculado a esta conversa." {
		t.Fatalf("confirmation=%q", got)
	}
}

func TestSubmitUserMessageInvalidProjectListsAgain(t *testing.T) {
	ctx := context.Background()
	svc, sessions := openChannelService(t, stubProjects{byName: map[string]app.Project{
		"demo": {Name: "demo", Path: "/p"},
	}}, &fakeAgent{})
	result, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "missing",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	messages, err := sessions.ListMessages(ctx, result.Session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if got := messages[len(messages)-1].Content; got != "Escolha um project para esta conversa: demo" {
		t.Fatalf("reply=%q", got)
	}
}

func TestSubmitUserMessageWithoutProjectsReportsUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, sessions := openChannelService(t, stubProjects{byName: map[string]app.Project{}}, &fakeAgent{})
	result, err := svc.SubmitUserMessage(ctx, agentchannel.UserMessageInput{
		Conversation: agentchannel.ConversationInput{
			Source:      "slack",
			ExternalKey: "slack:T1:C1:thread",
		},
		Content: "demo",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	messages, err := sessions.ListMessages(ctx, result.Session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if got := messages[len(messages)-1].Content; got != "Nenhum project cadastrado. Cadastre um project antes de continuar esta conversa." {
		t.Fatalf("reply=%q", got)
	}
}

func openChannelService(t *testing.T, projects stubProjects, agent *fakeAgent) (*agentchannel.Service, *session.Sessions) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := persistence.Open(ctx, filepath.Join(dir, "agentchannel.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	sessions, err := session.NewSessions(session.Options{DB: db, Projects: projects})
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	rec, err := diagnostics.NewRecorder(diagnostics.Options{DB: db, Policy: diagnostics.DefaultPolicy()})
	if err != nil {
		t.Fatalf("diagnostics: %v", err)
	}
	svc, err := agentchannel.NewService(agentchannel.Options{
		Sessions:    sessions,
		Projects:    projects,
		Diagnostics: rec,
		Events:      events.NewBroker(8),
		ChatAgent:   agent,
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return svc, sessions
}
