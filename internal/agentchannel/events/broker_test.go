package events_test

import (
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
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
