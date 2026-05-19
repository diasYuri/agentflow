package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedAssetsServeShell(t *testing.T) {
	p := NewEmbeddedAssets()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p.ServeShell(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "AgentFlow Web") {
		t.Fatalf("shell body missing marker: %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content-type=%q", got)
	}
}

func TestEmbeddedAssetsServeStaticFile(t *testing.T) {
	p := NewEmbeddedAssets()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/shell.js", nil)
	p.ServeAsset(rec, req, "shell.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "shell ready") {
		t.Fatalf("body did not contain expected marker: %q", rec.Body.String())
	}
}

func TestEmbeddedShellJsParses(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}
	cmd := exec.Command("node", "--check", filepath.Join("assets", "static", "shell.js"))
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node --check failed: %v\n%s", err, string(out))
	}
}

func TestEmbeddedAssetsRefuseTraversal(t *testing.T) {
	p := NewEmbeddedAssets()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/../shell.html", nil)
	p.ServeAsset(rec, req, "../shell.html")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDevAssetsPreferDirectoryThenFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>dev-shell"), 0o644); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "static", "dev.css"), []byte("body{color:red}"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	p := NewDevAssets(dir)

	shellRec := httptest.NewRecorder()
	p.ServeShell(shellRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if shellRec.Code != http.StatusOK {
		t.Fatalf("shell status=%d", shellRec.Code)
	}
	if !strings.Contains(shellRec.Body.String(), "dev-shell") {
		t.Fatalf("expected dev-shell content, got %s", shellRec.Body.String())
	}

	cssRec := httptest.NewRecorder()
	p.ServeAsset(cssRec, httptest.NewRequest(http.MethodGet, "/assets/dev.css", nil), "dev.css")
	if cssRec.Code != http.StatusOK {
		t.Fatalf("css status=%d", cssRec.Code)
	}
	if !strings.Contains(cssRec.Body.String(), "color:red") {
		t.Fatalf("dev css content unexpected: %s", cssRec.Body.String())
	}

	fallbackRec := httptest.NewRecorder()
	p.ServeAsset(fallbackRec, httptest.NewRequest(http.MethodGet, "/assets/shell.js", nil), "shell.js")
	if fallbackRec.Code != http.StatusOK {
		t.Fatalf("fallback status=%d", fallbackRec.Code)
	}
	if !strings.Contains(fallbackRec.Body.String(), "shell ready") {
		t.Fatalf("expected embedded fallback, got %s", fallbackRec.Body.String())
	}
}
