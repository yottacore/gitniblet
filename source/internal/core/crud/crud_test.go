// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"path/filepath"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const personDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: age\n" +
	"\t\t\t\ttype: int\n"

// newAPI builds a schema + store + log in a temp dir and returns the API and
// the log dir (so a test can rebuild a fresh view from the same log).
func newAPI(t *testing.T) (*API, string, *ddl.Schema) {
	t.Helper()
	dir := t.TempDir()
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
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

func TestCreateGetUpdateDelete(t *testing.T) {
	a, _, _ := newAPI(t)

	id, err := a.Create("person", map[string]string{"name": "Ann", "age": "30"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok, err := a.Get("person", id)
	if err != nil || !ok {
		t.Fatalf("Get after create: ok=%v err=%v", ok, err)
	}
	if m["name"] != "Ann" || m["age"] != "30" || m["id"] != id || m["is_deleted"] != "0" {
		t.Fatalf("row = %+v", m)
	}
	if m["date_created"] == "" {
		t.Fatal("date_created should be populated")
	}

	if err := a.Update("person", id, map[string]string{"name": "Anne"}); err != nil {
		t.Fatal(err)
	}
	if m, _, _ := a.Get("person", id); m["name"] != "Anne" {
		t.Fatalf("after update name=%q", m["name"])
	}

	if err := a.MarkDelete("person", id); err != nil {
		t.Fatal(err)
	}
	if m, _, _ := a.Get("person", id); m["is_deleted"] != "1" {
		t.Fatalf("after mark_delete is_deleted=%q", m["is_deleted"])
	}

	if err := a.Delete("person", id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := a.Get("person", id); ok {
		t.Fatal("row should be gone after Delete")
	}
}

func TestReadOnlyBlocksWrites(t *testing.T) {
	a, _, _ := newAPI(t)
	id, err := a.Create("person", map[string]string{"name": "Ann", "age": "30"})
	if err != nil {
		t.Fatal(err)
	}

	a.SetReadOnly(true)
	if !a.ReadOnly() {
		t.Fatal("ReadOnly() should be true")
	}
	// Every write path funnels through commit, so all of them refuse.
	if _, err := a.Create("person", map[string]string{"name": "Bo"}); err != ErrReadOnly {
		t.Fatalf("Create in read-only = %v, want ErrReadOnly", err)
	}
	if err := a.Update("person", id, map[string]string{"name": "Anne"}); err != ErrReadOnly {
		t.Fatalf("Update in read-only = %v, want ErrReadOnly", err)
	}
	if err := a.SetField("person", id, "age", "31"); err != ErrReadOnly {
		t.Fatalf("SetField in read-only = %v, want ErrReadOnly", err)
	}
	if err := a.MarkDelete("person", id); err != ErrReadOnly {
		t.Fatalf("MarkDelete in read-only = %v, want ErrReadOnly", err)
	}
	if err := a.Delete("person", id); err != ErrReadOnly {
		t.Fatalf("Delete in read-only = %v, want ErrReadOnly", err)
	}
	// Reads keep working, and nothing changed.
	if m, ok, _ := a.Get("person", id); !ok || m["name"] != "Ann" || m["age"] != "30" {
		t.Fatalf("read-only mutated the row: %+v ok=%v", m, ok)
	}

	// Clearing it restores writes.
	a.SetReadOnly(false)
	if err := a.Update("person", id, map[string]string{"name": "Anne"}); err != nil {
		t.Fatalf("Update after clearing read-only: %v", err)
	}
	if m, _, _ := a.Get("person", id); m["name"] != "Anne" {
		t.Fatalf("write after clear didn't land: %+v", m)
	}
}

func TestWritesStampHost(t *testing.T) {
	a, logDir, _ := newAPI(t)
	a.HostID = "box-x"
	if _, err := a.Create("person", map[string]string{"name": "Ann"}); err != nil {
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
	if len(entries) == 0 {
		t.Fatal("no entries written")
	}
	for _, e := range entries {
		if e.HostName != "box-x" {
			t.Fatalf("entry %+v: host_name = %q, want box-x", e, e.HostName)
		}
	}
}

func TestSetFieldRejectsID(t *testing.T) {
	a, _, _ := newAPI(t)
	id, _ := a.Create("person", nil)
	if err := a.SetField("person", id, "id", "deadbeef"); err == nil {
		t.Fatal("expected error setting id")
	}
}

// The log alone must reconstruct the view: replay the API's log into a brand-new
// store and confirm the rows match.
func TestLogIsSourceOfTruth(t *testing.T) {
	a, logDir, schema := newAPI(t)

	idA, err := a.Create("person", map[string]string{"name": "Ann", "age": "30"})
	if err != nil {
		t.Fatal(err)
	}
	idB, _ := a.Create("person", map[string]string{"name": "Bo"})
	if err := a.Update("person", idA, map[string]string{"age": "31"}); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkDelete("person", idB); err != nil {
		t.Fatal(err)
	}

	// Fresh, empty view rebuilt purely from the log.
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
	if m, ok, _ := rebuilt.Get("person", idA); !ok || m["name"] != "Ann" || m["age"] != "31" {
		t.Fatalf("rebuilt row A = %+v ok=%v", m, ok)
	}
	if m, _, _ := rebuilt.Get("person", idB); m["is_deleted"] != "1" {
		t.Fatalf("rebuilt row B is_deleted=%q", m["is_deleted"])
	}
}
