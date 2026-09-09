// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

const personDDL = `
tables:
	table: person
		fields:
			field: name
				type: string
			field: nick
				type: string
`

// capture runs fn with stdout redirected and returns what it printed.
func capture(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	ferr := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if ferr != nil {
		t.Fatalf("%v (output: %s)", ferr, out)
	}
	return string(out)
}

// regDB registers a database named "s" in an isolated registry so the
// name-based verbs resolve to it, and returns its record.
func regDB(t *testing.T) *config.DBConfig {
	t.Helper()
	base := t.TempDir()
	config.SetBaseOverride(base)
	t.Cleanup(func() { config.SetBaseOverride("") })

	work := t.TempDir()
	ddlPath := filepath.Join(work, "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(work, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Create("s", ddlPath, logDir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCrudVerbsEndToEnd(t *testing.T) {
	name := regDB(t).Name
	run := func(args ...string) string {
		return capture(t, func() error { return Run(args) })
	}

	id := strings.TrimSpace(run("create", name, "person", "name=Ann", "nick=A"))
	if len(id) != 22 {
		t.Fatalf("create should print a 22-char id, got %q", id)
	}

	got := run("get", name, "person", id)
	if !strings.Contains(got, "name: Ann") || !strings.Contains(got, "nick: A") {
		t.Fatalf("get output missing fields:\n%s", got)
	}

	run("update", name, "person", id, "name=Anne")
	got = run("get", name, "person", id)
	if !strings.Contains(got, "name: Anne") {
		t.Fatalf("update did not land:\n%s", got)
	}

	run("setnull", name, "person", id, "nick")
	got = run("query", name, `SELECT COUNT(*) AS n FROM "person" WHERE "nick" IS NULL`)
	if !strings.Contains(got, "1") {
		t.Fatalf("setnull did not produce SQL NULL:\n%s", got)
	}

	run("markdelete", name, "person", id)
	got = run("get", name, "person", id)
	if !strings.Contains(got, "is_deleted: 1") {
		t.Fatalf("markdelete did not set is_deleted:\n%s", got)
	}

	run("delete", name, "person", id)
	got = run("query", name, `SELECT COUNT(*) AS n FROM "person"`)
	if !strings.Contains(got, "0") {
		t.Fatalf("delete left the row behind:\n%s", got)
	}
}

// The database and table may be spelled explicitly with --db/--table (in any
// order) instead of positionally, and a name resolves with or without its
// file extension.
func TestCrudSelectorForms(t *testing.T) {
	name := regDB(t).Name
	run := func(args ...string) string {
		return capture(t, func() error { return Run(args) })
	}

	// fully explicit, flags leading in mixed order; name carries an extension
	id := strings.TrimSpace(run("create", "--table=person", "--db="+name+".ddl", "name=Bo"))
	if len(id) != 22 {
		t.Fatalf("flag-form create should print a 22-char id, got %q", id)
	}
	// space-form flags, table before db
	got := run("get", "-t", "person", "-d", name, id)
	if !strings.Contains(got, "name: Bo") {
		t.Fatalf("flag-form get missing the row:\n%s", got)
	}
}

// An unregistered name is a clear error, not a crash.
func TestUnknownDatabase(t *testing.T) {
	regDB(t) // isolates the registry; "nope" is not in it
	err := Run([]string{"get", "nope", "person", "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown database") {
		t.Fatalf("want an unknown-database error, got %v", err)
	}
}

func TestCreateRejectsBadAssign(t *testing.T) {
	name := regDB(t).Name
	err := Run([]string{"create", name, "person", "no-equals"})
	if err == nil || !strings.Contains(err.Error(), "field=value") {
		t.Fatalf("want a field=value error, got %v", err)
	}
}

// Rename ops end-to-end: rows written under the old names must survive the
// rename in place AND a full rebuild-from-log into a fresh view (the alias
// translation is what makes the second part work).
func TestRenameOpsEndToEnd(t *testing.T) {
	cfg := regDB(t)
	name := cfg.Name
	run := func(args ...string) string {
		return capture(t, func() error { return Run(args) })
	}

	id := strings.TrimSpace(run("create", name, "person", "name=Ann", "nick=Nan"))

	run("--rename-table", name, "person", "people")
	run("--rename-field", name, "people", "nick", "handle")

	// the DDL now has the new names with the old ones as aliases
	src, err := os.ReadFile(cfg.DDLPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"table: people", "aliases: person", "field: handle", "aliases: nick"} {
		if !strings.Contains(string(src), want) {
			t.Fatalf("DDL missing %q:\n%s", want, src)
		}
	}

	// renamed in place: the existing view answers under the new names
	out := run("get", name, "people", id)
	if !strings.Contains(out, "Ann") || !strings.Contains(out, "handle") {
		t.Fatalf("get after rename: %s", out)
	}

	// rebuild-from-log into a FRESH view: drop the sqlite and replay by name.
	// The log still says person/nick, so only the alias map makes this work.
	if err := os.Remove(cfg.SQLitePath); err != nil {
		t.Fatal(err)
	}
	run("replay", name)
	out = run("query", name, `SELECT "name","handle" FROM "people" WHERE "is_deleted"=0`)
	if !strings.Contains(out, "Ann") || !strings.Contains(out, "Nan") {
		t.Fatalf("replayed data missing: %s", out)
	}
}

// A command that could not run has to fail, or `ngdb ddl "$f" || die` in a
// script never fires. Usage goes to stderr for the same reason.
func TestUnrunnableCommandsFail(t *testing.T) {
	regDB(t) // isolate the registry
	for _, args := range [][]string{
		{"bogusverb"},
		{"ddl"},     // no file
		{"webuser"}, // no username
		{"build"},   // no database
	} {
		if err := Run(args); err == nil {
			t.Errorf("%v should have failed", args)
		}
	}
	if err := Help(); err != nil {
		t.Fatalf("--help should succeed: %v", err)
	}
}

// --table without --db used to swallow the first assignment as the database
// name, so the user was told their field value was a bad database.
func TestTableFlagWithoutDatabase(t *testing.T) {
	regDB(t)
	err := Run([]string{"create", "--table", "person", "name=Ann"})
	if err == nil || !strings.Contains(err.Error(), "missing database name") {
		t.Fatalf("want a missing-database error, got %v", err)
	}
}
