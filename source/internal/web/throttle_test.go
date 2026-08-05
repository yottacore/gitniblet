// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a hand-advanced clock, so the window can be tested without
// waiting for it.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func TestThrottleRefusesAfterTooManyFailures(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	limit := newThrottle(clock.now)

	for i := 0; i < loginMaxUser-1; i++ {
		limit.fail("alice", "10.0.0.1")
		if limit.blocked("alice", "10.0.0.1") {
			t.Fatalf("blocked after %d failures, want %d", i+1, loginMaxUser)
		}
	}
	limit.fail("alice", "10.0.0.1")
	if !limit.blocked("alice", "10.0.0.1") {
		t.Fatalf("still allowed after %d failures", loginMaxUser)
	}
	// A different username from the same address is still under its own limit,
	// and well under the looser address limit.
	if limit.blocked("bob", "10.0.0.1") {
		t.Error("one username's failures blocked another")
	}
}

func TestThrottleForgetsFailuresAfterTheWindow(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	limit := newThrottle(clock.now)

	for i := 0; i < loginMaxUser; i++ {
		limit.fail("alice", "10.0.0.1")
	}
	if !limit.blocked("alice", "10.0.0.1") {
		t.Fatal("want blocked")
	}
	clock.advance(loginWindow + time.Second)
	if limit.blocked("alice", "10.0.0.1") {
		t.Error("failures should age out; an account must not stay locked")
	}
	if n := len(limit.fails); n != 0 {
		t.Errorf("kept %d aged-out keys, want the table pruned", n)
	}
}

func TestThrottleClearsOnASuccessfulLogin(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	limit := newThrottle(clock.now)

	for i := 0; i < loginMaxUser; i++ {
		limit.fail("alice", "10.0.0.1")
	}
	limit.pass("alice", "10.0.0.1")
	if limit.blocked("alice", "10.0.0.1") {
		t.Error("a good password should clear the count")
	}
}

// The address limit is what catches a flood spread over many usernames, which
// the per-username count alone would let through.
func TestThrottleRefusesAFloodAcrossUsernames(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	limit := newThrottle(clock.now)

	for i := 0; i < loginMaxAddr; i++ {
		limit.fail(string(rune('a'+i%26))+"user", "10.0.0.9")
	}
	if !limit.blocked("brand-new-name", "10.0.0.9") {
		t.Error("a flood from one address should be refused whatever name it uses")
	}
}

// The counters bound the sustained rate but not a burst - simultaneous attempts
// all pass the count before any has failed. This is the limit that stops a
// burst from occupying every core.
func TestThrottleRationsConcurrentChecks(t *testing.T) {
	limit := newThrottle(time.Now)

	releases := make([]func(), 0, loginAtOnce)
	for i := 0; i < loginAtOnce; i++ {
		releases = append(releases, limit.enter())
	}
	got := make(chan struct{})
	go func() {
		release := limit.enter()
		close(got)
		release()
	}()
	select {
	case <-got:
		t.Fatalf("a %dst check started while %d were running", loginAtOnce+1, loginAtOnce)
	case <-time.After(50 * time.Millisecond):
	}
	releases[0]()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("a freed slot was never taken up")
	}
	for _, release := range releases[1:] {
		release()
	}
}

// End to end through the handler: the run of wrong guesses is refused with 429
// rather than being hashed, and a good password still gets in afterwards.
func TestLoginEndpointRefusesARunOfWrongGuesses(t *testing.T) {
	srv := proxiedServer(t) // knows alice / s3cret

	post := func(password string) *httptest.ResponseRecorder {
		form := url.Values{"username": {"alice"}, "password": {password}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "10.0.0.5:1234"
		rec := httptest.NewRecorder()
		srv.loginSubmit(rec, req)
		return rec
	}

	for i := 0; i < loginMaxUser; i++ {
		if code := post("wrong").Code; code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: code = %d, want %d", i+1, code, http.StatusUnauthorized)
		}
	}
	rec := post("wrong")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d, want %d once the limit is reached", rec.Code, http.StatusTooManyRequests)
	}
	// The right password is refused too while the limit holds - the point is to
	// stop spending CPU, and a rate this high is not a user typing.
	if code := post("s3cret").Code; code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want the limit to apply to every attempt", code)
	}
}
