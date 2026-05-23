package slack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientAuthTest(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"team_id":"T1","user_id":"U1"}`))
	}))
	t.Cleanup(server.Close)

	c := newClient("xapp-1", "xoxb-1")
	c.baseURL = server.URL
	c.http = server.Client()

	resp, err := c.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("auth test: %v", err)
	}
	if gotPath != "/auth.test" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer xoxb-1" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if resp.TeamID != "T1" || resp.UserID != "U1" {
		t.Fatalf("response=%+v", resp)
	}
}

func TestClientOpenSocketMode(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"url":"wss://example.invalid/socket"}`))
	}))
	t.Cleanup(server.Close)

	c := newClient("xapp-1", "xoxb-1")
	c.baseURL = server.URL
	c.http = server.Client()

	url, err := c.OpenSocketMode(context.Background())
	if err != nil {
		t.Fatalf("open socket mode: %v", err)
	}
	if url != "wss://example.invalid/socket" {
		t.Fatalf("url=%q", url)
	}
	if gotBody != "" {
		t.Fatalf("expected empty body, got %q", gotBody)
	}
}

func TestClientPostMessage(t *testing.T) {
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

	c := newClient("xapp-1", "xoxb-1")
	c.baseURL = server.URL
	c.http = server.Client()

	if err := c.PostMessage(context.Background(), PostMessageInput{Channel: "C1", ThreadTS: "1700.1", Text: "hello"}); err != nil {
		t.Fatalf("post message: %v", err)
	}
	if gotPath != "/chat.postMessage" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotAuth != "Bearer xoxb-1" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if !strings.Contains(gotBody, `"channel":"C1"`) || !strings.Contains(gotBody, `"thread_ts":"1700.1"`) || !strings.Contains(gotBody, `"text":"hello"`) {
		t.Fatalf("body=%s", gotBody)
	}
}

func TestClientPostMessageReportsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	}))
	t.Cleanup(server.Close)

	c := newClient("xapp-1", "xoxb-1")
	c.baseURL = server.URL
	c.http = server.Client()

	err := c.PostMessage(context.Background(), PostMessageInput{Channel: "C1", Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("expected invalid_auth error, got %v", err)
	}
}
