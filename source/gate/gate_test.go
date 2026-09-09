// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package gate_test

import (
	"testing"

	"github.com/jim-collier/nano-git-db/gate"
)

// fakeProvider is a stand-in for the external build's real provider.
type fakeProvider struct {
	status gate.Status
	calls  int
}

func (p *fakeProvider) Status() gate.Status {
	p.calls++
	return p.status
}

func TestNoProviderIsFull(t *testing.T) {
	// Default (open-source) build: nothing registered.
	if gate.Available() {
		t.Fatal("Available() should be false with no provider")
	}
	if s := gate.Evaluate(); s.Posture != gate.Full {
		t.Fatalf("Evaluate() posture = %d, want Full", s.Posture)
	}
}

func TestProviderIsAskedOncePerEvaluate(t *testing.T) {
	p := &fakeProvider{status: gate.Status{Posture: gate.Blocking, Message: "lapsed", WaitSeconds: 42}}
	gate.Register(p)
	defer gate.Register(nil) // leave the seam empty for other tests

	if !gate.Available() {
		t.Fatal("Available() should be true once a provider is registered")
	}
	s := gate.Evaluate()
	if s.Posture != gate.Blocking || s.Message != "lapsed" || s.WaitSeconds != 42 {
		t.Fatalf("Evaluate() = %+v, want the provider's status", s)
	}
	if p.calls != 1 {
		t.Fatalf("provider asked %d times, want exactly 1 per Evaluate", p.calls)
	}
}
