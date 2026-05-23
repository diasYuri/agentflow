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
}

func (f *fakeAgent) Run(_ context.Context, req chatagent.RunRequest) (chatagent.RunResponse, error) {
	f.req = req
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
