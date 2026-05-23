package slack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"log/slog"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/agentchannel/session"
	"github.com/diasYuri/agentflow/internal/app"
)

func TestSlackAppMentionBuildsUserMessageInput(t *testing.T) {
	ev := slackEvent{
		Type:     "app_mention",
		User:     "U1",
		Text:     "<@UBOT> hello there",
		Channel:  "C1",
		ThreadTS: "1700000000.000100",
		TS:       "1700000000.000100",
	}
	input, ok := ev.toUserMessageInput("demo", "T1", "UBOT", "env-1")
	if !ok {
		t.Fatal("expected app mention to be accepted")
	}
	if input.Content != "hello there" {
		t.Fatalf("content=%q", input.Content)
	}
	if input.Conversation.Source != "slack" || input.Conversation.ExternalKey != "slack:T1:C1:1700000000.000100" {
		t.Fatalf("conversation=%+v", input.Conversation)
	}
	if input.CorrelationID != "env-1" || !input.Async {
		t.Fatalf("unexpected input: %+v", input)
	}
}

func TestSlackDirectMessageBuildsUserMessageInput(t *testing.T) {
	ev := slackEvent{
		Type:        "message",
		User:        "U1",
		Text:        "hello",
		Channel:     "D1",
		ChannelType: "im",
		TS:          "1700000000.000200",
	}
	input, ok := ev.toUserMessageInput("demo", "T1", "UBOT", "env-2")
	if !ok {
		t.Fatal("expected DM to be accepted")
	}
	if input.Content != "hello" {
		t.Fatalf("content=%q", input.Content)
	}
	if input.Conversation.ExternalKey != "slack:T1:D1:1700000000.000200" {
		t.Fatalf("external key=%q", input.Conversation.ExternalKey)
	}
}

func TestSlackRejectsBotAndUnsupportedMessages(t *testing.T) {
	cases := []slackEvent{
		{Type: "message", ChannelType: "im", Text: "hi", Channel: "D1", BotID: "B1"},
		{Type: "reaction_added", Text: "hi", Channel: "D1"},
		{Type: "app_mention", Text: "<@UBOT>", Channel: "C1"},
	}
	for i, ev := range cases {
		if _, ok := ev.toUserMessageInput("demo", "T1", "UBOT", "env"); ok {
			t.Fatalf("case %d should have been rejected: %+v", i, ev)
		}
	}
}

func TestResponderPostsAssistantReply(t *testing.T) {
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), dir+"/slack.sqlite")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	projects := stubProjects{byName: map[string]app.Project{
		"demo": {Name: "demo", Path: "/workspace"},
	}}
	sessions, err := session.NewSessions(session.Options{DB: db, Projects: projects})
	if err != nil {
		t.Fatalf("new sessions: %v", err)
	}
	sess, err := sessions.Create(context.Background(), session.CreateInput{
		ProjectName:         "demo",
		Title:               "Slack thread",
		Source:              "slack",
		ExternalKey:         "slack:T1:C1:1700000000.0001",
		ExternalWorkspaceID: "T1",
		ExternalChannelID:   "C1",
		ExternalThreadID:    "1700000000.0001",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var gotPath, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	client := newClient("xapp", "xoxb")
	client.baseURL = server.URL
	client.http = server.Client()
	r := newResponder(sessions, events.NewBroker(8), client, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err = r.postAssistant(context.Background(), persistence.Message{
		SessionID: sess.ID,
		Role:      persistence.MessageRoleAssistant,
		Content:   "done",
	})
	if err != nil {
		t.Fatalf("post assistant: %v", err)
	}
	if gotPath != "/chat.postMessage" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer xoxb") {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"channel":"C1"`) || !strings.Contains(gotBody, `"thread_ts":"1700000000.0001"`) {
		t.Fatalf("unexpected body %s", gotBody)
	}
}

type stubProjects struct {
	byName map[string]app.Project
}

func (s stubProjects) Resolve(name string) (app.Project, error) {
	if p, ok := s.byName[name]; ok {
		return p, nil
	}
	return app.Project{}, fmt.Errorf("project %q not found", name)
}

func (s stubProjects) List() ([]app.Project, error) { return nil, nil }
