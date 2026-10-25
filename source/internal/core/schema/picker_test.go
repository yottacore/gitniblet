// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const pickerDDL = `
tables:
	table: tasks
		fields:
			field: kind
				type: string
				ui:
					list_type: literal
					list_source: "Bug, minor", 'Task', Chore
			field: owner
				type: int
				ui:
					list_type: sql
					list_source: ` + "`SELECT idx, title FROM lookups WHERE is_active ORDER BY title`" + `
			field: tag
				type: string
				ui:
					list_type: sql
					list_source: ` + "`SELECT title FROM lookups ORDER BY idx`" + `
			field: note
				type: string
	table: broken
		fields:
			field: what
				type: string
				ui:
					list_type: sql
					list_source: ` + "`SELECT nope FROM nowhere`" + `
			field: empty
				type: string
				ui:
					list_type: literal
`

func pickerSetup(t *testing.T) (*crud.API, *Catalog) {
	t.Helper()
	sch, err := ddl.Parse([]byte(pickerDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	if _, err := Bootstrap(st, sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	for _, row := range []map[string]string{
		{"idx": "1", "title": "Status"},
		{"idx": "2", "title": "Size"},
		{"idx": "3", "title": "Gone", "is_active": "0"},
	} {
		if _, err := api.Create("lookups", row); err != nil {
			t.Fatal(err)
		}
	}
	bs, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	return api, cat
}

func choiceText(choices []LookupChoice) string {
	var parts []string
	for _, choice := range choices {
		part := choice.Value + "=" + choice.Label
		if choice.Selected {
			part = "*" + part
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "|")
}

// literal and sql fields pick from their list_source, and keep a value the
// row holds that the list doesn't.
// ID: decb3
func TestPickerChoices(t *testing.T) {
	api, cat := pickerSetup(t)
	show, err := cat.FormChoices(api, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, cur, want string }{
		{"kind", "", "*=|Bug, minor=Bug, minor|Task=Task|Chore=Chore"},
		{"kind", "Task", "=|Bug, minor=Bug, minor|*Task=Task|Chore=Chore"},
		{"kind", "Epic", "=|*Epic=Epic|Bug, minor=Bug, minor|Task=Task|Chore=Chore"},
		{"owner", "1", "=|2=Size|*1=Status"},
		{"owner", "3", "=|*3=3|2=Size|1=Status"}, // an inactive group, held
		{"tag", "", "*=|Status=Status|Size=Size|Gone=Gone"},
	} {
		if got := choiceText(show.Choices(tc.field, tc.cur)); got != tc.want {
			t.Errorf("Choices(%s, %q) = %s, want %s", tc.field, tc.cur, got, tc.want)
		}
	}
	if show.HasPicker("note") || show.Choices("note", "x") != nil || !show.HasPicker("kind") {
		t.Error("only list_type fields get a picker")
	}
	if show.IsLookup("kind") || show.List("owner", "1") != "1" {
		t.Error("a literal or sql field still lists as stored")
	}

	// Lists don't run the queries.
	if list, err := cat.Lookups(api, "tasks"); err != nil || list.HasPicker("owner") {
		t.Errorf("Lookups loaded the sql picker: %v", err)
	}

	if _, err := cat.FormChoices(api, "broken"); err == nil || !strings.Contains(err.Error(), "broken.what") {
		t.Errorf("a bad list_source query should say which field: %v", err)
	}
	if !strings.Contains(strings.Join(cat.Warnings, "\n"), "broken.empty") {
		t.Errorf("a list_type with no list_source should warn, got %q", cat.Warnings)
	}

	// The built-in lookup_values.lookups_idx picks its group the same way.
	show, err = cat.FormChoices(api, "lookup_values")
	if err != nil {
		t.Fatal(err)
	}
	if got := choiceText(show.Choices("lookups_idx", "")); got != "*=|2=Size|1=Status" {
		t.Errorf("lookups_idx choices = %s", got)
	}
}
