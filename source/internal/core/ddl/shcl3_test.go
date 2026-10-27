// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"reflect"
	"testing"

	"github.com/yottacore/gitniblet/internal/core/shclio"
)

// The vocabulary is written for shcl 3, so it loads with no upgrade and no
// errors, and its allowed lists still check a DDL.
// ID: defnz
func TestVocabularyIsShcl3(t *testing.T) {
	text, err := schemaFS.ReadFile("schema.shcl")
	if err != nil {
		t.Fatal(err)
	}
	if shclio.Current(string(text)) != string(text) {
		t.Error("schema.shcl needs an upgrade to read on shcl 3")
	}
	if doc, _ := shclio.Load(string(text), shclio.Standard); doc.Errors() != 0 {
		t.Errorf("schema.shcl has errors on shcl 3: %v", doc.Diags())
	}
	if doc, _ := shclio.Load(queriesSchema, shclio.Standard); doc.Errors() != 0 {
		t.Errorf("the queries schema has errors on shcl 3: %v", doc.Diags())
	}
	s, _ := Parse([]byte("database:\n\tencryption: sometimes\n"))
	if !hasWarning(s, "sometimes") {
		t.Errorf("a value outside allowed went unreported: %v", s.Warnings)
	}
}

// A DDL written for the old rules reads the same as one written for shcl 3:
// bare lists, a backtick query, a literal list, and seed rows with quoted
// commas and escapes. The leading blank line keeps the line numbers honest.
// ID: defp0
func TestOldDDLReadsLikeNew(t *testing.T) {
	old := "\ntables:\n" +
		"\ttable: lookup_values\n" +
		"\t\taliases: values, \"old values\"\n" +
		"\t\tuniques:\n" +
		"\t\t\tunique: title, sort\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: title\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\t\tui:\n" +
		"\t\t\t\t\tlist_type: literal\n" +
		"\t\t\t\t\tlist_source: \"Bug, minor\", 'Task', Chore\n" +
		"\t\t\tfield: sort\n" +
		"\t\t\t\ttype: int\n" +
		"\t\t\t\tui:\n" +
		"\t\t\t\t\tlist_type: sql\n" +
		"\t\t\t\t\tlist_source: `SELECT idx, title FROM lookups`\n" +
		"seed:\n" +
		"\ttable: lookup_values\n" +
		"\t\trow: idx=1, title=\"a, b\", description=\"say \\\"hi\\\"\"  ## comment\n" +
		"\t\trow: idx=2, title='x=y'\n"
	current := "\ntables:\n" +
		"\ttable: lookup_values\n" +
		"\t\taliases: [values, \"old values\"]\n" +
		"\t\tuniques:\n" +
		"\t\t\tunique: [title, sort]\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: title\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\t\tui:\n" +
		"\t\t\t\t\tlist_type: literal\n" +
		"\t\t\t\t\tlist_source: [\"Bug, minor\", 'Task', Chore]\n" +
		"\t\t\tfield: sort\n" +
		"\t\t\t\ttype: int\n" +
		"\t\t\t\tui:\n" +
		"\t\t\t\t\tlist_type: sql\n" +
		"\t\t\t\t\tlist_source: `SELECT idx, title FROM lookups`\n" +
		"seed:\n" +
		"\ttable: lookup_values\n" +
		"\t\trow: [idx=1, \"title=a, b\", 'description=say \"hi\"']  ## comment\n" +
		"\t\trow: [idx=2, title=x=y]\n"
	was, err := Parse([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	now, err := Parse([]byte(current))
	if err != nil {
		t.Fatal(err)
	}
	if was.Errors != 0 || now.Errors != 0 {
		t.Fatalf("errors: old %v, new %v", was.Warnings, now.Warnings)
	}
	if !reflect.DeepEqual(was.Tables, now.Tables) {
		t.Errorf("tables differ:\nold %+v\nnew %+v", was.Tables, now.Tables)
	}
	if !reflect.DeepEqual(was.Seeds, now.Seeds) {
		t.Errorf("seeds differ:\nold %+v\nnew %+v", was.Seeds, now.Seeds)
	}
	if len(now.Seeds) != 2 || now.Seeds[0].Fields["title"] != "a, b" || now.Seeds[0].Fields["description"] != `say "hi"` || now.Seeds[0].Line != 20 {
		t.Errorf("seed rows read wrong: %+v", now.Seeds)
	}
	if got := now.Tables[0].Fields[1].UI.ListSource; got != "`SELECT idx, title FROM lookups`" {
		t.Errorf("list_source = %q", got)
	}
}
