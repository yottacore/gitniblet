// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// sessions is the in-memory table of proxied logins. It is intentionally not
// persisted: a server restart just requires everyone to log in again, which for a
// small self-hosted deployment is fine and avoids a session file on disk.
type sessions struct {
	mu  sync.Mutex
	m   map[string]session
	now func() time.Time // injectable for tests
}

type session struct {
	user    string
	expires time.Time
}

func newSessions(now func() time.Time) *sessions {
	return &sessions{m: map[string]session{}, now: now}
}

// create mints a random session id bound to user, valid for sessionTTL.
func (s *sessions) create(user string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = session{user: user, expires: s.now().Add(sessionTTL)}
	return id, nil
}

// lookup returns the session's user if the id is known and unexpired. An expired
// entry is dropped in passing, so the table self-prunes as ids are used.
func (s *sessions) lookup(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[id]
	if !ok {
		return "", false
	}
	if !s.now().Before(sess.expires) {
		delete(s.m, id)
		return "", false
	}
	return sess.user, true
}

func (s *sessions) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
}
