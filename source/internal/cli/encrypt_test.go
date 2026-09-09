// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

const encInitDDL = `database:
	tables:
		table: person
			fields:
				field: name
					type: string
				field: ssn
					type: string
					encryption: always
`

// The open-source build cannot mint keys: --init --encrypt=on fails cleanly
// (encryption is an enterprise feature) rather than pretending to set one up.
// The positive path - minting, encrypting, reading back - is exercised by the
// enterprise build's end-to-end test, which registers a real provider.
func TestOSSInitEncryptRefused(t *testing.T) {
	base := t.TempDir()
	config.SetBaseOverride(base)
	config.SetEncryptPref("on")
	t.Cleanup(func() { config.SetBaseOverride(""); config.SetEncryptPref("") })

	work := t.TempDir()
	ddlPath := filepath.Join(work, "people.ddl")
	if err := os.WriteFile(ddlPath, []byte(encInitDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	err := Init(nil)
	if err == nil || !strings.Contains(err.Error(), "enterprise") {
		t.Fatalf("--encrypt=on must be refused in the open-source build; got %v", err)
	}
}

// An `encryption: always` field cannot be written by the open-source build - it
// has no cipher, so it must refuse rather than log the value in the clear.
// Rows touching only non-always fields still write normally.
func TestOSSAlwaysFieldRefused(t *testing.T) {
	base := t.TempDir()
	config.SetBaseOverride(base)
	t.Cleanup(func() { config.SetBaseOverride("") })

	work := t.TempDir()
	ddlPath := filepath.Join(work, "people.ddl")
	if err := os.WriteFile(ddlPath, []byte(encInitDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	capture(t, func() error { return Init(nil) }) // plain registration, no encryption (name = "people")

	// The always-encrypted ssn cannot be written without encryption support.
	if err := Run([]string{"create", "people", "person", "name=Ann", "ssn=secret"}); err == nil ||
		!strings.Contains(err.Error(), "encryption") {
		t.Fatalf("writing an always-encrypted field must fail in the open-source build; got %v", err)
	}
	// A row with only the non-always field still writes.
	id := strings.TrimSpace(capture(t, func() error {
		return Run([]string{"create", "people", "person", "name=Ann"})
	}))
	if id == "" {
		t.Fatal("a non-always-field create should succeed")
	}
}
