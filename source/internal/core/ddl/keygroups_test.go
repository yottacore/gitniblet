// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"strings"
	"testing"
)

// keyGroupDDL is a two-field table whose uniques:/indexes: sections are filled
// in by the caller.
func keyGroupDDL(sections string) string {
	return "tables:\n" +
		"\ttable: person\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: name\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\tfield: email\n" +
		"\t\t\t\ttype: string\n" +
		sections
}

// A misspelled field name used to be kept and handed to SQLite, so a database
// the picker listed as fine failed to open with a raw SQL error.
func TestUniqueOnAnUnknownFieldIsAnError(t *testing.T) {
	schema, err := Parse([]byte(keyGroupDDL("\t\tuniques:\n\t\t\tunique: name, emial\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !schema.HasErrors() {
		t.Error("an unresolvable name must count as an error, so the picker can say so")
	}
	if got := schema.Tables[0].Uniques; len(got) != 0 {
		t.Errorf("uniques = %v, want the group dropped rather than passed to SQLite", got)
	}
	if !strings.Contains(strings.Join(schema.Warnings, "\n"), "emial") {
		t.Errorf("the warning should name the offending field: %v", schema.Warnings)
	}
}

// The whole group goes, not just the bad name: pruning `unique: name, emial`
// down to `unique: name` would enforce a stricter rule than was written.
func TestABadNameDropsTheWholeUniqueGroup(t *testing.T) {
	schema, err := Parse([]byte(keyGroupDDL(
		"\t\tuniques:\n\t\t\tunique: name, emial\n\t\t\tunique: email\n")))
	if err != nil {
		t.Fatal(err)
	}
	groups := schema.Tables[0].Uniques
	if len(groups) != 1 {
		t.Fatalf("uniques = %v, want only the sound group to survive", groups)
	}
	if len(groups[0]) != 1 || groups[0][0] != "email" {
		t.Errorf("surviving group = %v, want [email]", groups[0])
	}
}

func TestIndexOnAnUnknownFieldIsAnError(t *testing.T) {
	schema, err := Parse([]byte(keyGroupDDL("\t\tindexes:\n\t\t\tindex: nmae\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !schema.HasErrors() {
		t.Error("an unresolvable index name must count as an error")
	}
	if got := schema.Tables[0].Indexes; len(got) != 0 {
		t.Errorf("indexes = %v, want the group dropped", got)
	}
}

// The control: sound groups survive untouched and raise nothing.
func TestSoundKeyGroupsSurvive(t *testing.T) {
	schema, err := Parse([]byte(keyGroupDDL(
		"\t\tuniques:\n\t\t\tunique: name, email\n\t\tindexes:\n\t\t\tindex: email\n")))
	if err != nil {
		t.Fatal(err)
	}
	if schema.HasErrors() {
		t.Errorf("a sound schema reported errors: %v", schema.Warnings)
	}
	if got := schema.Tables[0].Uniques; len(got) != 1 || len(got[0]) != 2 {
		t.Errorf("uniques = %v, want one two-field group", got)
	}
	if got := schema.Tables[0].Indexes; len(got) != 1 {
		t.Errorf("indexes = %v, want one group", got)
	}
}

// A system column is a real column, so naming one is not a typo.
func TestKeyGroupsMayNameASystemField(t *testing.T) {
	schema, err := Parse([]byte(keyGroupDDL("\t\tindexes:\n\t\t\tindex: date_created\n")))
	if err != nil {
		t.Fatal(err)
	}
	if schema.HasErrors() {
		t.Errorf("indexing a system column reported errors: %v", schema.Warnings)
	}
	if got := schema.Tables[0].Indexes; len(got) != 1 {
		t.Errorf("indexes = %v, want the group kept", got)
	}
}
