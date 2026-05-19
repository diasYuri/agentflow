package events_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/web/events"
)

func TestPublishDeliversToSessionSubscriber(t *testing.T) {
	broker := events.NewBroker(4)
	defer broker.Close()
	sub := broker.Subscribe("session-a")
	defer sub.Close()
	broker.Publish("session-a", events.KindMessage, map[string]string{"text": "hi"}, "corr-1")
	select {
	case ev := <-sub.C:
		if ev.Kind != events.KindMessage {
			t.Fatalf("unexpected kind: %v", ev.Kind)
		}
		if ev.SessionID != "session-a" {
			t.Fatalf("session mismatch: %v", ev.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishSkipsUnrelatedSessions(t *testing.T) {
	broker := events.NewBroker(4)
	defer broker.Close()
	sub := broker.Subscribe("session-a")
	defer sub.Close()
	broker.Publish("session-b", events.KindMessage, "ignored", "")
	select {
	case ev := <-sub.C:
		t.Fatalf("did not expect to receive event for other session: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWildcardSubscriberReceivesAllSessions(t *testing.T) {
	broker := events.NewBroker(4)
	defer broker.Close()
	sub := broker.Subscribe("")
	defer sub.Close()
	broker.Publish("a", events.KindToolCall, nil, "")
	broker.Publish("b", events.KindToolCall, nil, "")
	for i := 0; i < 2; i++ {
		select {
		case <-sub.C:
		case <-time.After(time.Second):
			t.Fatalf("missed event %d", i)
		}
	}
}

func TestStreamWritesSSEFrames(t *testing.T) {
	broker := events.NewBroker(4)
	defer broker.Close()
	sub := broker.Subscribe("s")
	defer sub.Close()
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- events.Stream(ctx, rec, sub, 10*time.Millisecond) }()
	broker.Publish("s", events.KindMessage, map[string]string{"msg": "hello"}, "corr")
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Fatalf("stream error: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: message") {
		t.Fatalf("expected event frame, got: %q", body)
	}
	if !strings.Contains(body, "id: 1") {
		t.Fatalf("expected id frame, got: %q", body)
	}
	if !strings.Contains(body, "\"corr\"") {
		t.Fatalf("expected correlation id in payload, got: %q", body)
	}
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	broker := events.NewBroker(1)
	defer broker.Close()
	sub := broker.Subscribe("s")
	defer sub.Close()
	for i := 0; i < 5; i++ {
		broker.Publish("s", events.KindHeartbeat, i, "")
	}
	if sub.Dropped() == 0 {
		t.Fatalf("expected dropped events for slow subscriber")
	}
}
