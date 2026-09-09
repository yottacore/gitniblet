// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/gate"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
)

type fakeGate struct{ st gate.Status }

func (f fakeGate) Status() gate.Status { return f.st }

func TestApplyGateBlockingGoesReadOnly(t *testing.T) {
	gate.Register(fakeGate{gate.Status{Posture: gate.Blocking, Message: "lapsed"}})
	defer gate.Register(nil)

	api := crud.New(nil, nil)
	var buf bytes.Buffer
	applyGate(api, &buf)

	if !api.ReadOnly() {
		t.Fatal("Blocking should set the session read-only")
	}
	if out := buf.String(); !strings.Contains(out, "lapsed") || !strings.Contains(out, "read-only") {
		t.Fatalf("notice missing: %q", out)
	}
}

func TestApplyGateNagStaysWritable(t *testing.T) {
	gate.Register(fakeGate{gate.Status{Posture: gate.Nag, Message: "renews soon"}})
	defer gate.Register(nil)

	api := crud.New(nil, nil)
	var buf bytes.Buffer
	applyGate(api, &buf)

	if api.ReadOnly() {
		t.Fatal("Nag must not go read-only")
	}
	if !strings.Contains(buf.String(), "renews soon") {
		t.Fatalf("nag notice missing: %q", buf.String())
	}
}

func TestApplyGateFullSilent(t *testing.T) {
	api := crud.New(nil, nil) // no provider registered -> Full
	var buf bytes.Buffer
	applyGate(api, &buf)

	if api.ReadOnly() || buf.Len() != 0 {
		t.Fatalf("Full should be silent read/write: ro=%v out=%q", api.ReadOnly(), buf.String())
	}
}
