// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"strings"
	"testing"
)

func TestParseQueries(t *testing.T) {
	src := "## comment\n" +
		"query_name: \"Second\"\n" +
		"\tview: \"people\"\n" +
		"\tsort: 5.5\n" +
		"\tactive: yes\n" +
		"\tSQL: `SELECT * FROM t WHERE f = 'Joe;'`\n" +
		"query_name: \"First\"\n" +
		"\tsort: 1\n" +
		"\tsql: `SELECT 1`\n" +
		"query_name: \"Hidden\"\n" +
		"\tactive: no\n" +
		"\tSQL: `SELECT 2`\n"
	qs, warns, err := ParseQueries([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if len(qs) != 3 {
		t.Fatalf("got %d queries: %+v", len(qs), qs)
	}
	// dropdown order: sort value wins over file order; missing sort is 0
	if qs[0].Name != "Hidden" || qs[1].Name != "First" || qs[2].Name != "Second" {
		t.Fatalf("order = %s, %s, %s", qs[0].Name, qs[1].Name, qs[2].Name)
	}
	second := qs[2]
	if second.View != "people" || second.Sort != 5.5 || !second.Active ||
		second.SQL != "SELECT * FROM t WHERE f = 'Joe;'" {
		t.Fatalf("second = %+v", second)
	}
	if qs[0].Active {
		t.Fatal("active: no should stick")
	}
	if !qs[1].Active { // unspecified defaults to active
		t.Fatal("First should default active")
	}
}

func TestParseQueriesSoftErrors(t *testing.T) {
	src := "query_name:\n" +
		"\tSQL: `SELECT 1`\n" +
		"query_name: \"Dup\"\n" +
		"\tSQL: `SELECT 1`\n" +
		"query_name: \"Dup\"\n" +
		"\tSQL: `SELECT 2`\n" +
		"query_name: \"NoSQL\"\n" +
		"\tview: \"x\"\n" +
		"query_name: \"OddKey\"\n" +
		"\tfrobnicate: 3\n" +
		"\tSQL: `SELECT 3`\n"
	qs, warns, err := ParseQueries([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// "Dup" appears twice and merges into one query by SHCL's (name, value) rule,
	// so a repeat cannot arrive as a second entry - its children join the first,
	// and the first sql: wins.
	if len(qs) != 2 || qs[0].Name != "Dup" || qs[0].SQL != "SELECT 1" || qs[1].Name != "OddKey" {
		t.Fatalf("queries = %+v", qs)
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"no name", "no SQL", "frobnicate"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings missing %q:\n%s", want, joined)
		}
	}
}

func TestQueriesPathAndMissingFile(t *testing.T) {
	if got := QueriesPath("/a/b/example.shcl"); got != "/a/b/example.queries.shcl" {
		t.Fatalf("QueriesPath = %q", got)
	}
	qs, warns, err := ParseQueriesFile("/nonexistent/nope.queries")
	if qs != nil || warns != nil || err != nil {
		t.Fatalf("missing file should be all-nil: %v %v %v", qs, warns, err)
	}
}
