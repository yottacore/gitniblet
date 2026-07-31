// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package crud

import (
	"bytes"
	"testing"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/guid"
)

// now() must be strictly increasing even under rapid calls or a wall clock
// stepping backwards - replay order depends on it.
func TestNowIsStrictlyMonotonic(t *testing.T) {
	a, _, _ := newAPI(t)
	prev := a.now()
	for i := 0; i < 1000; i++ {
		cur := a.now()
		if cur <= prev {
			t.Fatalf("timestamp went backwards: %s then %s", prev, cur)
		}
		prev = cur
	}
}

func TestNowSurvivesClockStepBack(t *testing.T) {
	a, _, _ := newAPI(t)
	future := time.Now().UTC().Add(time.Hour).Format(tsLayout)
	a.lastDate = future
	got := a.now()
	if got <= future {
		t.Fatalf("now() = %s, want > pinned %s", got, future)
	}
}

// The raw bytes are what SQLite holds as the BLOB primary key, so they are what
// has to climb to keep inserts at the b-tree tail. The text form deliberately
// does not sort in time order - base64url is not ASCII-ordered - so decode
// before comparing.
func TestNewIDIsTimeOrdered(t *testing.T) {
	decode := func(id string) []byte {
		t.Helper()
		raw, err := guid.Decode(id)
		if err != nil {
			t.Fatalf("newID produced an undecodable id %q: %v", id, err)
		}
		return raw
	}
	prev := decode(newID())
	for i := 0; i < 100; i++ {
		cur := decode(newID())
		if bytes.Compare(cur, prev) < 0 {
			// v7 ids embed a ms timestamp + monotonic counter; within one
			// process they must not go backwards.
			t.Fatalf("id went backwards: %x then %x", prev, cur)
		}
		prev = cur
	}
}
