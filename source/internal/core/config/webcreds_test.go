// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import (
	"testing"
	"time"
)

func TestWebCredsHashVerify(t *testing.T) {
	c := &WebCreds{}
	if err := c.Set("alice", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if !c.Verify("alice", "correct horse") {
		t.Fatal("right password did not verify")
	}
	if c.Verify("alice", "wrong") {
		t.Fatal("wrong password verified")
	}
	if c.Verify("nobody", "correct horse") {
		t.Fatal("unknown user verified")
	}
	// The stored value is a hash, not the password.
	if c.Users["alice"] == "correct horse" {
		t.Fatal("password stored in the clear")
	}
}

func TestWebCredsSaveLoad(t *testing.T) {
	SetBaseOverride(t.TempDir())
	t.Cleanup(func() { SetBaseOverride("") })

	c := &WebCreds{}
	if err := c.Set("bob", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := LoadWebCreds()
	if !got.Verify("bob", "hunter2") {
		t.Fatal("reloaded creds do not verify")
	}
	if got.Verify("bob", "nope") {
		t.Fatal("reloaded creds verified a wrong password")
	}
}

// Usernames ride in the file as instance discriminators, not as field names,
// because SHCL folds field-name case. Spelling them as fields would quietly
// merge "Alice" into "alice" and hand one account the other's password. The
// awkward names cover the other two ways a name meets the grammar: one that
// needs quoting, and one a bare selector would read as an index.
func TestWebCredsAwkwardNamesSurviveSaveLoad(t *testing.T) {
	SetBaseOverride(t.TempDir())
	t.Cleanup(func() { SetBaseOverride("") })

	logins := map[string]string{
		"alice":     "lower",
		"Alice":     "upper",
		"bob smith": "spaced",
		"123":       "numeric",
	}
	c := &WebCreds{}
	for user, password := range logins {
		if err := c.Set(user, password); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	got := LoadWebCreds()
	if len(got.Users) != len(logins) {
		t.Fatalf("reloaded %d users, want %d", len(got.Users), len(logins))
	}
	for user, password := range logins {
		if !got.Verify(user, password) {
			t.Errorf("user %q did not verify its own password", user)
		}
	}
	// The case pair is the whole point: each must reject the other's password.
	if got.Verify("alice", "upper") || got.Verify("Alice", "lower") {
		t.Fatal("case-different usernames collapsed into one account")
	}
}

func TestWebModeProxied(t *testing.T) {
	if (&Settings{WebMode: "proxied"}).WebModeProxied() != true {
		t.Fatal(`web_mode "proxied" should be proxied`)
	}
	for _, m := range []string{"", "local", "Proxied", "typo"} {
		if (&Settings{WebMode: m}).WebModeProxied() {
			t.Fatalf("web_mode %q should fall back to local", m)
		}
	}
}

// An unknown user has no hash to compare, but it still has to cost the same as a
// wrong password - otherwise the response time says which accounts exist.
func TestUnknownUserStillCostsAKDFRun(t *testing.T) {
	creds := &WebCreds{}
	if err := creds.Set("alice", "correct horse"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if creds.Verify("alice", "wrong") {
		t.Fatal("a wrong password verified")
	}
	known := time.Since(start)

	start = time.Now()
	if creds.Verify("nosuchuser", "wrong") {
		t.Fatal("an unknown user verified")
	}
	unknown := time.Since(start)

	// A skipped KDF returns in microseconds against ~100ms of real work, so any
	// generous fraction separates the two without being timing-flaky.
	if unknown < known/4 {
		t.Fatalf("unknown user took %v against %v for a known one - the KDF was skipped", unknown, known)
	}
}
