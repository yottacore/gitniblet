// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package txlog

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
)

const personDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: age\n" +
	"\t\t\t\ttype: int\n"

func TestAppendReadRoundTrip(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := []Entry{
		{TxID: "t1", Date: "2026-07-01T00:00:00Z", Table: "person", RowID: "aabb", Op: "create"},
		{TxID: "t2", Date: "2026-07-01T00:00:01Z", Table: "person", RowID: "aabb", Field: "name", Op: "update", NewValue: "Ann, \"the boss\"", UserID: "u1", GCReady: true},
	}
	if err := lg.Append(in...); err != nil {
		t.Fatal(err)
	}
	// A second append must not re-emit the header.
	if err := lg.Append(Entry{TxID: "t3", Table: "person", RowID: "aabb", Op: "mark_delete"}); err != nil {
		t.Fatal(err)
	}

	got, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[1].NewValue != `Ann, "the boss"` || !got[1].GCReady || got[1].Field != "name" {
		t.Fatalf("round-trip mismatch: %+v", got[1])
	}
	if got[2].Op != "mark_delete" {
		t.Fatalf("entry 3 = %+v", got[2])
	}
}

// host_name round-trips, and a legacy record (no host_name column, as an
// older client or a pre-host_name log would have written) still parses with
// Columns are mapped by header name, so a reordered header and an unknown extra
// trailing column (as a newer client might write) both read correctly - field
// order does not affect forward or backward compatibility.
func TestReadMapsColumnsByName(t *testing.T) {
	dir := t.TempDir()
	content := "host_name,tx_id,date,table_name,row_id,field_name,operation,new_value,user_id,ok_to_garbage_collect,future_flag\n" +
		"box-z,t9,2026-07-02T00:00:00Z,person,rid1,name,update,Zoe,u9,false,X\n"
	if err := os.WriteFile(filepath.Join(dir, "txlog.csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	lg, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, warns, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("reordered/extended record should not warn, got %v", warns)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	e := got[0]
	if e.TxID != "t9" || e.HostName != "box-z" || e.Table != "person" || e.RowID != "rid1" ||
		e.Field != "name" || e.Op != "update" || e.NewValue != "Zoe" || e.UserID != "u9" {
		t.Fatalf("columns mismapped by name: %+v", e)
	}
}

// an empty host - the two widths coexist under a git union merge.
func TestHostNameRoundTripAndLegacy(t *testing.T) {
	dir := t.TempDir()
	lg, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(Entry{TxID: "t1", Date: "2026-07-02T00:00:00Z", Table: "person", RowID: "aabb", Field: "name", Op: "update", NewValue: "Ann", UserID: "u1", HostName: "box-a"}); err != nil {
		t.Fatal(err)
	}
	// A legacy 9-field line, appended raw as a union merge from an older client would.
	legacy := "t2,2026-07-02T00:00:01Z,person,aabb,age,update,40,u1,false\n"
	f, err := os.OpenFile(lg.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(legacy); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, warns, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("legacy record should not warn, got %v", warns)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].HostName != "box-a" {
		t.Fatalf("host_name round-trip: got %q", got[0].HostName)
	}
	if got[1].HostName != "" || got[1].NewValue != "40" {
		t.Fatalf("legacy record mismatch: %+v", got[1])
	}
}

// An encrypted entry round-trips through the ENC marker, and a cleartext value
// that literally equals the ENC marker text is escaped so it never reads back
// as encrypted.
func TestEncryptedMarkerRoundTrip(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := []Entry{
		{TxID: "t1", Date: "2026-07-02T00:00:00Z", Table: "person", RowID: "aabb", Field: "ssn", Op: "update", NewValue: "AbC-payload_123", Enc: true},
		{TxID: "t2", Date: "2026-07-02T00:00:01Z", Table: "person", RowID: "aabb", Field: "note", Op: "update", NewValue: encMarker + "not really encrypted"},
	}
	if err := lg.Append(in...); err != nil {
		t.Fatal(err)
	}
	got, warns, err := lg.ReadAll()
	if err != nil || len(warns) != 0 {
		t.Fatalf("read: warns=%v err=%v", warns, err)
	}
	if !got[0].Enc || got[0].NewValue != "AbC-payload_123" {
		t.Fatalf("encrypted entry round-trip: %+v", got[0])
	}
	if got[1].Enc || got[1].NewValue != encMarker+"not really encrypted" {
		t.Fatalf("cleartext equal to the marker must not read as encrypted: %+v", got[1])
	}
}

// Mangled lines skip WITH a warning; repeated header rows (union-merge
// leftovers) skip silently - they are expected, not a defect.
func TestReadAllWarnsOnMangledLines(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(Entry{TxID: "t1", Date: "1", Table: "person", RowID: "aa", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lg.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	// a torn line, then a header repeat, then another good record
	if _, err := f.WriteString("torn,line\n" +
		"tx_id,date,table_name,row_id,field_name,operation,new_value,user_id,ok_to_garbage_collect\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := lg.Append(Entry{TxID: "t2", Date: "2", Table: "person", RowID: "aa", Op: "mark_delete"}); err != nil {
		t.Fatal(err)
	}

	got, warns, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "torn") {
		t.Fatalf("warnings = %v, want exactly the torn-line one", warns)
	}
}

func TestReadMissingLogIsEmpty(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := lg.ReadAll()
	if err != nil || got != nil {
		t.Fatalf("missing log: got %v, err %v", got, err)
	}
}

func TestApplyReplaysToView(t *testing.T) {
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}

	rowA, rowB := testID("0a0a"), testID("0b0b")
	entries := []Entry{
		{Op: "create", Table: "person", RowID: rowA, Field: "name", NewValue: "Ann"},
		{Op: "update", Table: "person", RowID: rowA, Field: "age", NewValue: "30"},
		{Op: "create", Table: "person", RowID: rowB, Field: "name", NewValue: "Bo"},
		{Op: "mark_delete", Table: "person", RowID: rowB},
		{Op: "update", Table: "person", RowID: rowA, Field: "name", NewValue: "Anne"}, // last write wins
	}
	if _, err := Apply(st, entries); err != nil {
		t.Fatal(err)
	}

	// Row A: name coalesced to last write, age coerced to int.
	name, age := lookup(t, st, rowA)
	if name != "Anne" || age != 30 {
		t.Fatalf("row A = name %q age %d", name, age)
	}
	// Row B: soft-deleted.
	if !softDeleted(t, st, rowB) {
		t.Fatal("row B should be is_deleted=1")
	}

	// A hard delete removes the row entirely.
	if _, err := Apply(st, []Entry{{Op: "delete", Table: "person", RowID: rowA}}); err != nil {
		t.Fatal(err)
	}
	var n int
	idA, _ := decodeID(rowA)
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM "person" WHERE "id"=?`, idA).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("row A still present after delete: %d", n)
	}
}

func lookup(t *testing.T, st *store.Store, rowID string) (string, int) {
	t.Helper()
	id, _ := decodeID(rowID)
	var name sql.NullString
	var age sql.NullInt64
	err := st.DB().QueryRow(`SELECT "name","age" FROM "person" WHERE "id"=?`, id).Scan(&name, &age)
	if err != nil {
		t.Fatal(err)
	}
	return name.String, int(age.Int64)
}

func softDeleted(t *testing.T, st *store.Store, rowID string) bool {
	t.Helper()
	id, _ := decodeID(rowID)
	var del int
	if err := st.DB().QueryRow(`SELECT "is_deleted" FROM "person" WHERE "id"=?`, id).Scan(&del); err != nil {
		t.Fatal(err)
	}
	return del == 1
}
