// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const optInDDL = `
tables:
	table: person
		fields:
			field: name
				type: string
		features:
			row_level_access: y
`

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "view.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func hasTable(t *testing.T, st *store.Store, name string) bool {
	t.Helper()
	var n int
	err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestBuiltinsParseClean(t *testing.T) {
	s, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Warnings) != 0 {
		t.Fatalf("builtins DDL should parse without warnings, got %v", s.Warnings)
	}
	byName := map[string]ddl.Table{}
	for _, tb := range s.Tables {
		byName[tb.Name] = tb
	}
	for _, want := range []string{"users", "groups", "many2many", "comments",
		"audit_trail", "access_rows", "attachments_uri", "attachments_copy"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("builtins DDL missing table %q", want)
		}
	}
	if !byName["audit_trail"].NoSystemFields {
		t.Error("audit_trail must opt out of system fields")
	}
	if len(s.Relations) != 2 {
		t.Errorf("want 2 implied relationships, got %d", len(s.Relations))
	}
}

func TestBootstrapAlwaysOnAndOptIn(t *testing.T) {
	st := openStore(t)
	if _, err := Bootstrap(st, nil); err != nil {
		t.Fatal(err)
	}
	// users/groups opt in to comments/audit/attachments themselves, so those
	// exist even with no user schema at all.
	for _, want := range []string{"users", "groups", "many2many", "comments",
		"audit_trail", "attachments_uri", "attachments_copy"} {
		if !hasTable(t, st, want) {
			t.Errorf("always-on table %q not created", want)
		}
	}
	if hasTable(t, st, "access_rows") {
		t.Error("access_rows created with nothing opting in to row_level_access")
	}

	user, err := ddl.Parse([]byte(optInDDL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Bootstrap(st, user); err != nil {
		t.Fatal(err)
	}
	if !hasTable(t, st, "access_rows") {
		t.Error("access_rows not created after a table opted in to row_level_access")
	}
}

func TestAuditTrailHasExactlyItsFields(t *testing.T) {
	st := openStore(t)
	if _, err := Bootstrap(st, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := st.DB().Query(`SELECT name FROM pragma_table_info('audit_trail')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got[name] = true
	}
	want := []string{"id", "table_name", "parent_id", "user_id", "date", "action", "values"}
	if len(got) != len(want) {
		t.Errorf("audit_trail: want exactly %d columns, got %d (%v)", len(want), len(got), got)
	}
	for _, c := range want {
		if !got[c] {
			t.Errorf("audit_trail missing column %q", c)
		}
	}
	for _, c := range []string{"is_active", "date_created", "is_deleted"} {
		if got[c] {
			t.Errorf("audit_trail must not get system column %q", c)
		}
	}
}

func TestSeedDefaultsIdempotent(t *testing.T) {
	st := openStore(t)
	if _, err := Bootstrap(st, nil); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	api.UserID = "system"

	countGroups := func() int {
		var n int
		if err := st.DB().QueryRow(
			`SELECT COUNT(*) FROM "groups" WHERE "is_deleted"=0`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if err := SeedDefaults(api); err != nil {
		t.Fatal(err)
	}
	if n := countGroups(); n != 4 {
		t.Fatalf("want 4 default groups, got %d", n)
	}
	if err := SeedDefaults(api); err != nil {
		t.Fatal(err)
	}
	if n := countGroups(); n != 4 {
		t.Fatalf("second seed must be a no-op, got %d groups", n)
	}

	// The seed is log-first: a fresh view rebuilt from the log has the groups
	// already, so seeding it again is also a no-op.
	st2 := openStore(t)
	if _, err := Bootstrap(st2, nil); err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txlog.Apply(st2, entries); err != nil {
		t.Fatal(err)
	}
	api2 := crud.New(st2, lg)
	if err := SeedDefaults(api2); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st2.DB().QueryRow(
		`SELECT COUNT(*) FROM "groups" WHERE "is_deleted"=0`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("replayed view: want 4 groups, got %d", n)
	}
}

func TestBootstrapWarnsOnReservedName(t *testing.T) {
	st := openStore(t)
	user, err := ddl.Parse([]byte("tables:\n\ttable: users\n\t\tfields:\n\t\t\tfield: nick\n\t\t\t\ttype: string\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Build(user); err != nil {
		t.Fatal(err)
	}
	warns, err := Bootstrap(st, user)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "built-in") && strings.Contains(w, "users") {
			found = true
		}
	}
	if !found {
		t.Errorf("want a built-in name warning for %q, got %v", "users", warns)
	}
	// The user's shape stands: their column exists, the built-in ones were not merged in.
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='screen_name'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("built-in users columns leaked into the user-defined users table")
	}
}
