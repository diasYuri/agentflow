package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/web/settings"
)

func defaultTestSettings() settings.Settings {
	cfg := settings.Defaults()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Server.OpenBrowser = false
	cfg.Paths.Root = "/tmp/agentflow-web-test"
	return cfg
}

func newTestServer(t *testing.T) (*Server, *Auth) {
	t.Helper()
	auth, err := NewAuth("test-token")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	auth.SetAllowNonLoopback(true)
	srv, err := NewServer(Options{
		Settings: defaultTestSettings(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     auth,
		Assets:   NewEmbeddedAssets(),
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	return srv, auth
}

func doRequest(t *testing.T, h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:5555"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthEndpointIsPublic(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status=%s", resp.Status)
	}
}

func TestSettingsEndpointRequiresAuth(t *testing.T) {
	srv, auth := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/api/v1/settings", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	rec = doRequest(t, srv.handler(), http.MethodGet, "/api/v1/settings", map[string]string{
		"Authorization": "Bearer " + auth.Token(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp SettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Web.Host != "127.0.0.1" {
		t.Fatalf("host=%s", resp.Web.Host)
	}
}

func TestNonLoopbackRequestsAreRefused(t *testing.T) {
	auth, _ := NewAuth("token")
	srv, _ := NewServer(Options{
		Settings: defaultTestSettings(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     auth,
		Assets:   NewEmbeddedAssets(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.RemoteAddr = "192.0.2.10:5555"
	rec := httptest.NewRecorder()
	srv.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestShellRouteServesHTML(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "AgentFlow Web") {
		t.Fatalf("body did not contain shell marker: %s", rec.Body.String())
	}
}

func TestStaticRouteServesEmbeddedAssetsPublicly(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/static/shell.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "shell ready") {
		t.Fatalf("body did not contain expected marker: %q", rec.Body.String())
	}

	rec = doRequest(t, srv.handler(), http.MethodHead, "/static/shell.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("head status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFaviconRouteIsPublicNoContent(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/favicon.ico", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkflowRouteServesHTML(t *testing.T) {
	srv, auth := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/workflow", map[string]string{
		"Authorization": "Bearer " + auth.Token(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "AgentFlow Web") {
		t.Fatalf("body did not contain shell marker: %s", rec.Body.String())
	}
}

func TestQueryTokenIsAccepted(t *testing.T) {
	srv, auth := newTestServer(t)
	rec := doRequest(t, srv.handler(), http.MethodGet, "/api/v1/settings?token="+auth.Token(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServerListensAndShutsDown(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := srv.Listen(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("network sandbox blocked listen: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("addr=%s", addr)
	}
	resp, err := http.Get("http://" + addr + "/api/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
