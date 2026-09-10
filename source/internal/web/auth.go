// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Web login. Two shapes, chosen by the explicit web_mode setting (never guessed):
//
//   - local (default): a single-user machine. The user is identified with no
//     password - the git repo account, else the OS user - and every request acts
//     as them. A safety-net refuses to serve if a proxy header ever appears, so a
//     box accidentally exposed behind a proxy can never run passwordless.
//   - proxied: behind a reverse proxy. Every request needs a session; without one
//     it is bounced to a username/password login checked against the local creds
//     file. The logged-in user then drives the data layer, so the existing user
//     and group permissions apply to the web view.
//
// Stronger methods (2FA, passkeys, SSO) are an enterprise concern; this is the
// open-source baseline that makes the server safe to put behind a proxy at all.
package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const sessionCookie = "ngdb_session"
const sessionTTL = 12 * time.Hour

// authState carries the resolved login configuration for the running server.
type authState struct {
	proxied   bool             // proxied mode requires a login; local does not
	creds     *config.WebCreds // proxied credential store
	sessions  *sessions        // proxied session table
	limit     *throttle        // failed-login counters (throttle.go)
	localUser string           // the fixed identity in local mode
}

// newAuth resolves the login configuration once at startup. In local mode it
// also stamps the API with the resolved single user; in proxied mode the acting
// user is set per request by authGuard instead.
func newAuth(settings *config.Settings, logDir string, api *crud.API) *authState {
	au := &authState{
		proxied:  settings.WebModeProxied(),
		sessions: newSessions(time.Now),
		limit:    newThrottle(time.Now),
	}
	if au.proxied {
		au.creds = config.LoadWebCreds()
	} else {
		au.localUser = localUser(logDir)
		api.UserID = au.localUser
	}
	return au
}

// localUser identifies the machine's user without a password: the git account of
// the log dir's repo, else the default (NANOGITDB_USER or the OS user).
func localUser(logDir string) string {
	if account := txlog.RepoAccount(logDir); account != "" {
		return account
	}
	return crud.DefaultUserID()
}

// authGuard is the outermost middleware: it decides who may reach anything.
func (s *server) authGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || !s.auth.proxied {
			// Local mode is passwordless, so a proxy header means the box is
			// probably exposed - refuse rather than serve every request as the
			// local user to the world. (A nil auth means no login layer was wired,
			// e.g. in a unit test - just pass through.)
			if s.auth != nil {
				if hdr := proxyHeader(r); hdr != "" {
					http.Error(w, "refusing to serve: web_mode is local (passwordless) but this request arrived through a proxy ("+hdr+"). Set web_mode = proxied and add a login, or remove the proxy.", http.StatusServiceUnavailable)
					return
				}
			}
			next.ServeHTTP(w, r) // api.UserID was fixed to the local user at startup
			return
		}
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		user, ok := s.auth.sessions.lookup(sessionID(r))
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// Act as the logged-in user for the data layer. The lock serializes the
		// shared api.UserID across concurrent requests; this is a local UI lightly
		// extended to multi-user, not a high-throughput service.
		s.umu.Lock()
		defer s.umu.Unlock()
		s.api.UserID = user
		next.ServeHTTP(w, r)
	})
}

// publicPath is reachable in proxied mode without a session: the login endpoints
// and the static assets the login page itself needs.
func publicPath(path string) bool {
	return path == "/login" || path == "/logout" || strings.HasPrefix(path, "/assets/")
}

// proxyHeader returns the name of the first reverse-proxy header present, or "".
func proxyHeader(r *http.Request) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-Ip", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return h
		}
	}
	return ""
}

func (s *server) loginPage(w http.ResponseWriter, r *http.Request) {
	if !s.auth.proxied { // no login wall in local mode
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, ok := s.auth.sessions.lookup(sessionID(r)); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, "")
}

func (s *server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.auth.proxied {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, addr := r.PostForm.Get("username"), clientAddr(r)
	if s.auth.limit.blocked(user, addr) {
		w.WriteHeader(http.StatusTooManyRequests)
		s.renderLogin(w, "Too many attempts. Wait a minute and try again.")
		return
	}
	release := s.auth.limit.enter() // rations the expensive part
	ok := user != "" && s.auth.creds.Verify(user, r.PostForm.Get("password"))
	release()
	if !ok {
		s.auth.limit.fail(user, addr)
		w.WriteHeader(http.StatusUnauthorized)
		s.renderLogin(w, "Wrong username or password.")
		return
	}
	s.auth.limit.pass(user, addr)
	id, err := s.auth.sessions.create(user)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   int(sessionTTL / time.Second),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.sessions.delete(sessionID(r))
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *server) renderLogin(w http.ResponseWriter, errMsg string) {
	s.render(w, "login.html", map[string]any{"Error": errMsg})
}

// sessionID reads the session cookie value, or "" when absent.
func sessionID(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// isHTTPS reports whether the browser reached us over TLS - directly, or through
// a proxy that terminated it (so the Secure cookie flag is safe to set).
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
