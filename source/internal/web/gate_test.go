// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jim-collier/nano-git-db/gate"
)

// frozen returns a clock stuck at t, so the gate's wait math is deterministic.
func frozen(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestGateBlocksThenWaitClears(t *testing.T) {
	srv, api := newTestServer(t)
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	srv.now = frozen(base)
	srv.applyGate(gate.Status{Posture: gate.Blocking, Message: "subscription lapsed", WaitSeconds: 30})
	h := srv.routes()

	// While gated, every normal page returns the start screen, not the app.
	rec := do(t, h, http.MethodGet, "/", nil)
	if body := rec.Body.String(); !strings.Contains(body, "subscription lapsed") || !strings.Contains(body, "read-only") {
		t.Fatalf("gate page missing its content: %q", body)
	}
	if rec := do(t, h, http.MethodGet, "/t/person", nil); !strings.Contains(rec.Body.String(), "read/write") {
		t.Fatal("a table request while gated should also return the gate page")
	}

	// Continue before the wait elapses is refused (still the gate page).
	if rec := do(t, h, http.MethodPost, "/gate/continue", nil); rec.Code == http.StatusSeeOther {
		t.Fatal("continue before the wait elapsed should not clear the gate")
	}
	if !srv.gated {
		t.Fatal("still gated after an early continue")
	}

	// After the wait, continue clears into full read/write.
	srv.now = frozen(base.Add(31 * time.Second))
	rec = do(t, h, http.MethodPost, "/gate/continue", nil)
	if rec.Code != http.StatusSeeOther || srv.gated {
		t.Fatalf("continue after wait: code=%d gated=%v", rec.Code, srv.gated)
	}
	if api.ReadOnly() {
		t.Fatal("waiting out the gate must leave read/write intact")
	}
}

func TestGateDismissGoesReadOnly(t *testing.T) {
	srv, api := newTestServer(t)
	srv.now = frozen(time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	srv.applyGate(gate.Status{Posture: gate.Blocking, Message: "subscription lapsed", WaitSeconds: 30})
	h := srv.routes()

	rec := do(t, h, http.MethodPost, "/gate/dismiss", nil)
	if rec.Code != http.StatusSeeOther || srv.gated {
		t.Fatalf("dismiss: code=%d gated=%v", rec.Code, srv.gated)
	}
	if !api.ReadOnly() {
		t.Fatal("dismiss must open the session read-only")
	}
	// The app is reachable again, now carrying a read-only banner.
	if rec := do(t, h, http.MethodGet, "/", nil); !strings.Contains(rec.Body.String(), "read-only") {
		t.Fatalf("expected a read-only banner after dismiss: %q", rec.Body.String())
	}
}

func TestGateNagBannerNoBlock(t *testing.T) {
	srv, api := newTestServer(t)
	srv.applyGate(gate.Status{Posture: gate.Nag, Message: "renews soon"})
	if srv.gated {
		t.Fatal("Nag must not gate the UI")
	}
	if api.ReadOnly() {
		t.Fatal("Nag must not go read-only")
	}
	rec := do(t, srv.routes(), http.MethodGet, "/", nil)
	if !strings.Contains(rec.Body.String(), "renews soon") {
		t.Fatalf("Nag banner missing: %q", rec.Body.String())
	}
}
