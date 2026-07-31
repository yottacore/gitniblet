// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
)

func newView(t *testing.T, src string) *store.Store {
	t.Helper()
	schema, err := ddl.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	return st
}

const uniqueDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\tuniques:\n" +
	"\t\t\tunique: name\n"

// Two soft-deletes of the same natural key used to collide on the unique index
// and abort the entire replay; partial indexes must make this a non-event.
func TestReplaySurvivesRepeatedSoftDelete(t *testing.T) {
	st := newView(t, uniqueDDL)
	row1, row2, row3 := testID("01"), testID("02"), testID("03")
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "mark_delete", Table: "person", RowID: row1},
		{Date: "3", TxID: "c", Op: "create", Table: "person", RowID: row2, Field: "name", NewValue: "Ann"},
		{Date: "4", TxID: "d", Op: "mark_delete", Table: "person", RowID: row2},
		{Date: "5", TxID: "e", Op: "create", Table: "person", RowID: row3, Field: "name", NewValue: "Ann"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	var live, dead int
	if err := st.DB().QueryRow(
		`SELECT SUM("is_deleted"=0), SUM("is_deleted"=1) FROM "person"`).Scan(&live, &dead); err != nil {
		t.Fatal(err)
	}
	if live != 1 || dead != 2 {
		t.Fatalf("live=%d dead=%d, want 1 live + 2 dead", live, dead)
	}
}

// Entries against tables or columns the local schema doesn't know are skipped
// with a warning, never fatal - the log may be newer than the DDL.
func TestReplaySkipsSchemaDrift(t *testing.T) {
	st := newView(t, uniqueDDL)
	row1, row2 := testID("01"), testID("02")
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "update", Table: "person", RowID: row1, Field: "no_such_field", NewValue: "x"},
		{Date: "3", TxID: "c", Op: "create", Table: "no_such_table", RowID: row2, Field: "f", NewValue: "y"},
		{Date: "4", TxID: "d", Op: "update", Table: "person", RowID: row1, Field: "name", NewValue: "Anne"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want 2", warns)
	}
	id, _ := decodeID(row1)
	var name string
	if err := st.DB().QueryRow(`SELECT "name" FROM "person" WHERE "id"=?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Anne" {
		t.Fatalf("name = %q; good entries after a skipped one must still apply", name)
	}
}

// Concurrent creates of the same natural key merge deterministically: the
// later one is skipped everywhere, not fatal anywhere.
func TestReplaySkipsUniqueLoser(t *testing.T) {
	st := newView(t, uniqueDDL)
	row1, row2 := testID("01"), testID("02")
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "create", Table: "person", RowID: row2, Field: "name", NewValue: "Ann"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "UNIQUE") {
		t.Fatalf("warnings = %v, want one unique-loser skip", warns)
	}
}

// Entry-local data errors - a mangled row id, an operation this build does
// not know (possibly a newer client's) - skip with a warning like schema
// drift; they must never abort the whole replay.
func TestReplaySkipsBadEntries(t *testing.T) {
	st := newView(t, uniqueDDL)
	row1 := testID("01")
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "frobnicate", Table: "person", RowID: row1},
		{Date: "3", TxID: "c", Op: "update", Table: "person", RowID: "zz", Field: "name", NewValue: "x"},
		{Date: "4", TxID: "d", Op: "update", Table: "person", RowID: row1, Field: "name", NewValue: "Anne"},
	}
	warns, err := Apply(st, entries)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want 2", warns)
	}
	id, _ := decodeID(row1)
	var name string
	if err := st.DB().QueryRow(`SELECT "name" FROM "person" WHERE "id"=?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Anne" {
		t.Fatalf("name = %q; good entries after skipped ones must still apply", name)
	}
}

// A hard-deleted row must stay dead: later update entries (e.g. surviving a
// future GC of the delete's own creates) must not resurrect a one-field ghost.
func TestReplayDeleteWinsOverLaterUpdates(t *testing.T) {
	st := newView(t, uniqueDDL)
	row1 := testID("01")
	entries := []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Ann"},
		{Date: "2", TxID: "b", Op: "delete", Table: "person", RowID: row1},
		{Date: "3", TxID: "c", Op: "update", Table: "person", RowID: row1, Field: "name", NewValue: "Ghost"},
		{Date: "4", TxID: "d", Op: "mark_delete", Table: "person", RowID: row1},
	}
	if _, err := Apply(st, entries); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "person"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d row(s) after hard delete, want 0", n)
	}

	// ...but a later create legitimately reuses the row id.
	if _, err := Apply(st, append(entries,
		Entry{Date: "5", TxID: "e", Op: "create", Table: "person", RowID: row1, Field: "name", NewValue: "Reborn"},
	)); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "person"`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d row(s) after re-create, want 1", n)
	}
}

// A ref column holds the same raw bytes as a row's own primary key, so a
// reference joins straight against the id it points at.
func TestApplyStoresRefAsRawBytes(t *testing.T) {
	st := newView(t, "tables:\n"+
		"\ttable: task\n"+
		"\t\tfields:\n"+
		"\t\t\tfield: parent_task\n"+
		"\t\t\t\ttype: ref\n")
	row, parent := testID("01"), testID("02")
	warns, err := Apply(st, []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "task", RowID: parent},
		{Date: "2", TxID: "b", Op: "create", Table: "task", RowID: row, Field: "parent_task", NewValue: parent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}

	id, _ := decodeID(row)
	want, _ := decodeID(parent)
	var got []byte
	if err := st.DB().QueryRow(`SELECT "parent_task" FROM "task" WHERE "id"=?`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ref stored %x, want the parent's raw id %x", got, want)
	}

	var joined int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "task" c JOIN "task" p ON c."parent_task"=p."id"`).
		Scan(&joined); err != nil {
		t.Fatal(err)
	}
	if joined != 1 {
		t.Fatalf("ref joined %d row(s) against id, want 1", joined)
	}
}

// A ref value that is not an id skips with a warning rather than landing a
// string in a column everything else treats as bytes.
func TestApplySkipsMalformedRef(t *testing.T) {
	st := newView(t, "tables:\n"+
		"\ttable: task\n"+
		"\t\tfields:\n"+
		"\t\t\tfield: parent_task\n"+
		"\t\t\t\ttype: ref\n")
	warns, err := Apply(st, []Entry{
		{Date: "1", TxID: "a", Op: "create", Table: "task", RowID: testID("01"),
			Field: "parent_task", NewValue: "not-an-id"},
	})
	if err != nil {
		t.Fatalf("a bad ref must warn, not abort: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "parent_task") {
		t.Fatalf("warnings = %v, want one naming the ref column", warns)
	}
}
