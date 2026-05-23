package sse_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/web/sse"
)

func TestStreamWritesSSEFrames(t *testing.T) {
	broker := events.NewBroker(4)
	defer broker.Close()
	sub := broker.Subscribe("s")
	defer sub.Close()
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sse.Stream(ctx, rec, sub, 10*time.Millisecond) }()
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
