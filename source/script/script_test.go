// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package script

import (
	"strings"
	"testing"
)

// The open-source build registers no provider, so scripting is off: --script is
// rejected and a DDL that defines code: hooks gets a warning rather than silent
// dead triggers.
func TestNoProviderDisablesScripting(t *testing.T) {
	if Available() {
		t.Skip("a scripting provider is registered")
	}

	err := RunFile(nil, "job.lua")
	if err == nil || !strings.Contains(err.Error(), "enterprise") {
		t.Fatalf("RunFile with no host: want an enterprise-build error, got %v", err)
	}

	// No hooks defined: no triggers, no warning.
	if warns, err := Attach(nil, "db.ddl", "logdir"); err != nil || len(warns) != 0 {
		t.Fatalf("Attach with no hooks: want (nil, nil), got (%v, %v)", warns, err)
	}
}

// CodeSet.Empty distinguishes a DDL with hooks from one without.
func TestCodeSetEmpty(t *testing.T) {
	if !(CodeSet{}).Empty() {
		t.Fatal("zero CodeSet should be empty")
	}
	cs := CodeSet{Table: map[string]map[string]string{"person": {"before_update": "check"}}}
	if cs.Empty() {
		t.Fatal("CodeSet with a table hook should not be empty")
	}
}
