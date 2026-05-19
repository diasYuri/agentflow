package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Auth gates access to the local server. The web command runs as a
// developer tool on localhost, so we only need two layers:
//   - a per-process session token that proves the caller learned the
//     URL the CLI printed (or has access to the persisted state file);
//   - a remote-address check that refuses non-loopback requests so the
//     server cannot be reached if it is accidentally bound to 0.0.0.0.
type Auth struct {
	token            string
	allowNonLoopback bool // tests can disable the loopback check.
}

// NewAuth returns an Auth using the provided token. When token is
// empty a cryptographically random one is generated.
func NewAuth(token string) (*Auth, error) {
	if strings.TrimSpace(token) == "" {
		generated, err := generateToken()
		if err != nil {
			return nil, err
		}
		token = generated
	}
	return &Auth{token: token}, nil
}

// Token returns the active session token. Callers should not log it
// outside the explicit "URL ready" startup banner.
func (a *Auth) Token() string { return a.token }

// SetAllowNonLoopback removes the loopback guard. Reserved for tests.
func (a *Auth) SetAllowNonLoopback(b bool) { a.allowNonLoopback = b }

// Middleware wraps an http.Handler with both the loopback guard and
// the session-token check. GET requests to /, /index.html, /assets/*,
// /static/*, /favicon.ico
// and /api/v1/health are public so the shell can bootstrap.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.allowNonLoopback && !isLoopbackRequest(r) {
			http.Error(w, "remote requests are not allowed", http.StatusForbidden)
			return
		}
		if isPublic(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !a.checkToken(r) {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"agentflow-web\"")
			http.Error(w, "missing or invalid session token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPublic(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	switch r.URL.Path {
	case "/", "/index.html", "/api/v1/health":
		return r.Method == http.MethodGet
	case "/favicon.ico":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasPrefix(r.URL.Path, "/static/")
}

func (a *Auth) checkToken(r *http.Request) bool {
	if got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); got != "" {
		return constantTimeEqual(got, a.token)
	}
	if got := r.Header.Get("X-AgentFlow-Token"); got != "" {
		return constantTimeEqual(got, a.token)
	}
	if got := r.URL.Query().Get("token"); got != "" {
		return constantTimeEqual(got, a.token)
	}
	if cookie, err := r.Cookie("agentflow_session"); err == nil {
		return constantTimeEqual(cookie.Value, a.token)
	}
	return false
}

// isLoopbackRequest returns true when the remote peer addresses the
// loopback interface. It tolerates Forwarded headers being absent.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == ""
	}
	return ip.IsLoopback()
}

func generateToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
