// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package shclio

import (
	"slices"
	"strings"
	"testing"
)

// Files from before shcl 3 come forward as read: a bare list gets brackets,
// a backtick value keeps its commas, and a current file is left alone.
// ID: defnx
func TestCurrent(t *testing.T) {
	sql := "`SELECT idx, title FROM lookups`"
	cases := []struct {
		name, text string
		list       []string
		sql        string
		line       int
	}{
		{"bare list", "t:\n\taliases: a, b\n", []string{"a", "b"}, "", 2},
		{"bare list and backtick", "t:\n\taliases: a, b\n\tsql: " + sql + "\n", []string{"a", "b"}, sql, 2},
		{"leading blank lines", "\n\nt:\n\taliases: a, b\n\tsql: " + sql + "\n", []string{"a", "b"}, sql, 4},
		{"blank run", "t:\n\n\n\n\taliases: a, b\n\n\n\tsql: " + sql + "\n", []string{"a", "b"}, sql, 5},
		{"current", "t:\n\taliases: [a, b]\n\tsql: " + sql + "\n", []string{"a", "b"}, sql, 2},
	}
	for _, c := range cases {
		doc, err := Load(c.text, Standard)
		if err != nil || doc.Errors() != 0 {
			t.Errorf("%s: err %v, diags %v", c.name, err, doc.Diags())
			continue
		}
		if got := doc.Strs("t.aliases", nil); !slices.Equal(got, c.list) {
			t.Errorf("%s: aliases = %q, want %q", c.name, got, c.list)
		}
		if got := doc.Line("t.aliases"); got != c.line {
			t.Errorf("%s: aliases on line %d, want %d", c.name, got, c.line)
		}
		if c.sql == "" {
			continue
		}
		if got := doc.GetStr("t.sql").Text; got != c.sql {
			t.Errorf("%s: sql = %q, want %q", c.name, got, c.sql)
		}
	}

	stamped := "t:\n\taliases: [a, b]\n##    Format   3\n"
	if got := Current(stamped); got != stamped {
		t.Errorf("a file naming format 3 changed:\n%s", got)
	}
	plain := "t:\n\taliases: [a, b]\n"
	if got := Current(plain); got != plain {
		t.Errorf("a current file changed:\n%s", got)
	}
}

// A saved file names its format, so a later load never guesses, and a value
// that needs quoting reads back. A selector body is always quoted, since a
// bare number selects by index.
// ID: defny
func TestTextAndSelect(t *testing.T) {
	doc := New()
	for _, name := range []string{"42", `o"brien`, "a b", "plain"} {
		if err := doc.Put(Select("user", name)+".hash", "h-"+name); err != nil {
			t.Fatalf("put %q: %v", name, err)
		}
	}
	text := Text(doc)
	if !strings.Contains(text, "Format   3") {
		t.Fatalf("saved text doesn't name its format:\n%s", text)
	}
	back, err := Load(text, Strict)
	if err != nil {
		t.Fatalf("reload: %v\n%s", err, text)
	}
	got := map[string]string{}
	for _, n := range back.Each("user") {
		got[n.Value] = back.Str(back.Child(n.Path, "hash"), "")
	}
	for _, name := range []string{"42", `o"brien`, "a b", "plain"} {
		if got[name] != "h-"+name {
			t.Errorf("user %q reloaded as %q:\n%s", name, got[name], text)
		}
	}
}
