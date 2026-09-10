// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package store

import (
	"path/filepath"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/guid"
)

// refStore builds a table with a ref column and one row pointing at itself.
func refStore(t *testing.T) (*Store, []byte) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sch := &ddl.Schema{Tables: []ddl.Table{{
		Name:   "task",
		Fields: []ddl.Field{{Name: "title", Type: "string"}, {Name: "parent", Type: "ref"}},
	}}}
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, guid.RawLen)
	copy(raw, "0123456789abcdef")
	if _, err := st.DB().Exec(
		`INSERT INTO "task" ("id","title","parent") VALUES (?,?,?)`, raw, "root", raw); err != nil {
		t.Fatal(err)
	}
	return st, raw
}

// The point of id(): a hand-written query matches a ref column by id text.
func TestIDFuncMatchesRefColumn(t *testing.T) {
	st, raw := refStore(t)
	var title string
	if err := st.DB().QueryRow(
		`SELECT "title" FROM "task" WHERE "parent"=id(?)`, guid.Encode(raw)).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "root" {
		t.Fatalf("title = %q, want root", title)
	}
}

// Without id() the same query matches nothing and reports no error - the
// failure this function exists to prevent.
func TestTextComparisonMatchesNothing(t *testing.T) {
	st, raw := refStore(t)
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM "task" WHERE "parent"=?`, guid.Encode(raw)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("text compare matched %d rows; the premise of id() no longer holds", n)
	}
}

func TestIDTextFuncRendersID(t *testing.T) {
	st, raw := refStore(t)
	var got string
	if err := st.DB().QueryRow(`SELECT idtext("id") FROM "task"`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != guid.Encode(raw) {
		t.Fatalf("idtext = %q, want %q", got, guid.Encode(raw))
	}
}

// id() is idempotent, so wrapping an already-raw value is harmless.
func TestIDFuncAcceptsRawAndNull(t *testing.T) {
	st, raw := refStore(t)
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM "task" WHERE "parent"=id(?)`, raw).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("raw passthrough matched %d rows, want 1", n)
	}
	var null any
	if err := st.DB().QueryRow(`SELECT id(NULL)`).Scan(&null); err != nil {
		t.Fatal(err)
	}
	if null != nil {
		t.Fatalf("id(NULL) = %v, want NULL", null)
	}
}

// A typo says so rather than quietly returning no rows.
func TestIDFuncRejectsMalformed(t *testing.T) {
	st, _ := refStore(t)
	var n int
	err := st.DB().QueryRow(`SELECT COUNT(*) FROM "task" WHERE "parent"=id('nope')`).Scan(&n)
	if err == nil {
		t.Fatal("id('nope') was accepted")
	}
}
