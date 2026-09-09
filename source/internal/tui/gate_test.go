// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/jim-collier/nano-git-db/gate"
)

func TestDecideGateFullAndNag(t *testing.T) {
	if d := decideGate(gate.Status{Posture: gate.Full}, nil, time.Now); !d.proceed || d.readOnly || d.banner != "" {
		t.Fatalf("full = %+v", d)
	}
	if d := decideGate(gate.Status{Posture: gate.Nag, Message: "renews soon"}, nil, time.Now); !d.proceed || d.readOnly || d.banner != "renews soon" {
		t.Fatalf("nag = %+v", d)
	}
}

// blockingScreen runs a Blocking gate on a simulation screen and returns the
// screen, a decision channel, and a waitFor helper.
func blockingScreen(t *testing.T, st gate.Status, now func() time.Time) (tcell.SimulationScreen, chan gateDecision, func(string)) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	out := make(chan gateDecision, 1)
	go func() { out <- runBlockingGate(st, screen, now) }()
	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%q never appeared:\n%s", sub, screenText(screen))
	}
	return screen, out, waitFor
}

func TestBlockingGateDismissReadOnly(t *testing.T) {
	st := gate.Status{Posture: gate.Blocking, Message: "subscription lapsed", WaitSeconds: 60}
	screen, out, waitFor := blockingScreen(t, st, func() time.Time { return time.Unix(1000, 0) })
	waitFor("subscription lapsed")
	screen.InjectKey(tcell.KeyRune, 'd', tcell.ModNone)

	dec := <-out
	if !dec.proceed || !dec.readOnly || !strings.Contains(dec.banner, "read-only") {
		t.Fatalf("dismiss = %+v", dec)
	}
}

func TestBlockingGateWaitThenEnter(t *testing.T) {
	var mu sync.Mutex
	cur := time.Unix(1000, 0)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }

	st := gate.Status{Posture: gate.Blocking, Message: "lapsed", WaitSeconds: 30}
	screen, out, waitFor := blockingScreen(t, st, clock)
	waitFor("wait 30s")

	// Enter before the wait elapses is ignored - still on the gate screen.
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	select {
	case dec := <-out:
		t.Fatalf("gate cleared before the wait: %+v", dec)
	case <-time.After(150 * time.Millisecond):
	}

	// Advance past the wait; Enter now clears into full read/write.
	mu.Lock()
	cur = cur.Add(31 * time.Second)
	mu.Unlock()
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)

	dec := <-out
	if !dec.proceed || dec.readOnly {
		t.Fatalf("wait-then-enter = %+v", dec)
	}
}
