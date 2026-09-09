// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The web UI is reachable from any page the user has open in the same browser,
// so a write must prove it came from the UI itself, and a read must arrive under
// the name the server was bound to.

func TestCrossSiteWriteIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.routes()

	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{"foreign origin", map[string]string{"Origin": "https://evil.example"}},
		{"cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"same-site subdomain", map[string]string{"Sec-Fetch-Site": "same-site"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader("name=pwned")
			req := httptest.NewRequest(http.MethodPost, "/t/person", body)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("code = %d, want 403", rec.Code)
			}
		})
	}

	// The row must not exist: the guard has to run before the handler.
	rec := do(t, h, http.MethodGet, "/t/person", nil)
	if strings.Contains(rec.Body.String(), "pwned") {
		t.Fatal("a refused cross-site post still wrote a row")
	}
}

func TestSameOriginWriteIsAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.routes()

	body := strings.NewReader("name=alice")
	req := httptest.NewRequest(http.MethodPost, "/t/person", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://"+req.Host)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "alice") {
		t.Fatal("the row is missing from the refreshed grid")
	}
}

// A page can point a name it owns at 127.0.0.1 and read the whole database over
// it, so local mode only answers under the address it was bound to.
func TestForeignHostIsRefusedWhenBound(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.hostPin = "127.0.0.1:8765"
	h := srv.routes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "rebind.example:8765"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Host: code = %d, want 403", rec.Code)
	}

	for _, host := range []string{"127.0.0.1:8765", "localhost:8765"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Host %q: code = %d, want 200", host, rec.Code)
		}
	}
}

// Behind a reverse proxy the Host is legitimately rewritten, so the pin does not
// apply there; the cross-site write check still does.
func TestProxiedModeIgnoresTheHostPin(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.hostPin = "127.0.0.1:8765"
	srv.auth = &authState{proxied: true, sessions: newSessions(time.Now), limit: newThrottle(time.Now)}
	h := srv.routes()

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Host = "db.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxied Host: code = %d, want 200", rec.Code)
	}
}

// The gate routes stay live after the notice clears, so an ungated server must
// refuse them - otherwise any page could turn a running server read-only.
func TestGateRoutesRefusedWhenNotGated(t *testing.T) {
	srv, api := newTestServer(t)
	h := srv.routes()

	for _, path := range []string{"/gate/dismiss", "/gate/continue"} {
		rec := do(t, h, http.MethodPost, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: code = %d, want 404", path, rec.Code)
		}
	}
	if api.ReadOnly() {
		t.Fatal("a refused dismiss must not degrade the session to read-only")
	}
}
