// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"strings"
	"testing"
)

const renameDDL = "tables:\n" +
	"\ttable: person  ## the people\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: age\n" +
	"\t\t\t\ttype: int\n" +
	"\ttable: pets\n" +
	"\t\taliases: animals\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n"

func TestRenameTable(t *testing.T) {
	out, found := RenameTable([]byte(renameDDL), "person", "people")
	if !found {
		t.Fatal("person not found")
	}
	text := string(out)
	if !strings.Contains(text, "\ttable: people  ## the people\n") {
		t.Fatalf("value not replaced (comment must survive):\n%s", text)
	}
	if !strings.Contains(text, "\t\taliases: person\n") {
		t.Fatalf("alias line missing:\n%s", text)
	}
	s, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tables[0].Name != "people" || len(s.Tables[0].Aliases) != 1 || s.Tables[0].Aliases[0] != "person" {
		t.Fatalf("parsed rename = %+v", s.Tables[0])
	}

	// an existing aliases: line extends instead of duplicating
	out, _ = RenameTable(out, "pets", "companions")
	s, err = Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Tables[1].Aliases, ","); got != "animals,pets" {
		t.Fatalf("pets aliases = %q", got)
	}

	if _, found := RenameTable([]byte(renameDDL), "nope", "x"); found {
		t.Fatal("unknown table must report not-found")
	}
}

func TestRenameField(t *testing.T) {
	// both tables have a "name" field; only person's may change
	out, found := RenameField([]byte(renameDDL), "person", "name", "full_name")
	if !found {
		t.Fatal("field not found")
	}
	s, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	f := s.Tables[0].Fields[0]
	if f.Name != "full_name" || len(f.Aliases) != 1 || f.Aliases[0] != "name" {
		t.Fatalf("renamed field = %+v", f)
	}
	if s.Tables[1].Fields[0].Name != "name" {
		t.Fatalf("pets.name must be untouched: %+v", s.Tables[1].Fields[0])
	}

	if _, found := RenameField([]byte(renameDDL), "pets", "age", "x"); found {
		t.Fatal("pets has no age field")
	}
}

// A rename re-emits the whole file, so anything the formatter mislays is lost
// from the schema the user wrote. Commenting a section's contents out while
// keeping the header is ordinary editing, and those comments have to come back
// inside their section - dedented, they would read as belonging to whatever
// follows.
func TestRenameKeepsCommentedOutSectionIntact(t *testing.T) {
	src := "tables:\n" +
		"\ttable: person\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: name\n" +
		"\t\t\t\ttype: string\n" +
		"\t\tfeatures:\n" +
		"\t\t\t# comments: yes\n" +
		"\t\t\t# audit_trail: yes\n" +
		"\ttable: pets\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: name\n" +
		"\t\t\t\ttype: string\n"

	out, found := RenameField([]byte(src), "person", "name", "full_name")
	if !found {
		t.Fatal("field not found")
	}
	text := string(out)
	for _, line := range []string{"\t\t\t# comments: yes\n", "\t\t\t# audit_trail: yes\n"} {
		if !strings.Contains(text, line) {
			t.Fatalf("commented-out feature left its section:\n%s", text)
		}
	}
	// Uncommenting them again has to give person the features, not pets.
	revived := strings.ReplaceAll(text, "# comments:", "comments:")
	s, err := Parse([]byte(revived))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Tables[0].Features.Comments {
		t.Fatalf("revived feature landed on the wrong table:\n%s", revived)
	}
}
