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

const notNullDDL = "tables:\n" +
	"\ttable: tag\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: title\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\t\tvalidation:\n" +
	"\t\t\t\t\tnull_ok: n\n" +
	"\t\t\tfield: color\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\t\tnull_ok: n\n" +
	"\t\t\t\tdefaultval: \"red\"\n" +
	"\t\t\tfield: note\n" +
	"\t\t\t\ttype: string\n" +
	"\ttable: swatch\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: color\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\t\tnull_ok: n\n" +
	"\t\t\t\tdefaultval: \"red\"\n"

func newNotNullAPI(t *testing.T) (*API, string, *ddl.Schema) {
	t.Helper()
	dir := t.TempDir()
	sch, err := ddl.Parse([]byte(notNullDDL))
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
	return a, logDir, sch
}

func logLen(t *testing.T, logDir string) int {
	t.Helper()
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

// A null_ok: no field with no default made the view drop every row of its
// table, with no error (backlog 2026102407331876).
// ID: dec8s
func TestNotNullRowReachesView(t *testing.T) {
	a, logDir, sch := newNotNullAPI(t)

	id, err := a.Create("tag", map[string]string{"title": "urgent"})
	if err != nil {
		t.Fatal(err)
	}
	row, ok, err := a.Get("tag", id)
	if err != nil || !ok {
		t.Fatalf("live view lost the row: ok=%v err=%v", ok, err)
	}
	if row["title"] != "urgent" || row["color"] != "red" {
		t.Fatalf("row = %+v", row)
	}
	if row, ok, _ := rebuild(t, logDir, sch, nil).Get("tag", id); !ok || row["title"] != "urgent" {
		t.Fatalf("rebuilt view: ok=%v row=%+v", ok, row)
	}
}

// Leaving a null_ok: no field null is refused before anything reaches the log.
// A default covers an omitted field, as the view fills it.
// ID: dec8t
func TestNotNullRefusedUpFront(t *testing.T) {
	a, logDir, _ := newNotNullAPI(t)

	id, err := a.Create("tag", map[string]string{"title": "a"})
	if err != nil {
		t.Fatal(err)
	}
	before := logLen(t, logDir)

	if _, err := a.Create("tag", map[string]string{"note": "no title"}); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("create without title: err = %v", err)
	}
	if err := a.SetFieldNull("tag", id, "title"); err == nil {
		t.Error("SetFieldNull on a null_ok: no field went through")
	}
	if err := a.SetFieldNull("tag", id, "color"); err == nil {
		t.Error("SetFieldNull on a null_ok: no field with a default went through")
	}
	if err := a.SetFieldNull("tag", id, "is_active"); err == nil {
		t.Error("SetFieldNull on a system column went through")
	}
	if after := logLen(t, logDir); after != before {
		t.Errorf("refused writes reached the log: %d -> %d entries", before, after)
	}

	if err := a.SetFieldNull("tag", id, "note"); err != nil {
		t.Errorf("SetFieldNull on a nullable field: %v", err)
	}
	if err := a.SetField("tag", id, "title", ""); err != nil {
		t.Errorf("empty is not null: %v", err)
	}
}

// A null already in the log, from an older client or an older schema, must not
// stop the rebuild. It used to fail the whole replay with a NOT NULL error.
// ID: dec8v
func TestNotNullReplayKeepsLoggedNull(t *testing.T) {
	a, logDir, sch := newNotNullAPI(t)

	id, err := a.Create("tag", map[string]string{"title": "a"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"title", "color"} {
		old := a.entry("tag", id, field, "update", "")
		old.IsNull = true
		if err := a.log.Append(old); err != nil {
			t.Fatal(err)
		}
	}
	// swatch's only null_ok: no field has a default, so its row always reached
	// the view, and a logged null then failed the rebuild outright.
	swatch, err := a.Create("swatch", nil)
	if err != nil {
		t.Fatal(err)
	}
	old := a.entry("swatch", swatch, "color", "update", "")
	old.IsNull = true
	if err := a.log.Append(old); err != nil {
		t.Fatal(err)
	}
	// System columns stay NOT NULL, so a logged null there is skipped instead.
	sys := a.entry("tag", id, "is_active", "update", "")
	sys.IsNull = true
	if err := a.log.Append(sys); err != nil {
		t.Fatal(err)
	}

	got := rebuild(t, logDir, sch, nil)
	row, ok, err := got.Get("tag", id)
	if err != nil || !ok {
		t.Fatalf("rebuilt view lost the row: ok=%v err=%v", ok, err)
	}
	if row["title"] != "" || row["color"] != "" || row["is_active"] != "1" {
		t.Fatalf("row = %+v, want the logged nulls and is_active kept", row)
	}
	if row, ok, _ := got.Get("swatch", swatch); !ok || row["color"] != "" {
		t.Fatalf("swatch: ok=%v row=%+v", ok, row)
	}
}
