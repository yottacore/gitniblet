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

// lookupShowDDL has one field per display setting, plus one whose
// list_source can't be a group.
const lookupShowDDL = `
tables:
	table: tasks
		fields:
			field: name
				type: string
			field: status
				type: int
				ui:
					list_type: lookup
					list_source: 1
			field: size
				type: int
				ui:
					list_type: lookup
					list_source: 2
					lookup_in_lists: title
					lookup_in_forms: symbol
			field: broken
				type: int
				ui:
					list_type: lookup
					list_source: nope
`

// lookupShowSetup gives a catalog over lookupShowDDL, with group 1 holding
// Open, an inactive Closed and a symbol-less Later, and group 2 holding Big.
func lookupShowSetup(t *testing.T) (*crud.API, *Catalog, map[string]string) {
	t.Helper()
	sch, err := ddl.Parse([]byte(lookupShowDDL))
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
	ids := map[string]string{}
	for _, row := range []map[string]string{
		{"idx": "1", "title": "Status"},
		{"idx": "2", "title": "Size"},
	} {
		if _, err := api.Create("lookups", row); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []map[string]string{
		{"idx": "1", "lookups_idx": "1", "title": "Open", "symbol": "🔘"},
		{"idx": "2", "lookups_idx": "1", "title": "Closed", "symbol": "✅", "is_active": "0"},
		{"idx": "3", "lookups_idx": "1", "title": "Later"},
		{"idx": "4", "lookups_idx": "2", "title": "Big", "symbol": "B"},
	} {
		id, err := api.Create("lookup_values", row)
		if err != nil {
			t.Fatal(err)
		}
		ids[row["title"]] = id
	}
	bs, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	return api, cat, ids
}

func TestLookupShow(t *testing.T) {
	api, cat, ids := lookupShowSetup(t)
	if err := api.MarkDelete("lookup_values", ids["Later"]); err != nil {
		t.Fatal(err)
	}
	show, err := cat.Lookups(api, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, val, list, form string }{
		{"status", "1", "🔘", "🔘 Open"},
		{"status", "2", "✅", "✅ Closed"},  // inactive still shows
		{"status", "3", "Later", "Later"}, // no symbol, and soft-deleted
		{"status", " 1 ", "🔘", "🔘 Open"},
		{"status", "4", "4", "4"}, // another group's value
		{"status", "99", "99", "99"},
		{"status", "", "", ""},
		{"size", "4", "Big", "B"},
		{"name", "1", "1", "1"},
		{"broken", "1", "1", "1"},
	} {
		if got := show.List(tc.field, tc.val); got != tc.list {
			t.Errorf("List(%s, %q) = %q, want %q", tc.field, tc.val, got, tc.list)
		}
		if got := show.Form(tc.field, tc.val); got != tc.form {
			t.Errorf("Form(%s, %q) = %q, want %q", tc.field, tc.val, got, tc.form)
		}
	}
	if !show.IsLookup("status") || show.IsLookup("name") || show.IsLookup("broken") {
		t.Error("IsLookup should hold for status and size only")
	}
	if !strings.Contains(strings.Join(cat.Warnings, "\n"), "tasks.broken") {
		t.Errorf("a non-idx list_source should warn, got %q", cat.Warnings)
	}

	// A symbol the front-end can't draw falls back to the title.
	show.SymbolOK = func(symbol string) bool { return symbol != "🔘" }
	if got := show.List("status", "1"); got != "Open" {
		t.Errorf("undrawable symbol in a list: got %q, want Open", got)
	}
	if got := show.Form("status", "1"); got != "Open" {
		t.Errorf("undrawable symbol in a form: got %q, want Open", got)
	}
	if got := show.Form("size", "4"); got != "B" {
		t.Errorf("drawable symbol in a symbol-only form: got %q, want B", got)
	}

	var none *LookupShow
	if none.List("status", "1") != "1" || none.Form("status", "1") != "1" || none.IsLookup("status") {
		t.Error("a nil LookupShow should show values as stored")
	}
	plain, err := cat.Lookups(api, "lookups")
	if err != nil || plain.List("title", "Status") != "Status" {
		t.Errorf("a table with no lookup fields: %v, %q", err, plain.List("title", "Status"))
	}
}

// A form picks from the group's active values in sort order, and keeps a
// retired value the row already holds.
func TestLookupChoices(t *testing.T) {
	api, cat, ids := lookupShowSetup(t)
	if _, err := api.Create("lookup_values", map[string]string{"idx": "5", "lookups_idx": "1", "title": "First", "sort": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := api.SetField("lookup_values", ids["Later"], "sort", "2.5"); err != nil {
		t.Fatal(err)
	}
	show, err := cat.Lookups(api, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	picked := func(choices []LookupChoice) (vals []string, sel string) {
		sel = "none"
		for _, choice := range choices {
			vals = append(vals, choice.Value+"="+choice.Label)
			if choice.Selected {
				sel = choice.Value
			}
		}
		return vals, sel
	}
	for _, tc := range []struct{ field, cur, want, sel string }{
		{"status", "", "=|5=First|3=Later|1=🔘 Open", ""},
		{"status", "1", "=|5=First|3=Later|1=🔘 Open", "1"},
		{"status", " 3 ", "=|5=First|3=Later|1=🔘 Open", "3"},
		{"status", "2", "=|2=✅ Closed|5=First|3=Later|1=🔘 Open", "2"}, // inactive, held
		{"status", "99", "=|99=99|5=First|3=Later|1=🔘 Open", "99"},
		{"size", "4", "=|4=B", "4"},
	} {
		vals, sel := picked(show.Choices(tc.field, tc.cur))
		if got := strings.Join(vals, "|"); got != tc.want || sel != tc.sel {
			t.Errorf("Choices(%s, %q) = %s selecting %q, want %s selecting %q", tc.field, tc.cur, got, sel, tc.want, tc.sel)
		}
	}
	if show.Choices("name", "1") != nil || show.Choices("broken", "1") != nil {
		t.Error("only lookup fields get choices")
	}

	if err := api.MarkDelete("lookup_values", ids["Later"]); err != nil {
		t.Fatal(err)
	}
	if show, err = cat.Lookups(api, "tasks"); err != nil {
		t.Fatal(err)
	}
	if vals, _ := picked(show.Choices("status", "")); strings.Join(vals, "|") != "=|5=First|1=🔘 Open" {
		t.Errorf("a deleted value is still offered: %v", vals)
	}
}
