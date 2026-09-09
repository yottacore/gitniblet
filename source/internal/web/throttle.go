// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Login throttling. Checking a password is meant to be expensive - the stored
// hash costs hundreds of thousands of rounds - which makes an open login
// endpoint a way to spend the machine's CPU for free. Two limits, because they
// cover different halves of the problem:
//
//   - a failure count, per username and per source address, refuses a run of
//     wrong guesses before any hashing happens. It bounds the sustained rate,
//     but not a burst: attempts arriving together all pass the count before any
//     of them has failed yet.
//   - a cap on how many passwords are checked at once bounds that burst, so no
//     number of simultaneous attempts can occupy more than a couple of cores.
//
// Neither locks an account out. The counts age out on their own, so a user who
// fumbles a password waits a minute rather than needing someone to reset them.
package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// loginWindow is how long a failure keeps counting against a key.
	loginWindow = time.Minute
	// loginMaxUser is the failures per username before attempts are refused.
	loginMaxUser = 10
	// loginMaxAddr is the same per source address. It is deliberately looser: a
	// reverse proxy puts every user behind one address, so this limit has to
	// stay clear of ordinary shared use and only catch a flood.
	loginMaxAddr = 30
	// loginAtOnce is how many passwords may be checked concurrently.
	loginAtOnce = 2
)

// throttle counts recent login failures per key and rations the password checks.
type throttle struct {
	mu    sync.Mutex
	fails map[string][]time.Time
	slots chan struct{}
	now   func() time.Time
}

func newThrottle(now func() time.Time) *throttle {
	return &throttle{
		fails: map[string][]time.Time{},
		slots: make(chan struct{}, loginAtOnce),
		now:   now,
	}
}

// blocked reports whether either key is over its limit. Both keys are counted
// whether or not the username exists, so this cannot be used to tell a real
// account from an invented one.
func (t *throttle) blocked(user, addr string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.over("u:"+user, loginMaxUser) || t.over("a:"+addr, loginMaxAddr)
}

// over drops the failures that have aged out of the window and reports whether
// what is left reaches the limit. The pruning is also what keeps the table
// bounded - every key a caller touches is either trimmed or dropped. Caller
// holds the lock.
func (t *throttle) over(key string, limit int) bool {
	cutoff := t.now().Add(-loginWindow)
	kept := t.fails[key][:0]
	for _, at := range t.fails[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.fails, key)
		return false
	}
	t.fails[key] = kept
	return len(kept) >= limit
}

// fail records a wrong guess against both keys.
func (t *throttle) fail(user, addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	at := t.now()
	t.fails["u:"+user] = append(t.fails["u:"+user], at)
	t.fails["a:"+addr] = append(t.fails["a:"+addr], at)
}

// pass forgets both keys after a login succeeds.
func (t *throttle) pass(user, addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, "u:"+user)
	delete(t.fails, "a:"+addr)
}

// enter reserves one of the concurrent password checks and returns its release.
// It waits rather than refusing, because only attempts that already got past
// the failure counts ever reach it, so the queue stays short.
func (t *throttle) enter() func() {
	t.slots <- struct{}{}
	return func() { <-t.slots }
}

// clientAddr is the source a failure counts against: the socket's own address,
// never a forwarded header. A header is whatever the client says it is, so
// keying on one would hand an attacker a fresh budget with every request.
func clientAddr(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
