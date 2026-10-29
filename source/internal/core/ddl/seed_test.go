// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"maps"
	"strings"
	"testing"
)

// ID: dec93
func TestSeedParse(t *testing.T) {
	src := `database:
	seed:
		table: lookups
			row: idx=1, title="Task status"
		table: lookup_values
			row: idx=1, lookups_idx=1, title="Open",   symbol="🟢", sort=1
			row: idx=2, lookups_idx=1, title="a, b", description="say \"hi\""  ## comment
			row: idx=3, title='x=y'
			row: title="no idx"
			row: idx=two, title="bad idx"
			row: idx=4, oops
			row: idx=2, title="dup"
		table: task
			row: idx=1
seed:
	table: lookups
		row: idx=9, title=, description=""
`
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		table  string
		idx    int
		fields map[string]string
	}{
		{"lookups", 1, map[string]string{"idx": "1", "title": "Task status"}},
		{"lookup_values", 1, map[string]string{"idx": "1", "lookups_idx": "1", "title": "Open", "symbol": "🟢", "sort": "1"}},
		{"lookup_values", 2, map[string]string{"idx": "2", "lookups_idx": "1", "title": "a, b", "description": `say "hi"`}},
		{"lookup_values", 3, map[string]string{"idx": "3", "title": "x=y"}},
		{"lookups", 9, map[string]string{"idx": "9", "title": "", "description": ""}},
	}
	if len(s.Seeds) != len(want) {
		t.Fatalf("seeds = %+v, want %d rows", s.Seeds, len(want))
	}
	for i, w := range want {
		got := s.Seeds[i]
		if got.Table != w.table || got.Idx != w.idx || len(got.Fields) != len(w.fields) {
			t.Fatalf("seed %d = %+v, want %s idx %d %v", i, got, w.table, w.idx, w.fields)
		}
		for k, v := range w.fields {
			if got.Fields[k] != v {
				t.Fatalf("seed %d %s = %q, want %q", i, k, got.Fields[k], v)
			}
		}
	}
	if s.Seeds[0].Line != 4 {
		t.Fatalf("seed line = %d, want 4", s.Seeds[0].Line)
	}

	joined := strings.Join(s.Warnings, "\n")
	for _, msg := range []string{
		"line 9: seed: lookup_values row is skipped; it needs an idx",
		"line 10: seed: lookup_values row is skipped; it needs an idx",
		`line 11: seed: lookup_values row is skipped; "oops" isn't a field=value pair`,
		"line 12: seed: lookup_values idx 2 is seeded more than once",
		`seed: table "task" is skipped`,
	} {
		if !strings.Contains(joined, msg) {
			t.Fatalf("missing warning %q in:\n%s", msg, joined)
		}
	}
	// a bad seed row costs a pick-list value, not data, so the db still opens
	if s.HasErrors() {
		t.Fatalf("seed problems should only warn, got errors: %v", s.Warnings)
	}
}

// ID: dec94
func TestSeedParseExample(t *testing.T) {
	s, err := ParseFile(findExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Seeds) != 3 {
		t.Fatalf("example seeds = %+v, want 3", s.Seeds)
	}
	if v := s.Seeds[2]; v.Table != "lookup_values" || v.Fields["title"] != "Closed" || v.Fields["symbol"] != "✅" {
		t.Fatalf("example seed 3 = %+v", v)
	}
	for _, w := range s.Warnings {
		if strings.Contains(w, "seed") {
			t.Fatalf("example seed warns: %s", w)
		}
	}
}

// Old seed rows keep what a quote holds, a # or a comma, on rows that repeat,
// and a trailing comment stays a comment.
// ID: dejk9
func TestOldSeedRowQuotes(t *testing.T) {
	src := "seed:\n" +
		"\ttable: lookups\n" +
		"\t\trow: idx=1, note=\"x # y\", title='p, q'  # comment\n" +
		"\t\trow: idx=2, title=\"say \\\"hi\\\", then go\"\n" +
		"\t\trow: idx=3, title=plain\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{
		{"idx": "1", "note": "x # y", "title": "p, q"},
		{"idx": "2", "title": `say "hi", then go`},
		{"idx": "3", "title": "plain"},
	}
	if len(s.Seeds) != len(want) || s.HasErrors() || len(s.Warnings) != 0 {
		t.Fatalf("seeds = %+v, warnings %v", s.Seeds, s.Warnings)
	}
	for i, w := range want {
		if got := s.Seeds[i]; !maps.Equal(got.Fields, w) || got.Line != i+3 {
			t.Errorf("seed %d = %+v, want %v on line %d", i, got, w, i+3)
		}
	}
}
