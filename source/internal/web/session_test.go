// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	clock := base
	ss := newSessions(func() time.Time { return clock })

	id, err := ss.create("bob")
	if err != nil {
		t.Fatal(err)
	}
	if user, ok := ss.lookup(id); !ok || user != "bob" {
		t.Fatalf("lookup right after create = (%q,%v), want (bob,true)", user, ok)
	}

	// Just before the TTL: still valid. At/after it: gone (and pruned).
	clock = base.Add(sessionTTL - time.Second)
	if _, ok := ss.lookup(id); !ok {
		t.Fatal("session expired one second early")
	}
	clock = base.Add(sessionTTL)
	if _, ok := ss.lookup(id); ok {
		t.Fatal("session outlived its TTL")
	}

	// A fresh session, then an explicit delete.
	clock = base
	id2, _ := ss.create("carol")
	ss.delete(id2)
	if _, ok := ss.lookup(id2); ok {
		t.Fatal("session survived delete")
	}
	if _, ok := ss.lookup(""); ok {
		t.Fatal("empty id resolved to a session")
	}
}
