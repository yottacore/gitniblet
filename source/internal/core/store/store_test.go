// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/yottacore/gitniblet/internal/core/ddl"
)

const personDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\t\tnull_ok: no\n" +
	"\t\t\tfield: age\n" +
	"\t\t\t\ttype: int\n" +
	"\t\t\t\tdefaultval: 0\n" +
	"\t\tuniques:\n" +
	"\t\t\tunique: name\n" +
	"\t\tindexes:\n" +
	"\t\t\tindex: age\n"

// ID: d9e0m
func TestBuildSchema(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}

	st := openTemp(t)
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	// idempotent: building again must not error
	if err := st.Build(schema); err != nil {
		t.Fatalf("second build: %v", err)
	}

	cols := columns(t, st, "person")
	for _, want := range []string{"id", "name", "age", "is_active", "date_created", "is_deleted"} {
		if !cols[want] {
			t.Fatalf("missing column %q; have %v", want, cols)
		}
	}
	if !indexExists(t, st, "ux_person__name") {
		t.Fatal("expected partial unique index ux_person__name")
	}
	if !indexExists(t, st, "ix_person__age") {
		t.Fatal("expected index ix_person__age")
	}
}

// A field added to the DDL must reach an existing database via ALTER TABLE.
// ID: d9e0n
func TestBuildMigratesNewFields(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st := openTemp(t)
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`INSERT INTO "person" ("id","name") VALUES (?,?)`, []byte("01"), "Ann"); err != nil {
		t.Fatal(err)
	}

	const grownDDL = "tables:\n" +
		"\ttable: person\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: name\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\t\tnull_ok: no\n" +
		"\t\t\tfield: age\n" +
		"\t\t\t\ttype: int\n" +
		"\t\t\t\tdefaultval: 0\n" +
		"\t\t\tfield: email\n" + // new since first build
		"\t\t\t\ttype: string\n" +
		"\t\t\t\tnull_ok: no\n" + // NOT NULL without default: alter must relax it
		"\t\tuniques:\n" +
		"\t\t\tunique: name\n"
	grown, err := ddl.Parse([]byte(grownDDL))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Build(grown); err != nil {
		t.Fatalf("migrating build: %v", err)
	}
	if !columns(t, st, "person")["email"] {
		t.Fatal("email column not added to existing table")
	}
	// Existing row survives with the new column NULL.
	var email sql.NullString
	if err := st.DB().QueryRow(`SELECT "email" FROM "person"`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email.Valid {
		t.Fatalf("email = %q, want NULL", email.String)
	}
}

// Soft-deleted rows must not consume the unique key: many deleted rows may
// share a name, and a new live row may reuse it.
// ID: d9e0p
func TestUniqueIsPartial(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st := openTemp(t)
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO "person" ("id","name","is_deleted") VALUES (?,?,?)`
	for i, del := range []int{1, 1, 0} { // two deleted + one live, same name
		if _, err := st.DB().Exec(ins, []byte{byte(i)}, "Ann", del); err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
	}
	// A second LIVE row with the same name must still fail.
	if _, err := st.DB().Exec(ins, []byte{9}, "Ann", 0); err == nil {
		t.Fatal("expected unique violation on second live Ann")
	}
}

// ID: d9e0q
func TestSystemColumnsAndDefaults(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st := openTemp(t)
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}

	// Insert with only id + required name; defaults must fill the rest.
	if _, err := st.DB().Exec(`INSERT INTO "person" ("id","name") VALUES (?, ?)`, []byte("0123456789abcdef"), "Ann"); err != nil {
		t.Fatal(err)
	}
	var age, active, deleted int
	var created string
	row := st.DB().QueryRow(`SELECT "age","is_active","date_created","is_deleted" FROM "person"`)
	if err := row.Scan(&age, &active, &created, &deleted); err != nil {
		t.Fatal(err)
	}
	if age != 0 || active != 1 || deleted != 0 || created == "" {
		t.Fatalf("defaults wrong: age=%d active=%d deleted=%d created=%q", age, active, deleted, created)
	}

	// The unique index only polices live rows; a duplicate live name must fail.
	_, err = st.DB().Exec(`INSERT INTO "person" ("id","name") VALUES (?, ?)`, []byte("faceface0000 face"), "Ann")
	if err == nil {
		t.Fatal("expected unique-constraint violation on duplicate live name")
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func columns(t *testing.T, st *Store, table string) map[string]bool {
	t.Helper()
	rows, err := st.DB().Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	return out
}

func indexExists(t *testing.T, st *Store, name string) bool {
	t.Helper()
	var got string
	err := st.DB().QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&got)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return got == name
}

// A ref column is declared BLOB so a reference is stored the same way the row
// id it points at is, and IsRef marks it for replay's value conversion.
// ID: dabe5
func TestRefColumnIsBlob(t *testing.T) {
	schema, err := ddl.Parse([]byte("tables:\n" +
		"\ttable: task\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: parent_task\n" +
		"\t\t\t\ttype: ref\n" +
		"\t\t\tfield: title\n" +
		"\t\t\t\ttype: string\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(filepath.Join(t.TempDir(), "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}

	types := map[string]string{}
	rows, err := st.DB().Query(`SELECT name, type FROM pragma_table_info('task')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			t.Fatal(err)
		}
		types[name] = typ
	}
	if types["parent_task"] != "BLOB" {
		t.Fatalf("parent_task declared %q, want BLOB", types["parent_task"])
	}
	if types["id"] != "BLOB" {
		t.Fatalf("id declared %q, want BLOB", types["id"])
	}
	if !st.IsRef("task", "parent_task") {
		t.Fatal("IsRef should mark parent_task")
	}
	if st.IsRef("task", "title") {
		t.Fatal("IsRef must not mark a plain string field")
	}
}

// A view an older build made with NOT NULL on a null_ok: no field loses that
// constraint on the next Build, with its rows, extra columns and indexes intact
// (backlog 2026102407331876).
// ID: dec8x
func TestBuildRelaxesOldNotNull(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st := openTemp(t)
	for _, stmt := range []string{
		`CREATE TABLE "person" ("id" BLOB PRIMARY KEY, "name" TEXT NOT NULL, "age" INTEGER NOT NULL DEFAULT 0, "gone" TEXT,
			"is_active" INTEGER NOT NULL DEFAULT 1, "date_created" TEXT NOT NULL DEFAULT 'then', "is_deleted" INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO "person" ("id", "name", "age", "gone") VALUES (x'01', 'Ann', 7, 'kept')`,
	} {
		if _, err := st.DB().Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}

	var name, gone, created string
	var age int
	if err := st.DB().QueryRow(`SELECT "name", "age", "gone", "date_created" FROM "person" WHERE "id"=x'01'`).
		Scan(&name, &age, &gone, &created); err != nil {
		t.Fatal(err)
	}
	if name != "Ann" || age != 7 || gone != "kept" || created != "then" {
		t.Fatalf("row after rebuild: %q %d %q %q", name, age, gone, created)
	}
	var strict int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('person') WHERE "notnull" AND name IN ('name','age')`).Scan(&strict); err != nil {
		t.Fatal(err)
	}
	if strict != 0 {
		t.Fatalf("%d field column(s) still NOT NULL", strict)
	}
	if _, err := st.DB().Exec(`INSERT INTO "person" ("id") VALUES (x'02')`); err != nil {
		t.Fatalf("bare-id insert, as replay does it: %v", err)
	}
	if !indexExists(t, st, "ux_person__name") || !indexExists(t, st, "ix_person__age") {
		t.Fatal("indexes missing after rebuild")
	}
	if cols := columns(t, st, "person__relaxed"); len(cols) > 0 {
		t.Fatalf("temp table left behind: %v", cols)
	}
}
