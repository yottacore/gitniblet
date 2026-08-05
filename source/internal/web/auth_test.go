// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// proxiedServer returns a server in proxied mode with one known login.
func proxiedServer(t *testing.T) *server {
	t.Helper()
	s, _ := newTestServer(t)
	creds := &config.WebCreds{Users: map[string]string{}}
	if err := creds.Set("alice", "s3cret"); err != nil {
		t.Fatal(err)
	}
	s.auth = &authState{proxied: true, creds: creds, sessions: newSessions(time.Now), limit: newThrottle(time.Now)}
	return s
}

func TestLocalRefusesProxyHeader(t *testing.T) {
	s, _ := newTestServer(t)
	s.auth = &authState{proxied: false, localUser: "someone"}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("proxy-header request: code = %d, want 503", w.Code)
	}
}

func TestLocalServesWithoutProxyHeader(t *testing.T) {
	s, _ := newTestServer(t)
	s.auth = &authState{proxied: false, localUser: "someone"}
	w := do(t, s.routes(), "GET", "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("plain local request: code = %d, want 200", w.Code)
	}
}

func TestProxiedRedirectsWhenNoSession(t *testing.T) {
	s := proxiedServer(t)
	w := do(t, s.routes(), "GET", "/", nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("no-session request: code = %d loc = %q, want 303 /login", w.Code, w.Header().Get("Location"))
	}
}

func TestLoginPageAndAssetsPublic(t *testing.T) {
	s := proxiedServer(t)
	if w := do(t, s.routes(), "GET", "/login", nil); w.Code != http.StatusOK {
		t.Fatalf("GET /login: %d", w.Code)
	}
	if w := do(t, s.routes(), "GET", "/assets/style.css", nil); w.Code != http.StatusOK {
		t.Fatalf("GET /assets/style.css: %d", w.Code)
	}
}

func TestProxiedBadLogin(t *testing.T) {
	s := proxiedServer(t)
	w := do(t, s.routes(), "POST", "/login", url.Values{"username": {"alice"}, "password": {"wrong"}})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: code = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Wrong username or password") {
		t.Fatalf("bad login: missing error message: %s", w.Body)
	}
}

func TestProxiedLoginThenReachAsUser(t *testing.T) {
	s, api := newTestServer(t)
	creds := &config.WebCreds{Users: map[string]string{}}
	if err := creds.Set("alice", "s3cret"); err != nil {
		t.Fatal(err)
	}
	s.auth = &authState{proxied: true, creds: creds, sessions: newSessions(time.Now), limit: newThrottle(time.Now)}

	// Good login -> 303 + a session cookie.
	w := do(t, s.routes(), "POST", "/login", url.Values{"username": {"alice"}, "password": {"s3cret"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("good login: code = %d, want 303 (%s)", w.Code, w.Body)
	}
	cookie := sessionSetCookie(t, w)

	// The cookie reaches the app, and the request acts as alice.
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed request: code = %d, want 200", rec.Code)
	}
	if api.UserID != "alice" {
		t.Fatalf("acting user = %q, want alice", api.UserID)
	}
	if !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("page does not show the signed-in user: %s", rec.Body)
	}

	// Logout drops the session; the same cookie no longer authenticates.
	lreq := httptest.NewRequest("POST", "/logout", nil)
	lreq.AddCookie(cookie)
	lrec := httptest.NewRecorder()
	s.routes().ServeHTTP(lrec, lreq)
	if lrec.Code != http.StatusSeeOther {
		t.Fatalf("logout: code = %d, want 303", lrec.Code)
	}
	greq := httptest.NewRequest("GET", "/", nil)
	greq.AddCookie(cookie)
	grec := httptest.NewRecorder()
	s.routes().ServeHTTP(grec, greq)
	if grec.Code != http.StatusSeeOther {
		t.Fatalf("after logout: code = %d, want 303 redirect to login", grec.Code)
	}
}

// sessionSetCookie extracts the session cookie the login response set.
func sessionSetCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}
