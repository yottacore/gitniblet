// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

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

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"plain text",
		"line one\nline two\r\nthree",
		`comma, 'single' "double" ` + "`tick`" + ` back\slash for/ward`,
		"literal delims: \U0001F5E6 and \U0001F5E7",
		"looks like a marker: \U0001F5E6NEWLINE\U0001F5E7",
		"looks like null: \U0001F5E6NULL\U0001F5E7",
		"unicode: héllo wörld 你好",
	}
	for _, in := range cases {
		enc := encodeSym(in)
		if strings.ContainsAny(enc, "\n\r\",") {
			t.Errorf("encoded %q still holds CSV-unsafe chars: %q", in, enc)
		}
		if got := decodeSym(enc); got != in {
			t.Errorf("round-trip %q -> %q -> %q", in, enc, got)
		}
	}
}

func TestEncodeScrubsControlChars(t *testing.T) {
	if got := decodeSym(encodeSym("a\x07b\x00c\x7Fd\te")); got != "abcd\te" {
		t.Fatalf("scrub: %q", got)
	}
}

func TestDecodeLeavesLegacyAndUnknownAlone(t *testing.T) {
	for _, s := range []string{"plain", "\U0001F5E6BOGUS\U0001F5E7", "\U0001F5E6 unterminated"} {
		if got := decodeSym(s); got != s {
			t.Errorf("decode %q = %q, want unchanged", s, got)
		}
	}
}

// One entry must always be one physical line, or merge=union can tear records.
func TestAppendIsOneLinePerEntry(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{TxID: "t1", Table: "person", RowID: "aa", Field: "bio", Op: "update",
			NewValue: "multi\nline\r\nwith \"quotes\", commas"},
		{TxID: "t2", Table: "person", RowID: "aa", Field: "note", Op: "update", IsNull: true},
	}
	if err := lg.Append(entries...); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(lg.Path())
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimRight(string(b), "\n")
	if n := strings.Count(raw, "\n") + 1; n != 3 { // header + 2 entries
		t.Fatalf("file has %d lines, want 3:\n%s", n, raw)
	}
	if strings.Contains(raw, `"`) {
		t.Fatalf("writer needed CSV quoting; encoding should prevent that:\n%s", raw)
	}

	got, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].NewValue != entries[0].NewValue {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got[1].IsNull || got[1].NewValue != "" {
		t.Fatalf("null entry round-trip: %+v", got[1])
	}
}

// A user literal that spells the NULL sentinel must stay a string.
func TestNullSentinelCannotBeSpoofed(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lit := "\U0001F5E6NULL\U0001F5E7"
	if err := lg.Append(Entry{TxID: "t1", Table: "x", RowID: "aa", Field: "f", Op: "update", NewValue: lit}); err != nil {
		t.Fatal(err)
	}
	got, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].IsNull || got[0].NewValue != lit {
		t.Fatalf("sentinel spoofed: %+v", got[0])
	}
}

func TestApplyNullSetsSQLNull(t *testing.T) {
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
	row := testID("0a0a")
	entries := []Entry{
		{Op: "create", Table: "person", RowID: row, Field: "name", NewValue: "Ann"},
		{Op: "update", Table: "person", RowID: row, Field: "name", IsNull: true},
	}
	if _, err := Apply(st, entries); err != nil {
		t.Fatal(err)
	}
	id, _ := decodeID(row)
	var name sql.NullString
	if err := st.DB().QueryRow(`SELECT "name" FROM "person" WHERE "id"=?`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name.Valid {
		t.Fatalf("name = %q, want SQL NULL", name.String)
	}
}

// A torn or garbage line (e.g. a badly-merged fragment) must not make the rest
// of the log unreadable.
func TestReadAllSkipsMangledLines(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(
		Entry{TxID: "t1", Table: "person", RowID: "aa", Op: "create"},
		Entry{TxID: "t2", Table: "person", RowID: "aa", Field: "name", Op: "update", NewValue: "Ann"},
	); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lg.Path(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("torn,line\ngarbage \"unclosed\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := lg.Append(Entry{TxID: "t3", Table: "person", RowID: "aa", Op: "mark_delete"}); err != nil {
		t.Fatal(err)
	}

	got, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].TxID != "t3" {
		t.Fatalf("got %d entries (%+v), want the 3 good ones", len(got), got)
	}
}
