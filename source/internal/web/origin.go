// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// originGuard refuses requests that did not come from this UI itself.
//
// Binding to loopback keeps other machines out, but not other pages on this
// one: a form POST from any site the user has open is a plain cross-origin
// request the browser sends without asking, and a page that points a hostname
// it controls at 127.0.0.1 can read the database over that name. So writes must
// carry a same-origin marker, and in local mode the Host header has to be the
// address the server was actually told to serve on.
//
// Proxied mode is exempt from the Host pin - a reverse proxy legitimately
// rewrites it - and keeps the write check, which the session cookie's
// SameSite=Lax already backs up.
func (s *server) originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied := s.auth != nil && s.auth.proxied
		if !proxied && !s.hostAllowed(r.Host) {
			http.Error(w, "unexpected Host header", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed checks the Host header against the listen address. An empty pin
// means nothing was bound (tests, or an embedder driving the handler directly),
// so the check is skipped rather than guessed at.
func (s *server) hostAllowed(host string) bool {
	if s.hostPin == "" {
		return true
	}
	if host == s.hostPin {
		return true
	}
	// The same port reached by name rather than by address.
	pinHost, pinPort, err := net.SplitHostPort(s.hostPin)
	if err != nil {
		return false
	}
	gotHost, gotPort, err := net.SplitHostPort(host)
	if err != nil || gotPort != pinPort {
		return false
	}
	if !isLoopback(pinHost) {
		return false
	}
	return isLoopback(gotHost) || strings.EqualFold(gotHost, "localhost")
}

func isLoopback(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin reports whether a state-changing request came from this UI. A
// browser sends Origin on cross-site form posts and Sec-Fetch-Site on every
// request; a bare curl sends neither, which is allowed - the point is to stop
// another page driving the UI, not to lock out the shell.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none", "":
	default:
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
