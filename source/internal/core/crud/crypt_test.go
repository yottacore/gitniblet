// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const encPersonDDL = `
tables:
	table: person
		fields:
			field: name
				type: string
			field: ssn
				type: string
				encryption: always
			field: public_id
				type: string
				encryption: never
`

// fakeCipher is a reversible stand-in for the enterprise enc.Cipher. It lets the
// open-source package test the encryption ORCHESTRATION - which fields seal per
// policy, the seal-then-replay round-trip, the always-without-key refusal -
// without real cryptography, which lives in and is tested by the enterprise
// build. The token differs from the plaintext and reverses exactly; context is
// ignored (tamper/wrong-key behavior is the real cipher's concern).
type fakeCipher struct{}

func (fakeCipher) Seal(_ enc.Ctx, pt string) (string, error) {
	return "fake:" + base64.RawURLEncoding.EncodeToString([]byte(pt)), nil
}

func (fakeCipher) Open(_ enc.Ctx, tok string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(tok, "fake:"))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func encAPI(t *testing.T, cipher enc.Cipher, pref string) (*API, string, *ddl.Schema) {
	t.Helper()
	dir := t.TempDir()
	sch, err := ddl.Parse([]byte(encPersonDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(filepath.Join(dir, "log"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(st, lg)
	a.UserID = "u1"
	a.EnableEncryption(cipher, pref, sch)
	return a, filepath.Join(dir, "log"), sch
}

// rebuild opens a fresh view and replays the log through the decrypt pass with
// the given cipher (nil = no key), returning an API to read it back.
func rebuild(t *testing.T, logDir string, sch *ddl.Schema, cipher enc.Cipher) *API {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "v2.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	DecryptEntries(entries, cipher)
	if _, err := txlog.Apply(st, entries); err != nil {
		t.Fatal(err)
	}
	return New(st, lg)
}

// A key-armed write puts ciphertext in the log for auto/always fields and
// cleartext for never fields; a replay with the key restores everything, and a
// replay without the key leaves encrypted fields empty but keeps clear ones.
func TestEncryptWriteReplay(t *testing.T) {
	a, logDir, sch := encAPI(t, fakeCipher{}, "auto")
	id, err := a.Create("person", map[string]string{"name": "Ann", "ssn": "123-45-6789", "public_id": "P1"})
	if err != nil {
		t.Fatal(err)
	}

	// The log holds ciphertext for name (auto+key) and ssn (always), clear for public_id (never).
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	es, _, _ := lg.ReadAll()
	saw := map[string]txlog.Entry{}
	for _, e := range es {
		if e.Field != "" {
			saw[e.Field] = e
		}
	}
	if !saw["name"].Enc || saw["name"].NewValue == "Ann" {
		t.Fatalf("name should be encrypted in the log: %+v", saw["name"])
	}
	if !saw["ssn"].Enc || saw["ssn"].NewValue == "123-45-6789" {
		t.Fatalf("ssn (always) should be encrypted: %+v", saw["ssn"])
	}
	if saw["public_id"].Enc || saw["public_id"].NewValue != "P1" {
		t.Fatalf("public_id (never) should be cleartext: %+v", saw["public_id"])
	}

	// Replay WITH the key restores cleartext.
	withKey := rebuild(t, logDir, sch, fakeCipher{})
	row, _, _ := withKey.Get("person", id)
	if row["name"] != "Ann" || row["ssn"] != "123-45-6789" || row["public_id"] != "P1" {
		t.Fatalf("with key: %+v", row)
	}

	// Replay WITHOUT the key: encrypted fields are empty, clear ones survive.
	noKey := rebuild(t, logDir, sch, nil)
	row2, _, _ := noKey.Get("person", id)
	if row2["name"] != "" || row2["ssn"] != "" {
		t.Fatalf("no key should leave encrypted fields empty: %+v", row2)
	}
	if row2["public_id"] != "P1" {
		t.Fatalf("no key should still show clear fields: %+v", row2)
	}
}

// A field marked encryption: always cannot be written without a cipher.
func TestAlwaysRefusesWithoutKey(t *testing.T) {
	a, _, _ := encAPI(t, nil, "off")
	if _, err := a.Create("person", map[string]string{"name": "Ann", "ssn": "123"}); err == nil {
		t.Fatal("writing an always-encrypted field with no key must fail")
	}
	// A row touching only non-always fields still writes.
	if _, err := a.Create("person", map[string]string{"name": "Ann", "public_id": "P1"}); err != nil {
		t.Fatalf("non-always write should succeed: %v", err)
	}
}

// pref=off leaves everything (except always fields) cleartext even with a key.
func TestPrefOffStaysClear(t *testing.T) {
	a, logDir, _ := encAPI(t, fakeCipher{}, "off")
	if _, err := a.Create("person", map[string]string{"name": "Ann", "ssn": "S", "public_id": "P1"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := txlog.Open(logDir)
	es, _, _ := lg.ReadAll()
	for _, e := range es {
		if e.Field == "name" && e.Enc {
			t.Fatal("pref=off must not encrypt an auto field")
		}
		if e.Field == "ssn" && !e.Enc {
			t.Fatal("an always field stays encrypted even with pref=off")
		}
	}
}
