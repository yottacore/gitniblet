// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yottacore/gitniblet/internal/core/ddl"
	"github.com/yottacore/gitniblet/internal/core/store"
	"github.com/yottacore/gitniblet/internal/core/txlog"
)

const lookupCheckDDL = `
tables:
	table: lookup_values
		fields:
			field: idx
				type: int
			field: lookups_idx
				type: int
			field: title
				type: string
	table: tasks
		fields:
			field: name
				type: string
			field: status
				type: int
				ui:
					list_type: lookup
					list_source: 1
			field: odd
				type: int
				ui:
					list_type: lookup
					list_source: nope
`

// newLookupCheckAPI gives group 1 an active Open (1), an inactive Closed (2)
// and Later (3), and group 2 Big (4).
func newLookupCheckAPI(t *testing.T) (*API, string, *ddl.Schema, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	sch, err := ddl.Parse([]byte(lookupCheckDDL))
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
	logDir := filepath.Join(dir, "log")
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(st, lg)
	a.UserID = "u1"
	a.EnableFeatures(sch)
	ids := map[string]string{}
	for _, row := range []map[string]string{
		{"idx": "1", "lookups_idx": "1", "title": "Open"},
		{"idx": "2", "lookups_idx": "1", "title": "Closed", "is_active": "0"},
		{"idx": "3", "lookups_idx": "1", "title": "Later"},
		{"idx": "4", "lookups_idx": "2", "title": "Big"},
	} {
		id, err := a.Create("lookup_values", row)
		if err != nil {
			t.Fatal(err)
		}
		ids[row["title"]] = id
	}
	return a, logDir, sch, ids
}

// ID: decar
func TestLookupWriteCheck(t *testing.T) {
	a, logDir, _, ids := newLookupCheckAPI(t)

	for _, tc := range []struct {
		val, want string // want "-" = refused
	}{
		{"1", "1"},
		{" 1 ", "1"},
		{"01", "1"},
		{"", ""},
		{"2", "-"}, // inactive
		{"4", "-"}, // another group's
		{"99", "-"},
		{"open", "-"},
	} {
		before := logLen(t, logDir)
		id, err := a.Create("tasks", map[string]string{"name": "t", "status": tc.val})
		if tc.want == "-" {
			if err == nil || !strings.Contains(err.Error(), "lookup group 1") {
				t.Errorf("create status=%q: err = %v, want a lookup refusal", tc.val, err)
			}
			if got := logLen(t, logDir); got != before {
				t.Errorf("create status=%q: refused, yet the log grew %d -> %d", tc.val, before, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("create status=%q: %v", tc.val, err)
			continue
		}
		if row, _, _ := a.Get("tasks", id); row["status"] != tc.want {
			t.Errorf("create status=%q stored %q, want %q", tc.val, row["status"], tc.want)
		}
	}

	// A non-lookup field, and one whose list_source isn't a group, take anything.
	if _, err := a.Create("tasks", map[string]string{"name": "99", "odd": "99"}); err != nil {
		t.Errorf("unchecked fields: %v", err)
	}

	id, err := a.Create("tasks", map[string]string{"name": "x", "status": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetField("tasks", id, "status", "2"); err == nil {
		t.Error("SetField to an inactive value went through")
	}
	if err := a.Update("tasks", id, map[string]string{"status": "4"}); err == nil {
		t.Error("Update to another group's value went through")
	}
	if err := a.SetField("tasks", id, "status", "1"); err != nil {
		t.Errorf("SetField to an active value: %v", err)
	}
	if err := a.SetField("tasks", id, "status", "3"); err != nil {
		t.Fatal(err)
	}

	// Once Later is gone, the row keeps it and may write it back, as a form
	// save does, but no other row can pick it.
	if err := a.MarkDelete("lookup_values", ids["Later"]); err != nil {
		t.Fatal(err)
	}
	if err := a.Update("tasks", id, map[string]string{"name": "y", "status": "3"}); err != nil {
		t.Errorf("writing back a deleted value the row holds: %v", err)
	}
	if err := a.SetField("tasks", id, "status", " 3"); err != nil {
		t.Errorf("SetField of the held value: %v", err)
	}
	if _, err := a.Create("tasks", map[string]string{"status": "3"}); err == nil {
		t.Error("create with a deleted value went through")
	}
	if err := a.SetFieldNull("tasks", id, "status"); err != nil {
		t.Errorf("SetFieldNull on a lookup field: %v", err)
	}
	if err := a.SetField("tasks", id, "status", "3"); err == nil {
		t.Error("a row that let go of a deleted value picked it again")
	}
}

// The check is write-time only. A value that was fine when logged, or one an
// older build let in, still applies at replay.
// ID: decas
func TestLookupReplayDoesNotCheck(t *testing.T) {
	a, logDir, sch, _ := newLookupCheckAPI(t)
	unchecked := New(a.st, a.log)
	unchecked.UserID = "u1"
	id, err := unchecked.Create("tasks", map[string]string{"name": "t", "status": "99"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.Create("tasks", map[string]string{"name": "u", "status": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetField("lookup_values", lookupIDOf(t, a, "1"), "is_active", "0"); err != nil {
		t.Fatal(err)
	}

	got := rebuild(t, logDir, sch, nil)
	for want, rowID := range map[string]string{"99": id, "1": other} {
		row, ok, err := got.Get("tasks", rowID)
		if err != nil || !ok || row["status"] != want {
			t.Errorf("rebuilt row: ok=%v err=%v status=%q, want %q", ok, err, row["status"], want)
		}
	}
}

func lookupIDOf(t *testing.T, a *API, idx string) string {
	t.Helper()
	rows, err := a.Query(`SELECT idtext("id") AS id FROM "lookup_values" WHERE "idx"=?`, idx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("lookup_values idx %s: rows=%v err=%v", idx, rows, err)
	}
	return rows[0]["id"]
}
