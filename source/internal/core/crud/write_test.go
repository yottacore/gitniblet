// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// uniquePersonDDL is personDDL with a natural key, so the view can refuse a
// write the log accepted.
const uniquePersonDDL = personDDL +
	"\t\tuniques:\n" +
	"\t\t\tunique: name\n"

// newUniqueAPI is newAPI over a schema whose name field is unique.
func newUniqueAPI(t *testing.T) (*API, string, *ddl.Schema) {
	t.Helper()
	dir := t.TempDir()
	schema, err := ddl.Parse([]byte(uniquePersonDDL))
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Tables[0].Uniques) != 1 {
		t.Fatalf("fixture has no unique group: %+v", schema.Tables[0].Uniques)
	}
	st, err := store.Open(filepath.Join(dir, "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(schema); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "log")
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(st, lg)
	a.UserID = "u1"
	return a, logDir, schema
}

// A duplicate natural key used to print a new id and succeed, leaving the field
// empty for good.
func TestCreateReportsAFieldTheViewRefused(t *testing.T) {
	a, _, _ := newUniqueAPI(t)

	if _, err := a.Create("person", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	id, err := a.Create("person", map[string]string{"name": "Ann"})
	if err == nil {
		t.Fatal("second create with a duplicate unique value reported success")
	}
	if !IsViewRejected(err) {
		t.Fatalf("want a view refusal, got %v", err)
	}
	if id == "" {
		t.Error("the row is in the log, so its id should still come back")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Errorf("the refusal should say what the view objected to: %v", err)
	}
	// The row exists; only the colliding field is missing.
	row, ok, err := a.Get("person", id)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if row["name"] != "" {
		t.Errorf("name = %q, want empty (the view refused it)", row["name"])
	}
}

// An ordinary write must not be mistaken for a refusal.
func TestCreateWithNoConflictIsNotReported(t *testing.T) {
	a, _, _ := newUniqueAPI(t)

	id, err := a.Create("person", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatalf("clean create: %v", err)
	}
	row, _, _ := a.Get("person", id)
	if row["name"] != "Ann" {
		t.Errorf("name = %q, want Ann", row["name"])
	}
}

// Editing a hard-deleted id used to recreate it as a stub holding only the
// edited field, which vanished again at the next rebuild.
func TestFieldWriteOnAHardDeletedRowIsRefused(t *testing.T) {
	a, _, _ := newAPI(t)

	id, err := a.Create("person", map[string]string{"name": "Ann", "age": "30"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Delete("person", id); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		do   func() error
	}{
		{"Update", func() error { return a.Update("person", id, map[string]string{"age": "31"}) }},
		{"SetField", func() error { return a.SetField("person", id, "age", "31") }},
		{"SetFieldNull", func() error { return a.SetFieldNull("person", id, "age") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.do(); err == nil {
				t.Fatal("writing to a hard-deleted row reported success")
			}
			if _, ok, err := a.Get("person", id); err != nil || ok {
				t.Errorf("a ghost row appeared: ok=%v err=%v", ok, err)
			}
		})
	}
}

// A soft-deleted row is still there, so it stays writable.
func TestFieldWriteOnASoftDeletedRowStillWorks(t *testing.T) {
	a, _, _ := newAPI(t)

	id, err := a.Create("person", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.MarkDelete("person", id); err != nil {
		t.Fatal(err)
	}
	if err := a.SetField("person", id, "age", "31"); err != nil {
		t.Fatalf("soft-deleted rows must stay writable: %v", err)
	}
	row, ok, _ := a.Get("person", id)
	if !ok || row["age"] != "31" {
		t.Errorf("age = %q ok=%v, want 31", row["age"], ok)
	}
}

// The view and the log used to disagree about a value carrying a control
// character: the view kept it, the log could not, and the next rebuild changed
// the value with no warning.
func TestControlCharactersReadBackTheSameAfterARebuild(t *testing.T) {
	a, logDir, schema := newAPI(t)

	const dirty = "a\x01b\x7fc"
	const clean = "abc"
	id, err := a.Create("person", map[string]string{"name": dirty})
	if err != nil {
		t.Fatal(err)
	}
	row, ok, err := a.Get("person", id)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if row["name"] != clean {
		t.Errorf("view holds %q, want %q - the log cannot carry the rest", row["name"], row)
	}
	if got := rebuildValue(t, logDir, schema, id, "name"); got != row["name"] {
		t.Errorf("rebuild gives %q but the live view holds %q", got, row["name"])
	}

	// Same for an update, which takes a different path into the log.
	if err := a.SetField("person", id, "name", "x\x02y"); err != nil {
		t.Fatal(err)
	}
	row, _, _ = a.Get("person", id)
	if row["name"] != "xy" {
		t.Errorf("view holds %q, want %q", row["name"], "xy")
	}
	if got := rebuildValue(t, logDir, schema, id, "name"); got != row["name"] {
		t.Errorf("rebuild gives %q but the live view holds %q", got, row["name"])
	}
}

// rebuildValue replays the log into a fresh view and reads one field back.
func rebuildValue(t *testing.T, logDir string, schema *ddl.Schema, id, field string) string {
	t.Helper()
	fresh, err := store.Open(filepath.Join(t.TempDir(), "rebuild.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.Build(schema); err != nil {
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
	if _, err := txlog.Apply(fresh, entries); err != nil {
		t.Fatal(err)
	}
	rebuilt := New(fresh, lg)
	row, ok, err := rebuilt.Get("person", id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		return ""
	}
	return row[field]
}

// Replay takes the write lock, so a background sync's rebuild cannot interleave
// with a write's log-then-view sequence. This exercises the two together (run
// with -race); the guarantee itself is structural - Replay is the only way in.
func TestReplayRunsAlongsideWrites(t *testing.T) {
	a, logDir, _ := newAPI(t)

	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	const writes = 20
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			if _, err := a.Create("person", map[string]string{"name": "Ann"}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			entries, _, err := lg.ReadAll()
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := a.Replay(entries); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()

	rows, err := a.Query(`SELECT COUNT(*) AS n FROM "person"`)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["n"] != "20" {
		t.Errorf("rows = %s, want 20", rows[0]["n"])
	}
}
