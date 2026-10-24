// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/url"
	"strings"
	"testing"
)

const lookupDDL = `
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

views:
	view: "board"
		layout:
			block: "all"
				table: tasks
				type: grid
			block: "one"
				table: tasks
				location: all, right, 50%
				type: form
`

// A lookup field shows its symbol in lists and "symbol title" in forms, and
// the edit form picks it from the group.
func TestLookupsShowInWeb(t *testing.T) {
	s, api := newTestServerFor(t, lookupDDL)
	h := s.routes()
	if _, err := api.Create("lookups", map[string]string{"idx": "1", "title": "Status"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("lookup_values", map[string]string{"idx": "7", "lookups_idx": "1", "title": "Open", "symbol": "🔘"}); err != nil {
		t.Fatal(err)
	}
	id, err := api.Create("tasks", map[string]string{"name": "t1", "status": "7"})
	if err != nil {
		t.Fatal(err)
	}

	if body := do(t, h, "GET", "/t/tasks", nil).Body.String(); !strings.Contains(body, "<td>🔘</td>") {
		t.Errorf("table list should show the symbol:\n%s", body)
	}
	if body := do(t, h, "GET", "/v/board/b/0/rows", nil).Body.String(); !strings.Contains(body, "<td>🔘</td>") {
		t.Errorf("grid block should show the symbol:\n%s", body)
	}
	if body := do(t, h, "GET", "/v/board/b/1/rows", nil).Body.String(); !strings.Contains(body, "<td>🔘 Open</td>") {
		t.Errorf("form block should show symbol then title:\n%s", body)
	}
	body := do(t, h, "GET", "/t/tasks/"+id+"/edit", nil).Body.String()
	if !strings.Contains(body, `<option value="7" selected>🔘 Open</option>`) {
		t.Errorf("edit form should pick the stored value:\n%s", body)
	}
	if strings.Count(body, "<select") != 1 {
		t.Errorf("only the lookup field gets a picker:\n%s", body)
	}
	if body := do(t, h, "GET", "/t/tasks/new", nil).Body.String(); !strings.Contains(body, `<option value="" selected></option><option value="7">🔘 Open</option>`) {
		t.Errorf("a new row starts with nothing picked:\n%s", body)
	}
}

// The picker offers only active values, keeps a retired one the row holds
// across a save, and a posted value outside the group is refused.
func TestLookupPickerInWeb(t *testing.T) {
	s, api := newTestServerFor(t, lookupDDL)
	h := s.routes()
	if _, err := api.Create("lookups", map[string]string{"idx": "1", "title": "Status"}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []map[string]string{
		{"idx": "7", "lookups_idx": "1", "title": "Open", "sort": "2"},
		{"idx": "8", "lookups_idx": "1", "title": "Done", "sort": "1"},
		{"idx": "9", "lookups_idx": "1", "title": "Old"},
	} {
		if _, err := api.Create("lookup_values", row); err != nil {
			t.Fatal(err)
		}
	}
	id, err := api.Create("tasks", map[string]string{"name": "t1", "status": "9"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := api.Query(`SELECT idtext("id") AS id FROM lookup_values WHERE idx=9`)
	if err != nil || len(old) != 1 {
		t.Fatal(old, err)
	}
	if err := api.SetField("lookup_values", old[0]["id"], "is_active", "0"); err != nil {
		t.Fatal(err)
	}

	body := do(t, h, "GET", "/t/tasks/"+id+"/edit", nil).Body.String()
	if !strings.Contains(body, `<option value=""></option><option value="9" selected>Old</option><option value="8">Done</option><option value="7">Open</option>`) {
		t.Errorf("want none, the held value, then active values by sort:\n%s", body)
	}
	if body := do(t, h, "GET", "/t/tasks/new", nil).Body.String(); strings.Contains(body, ">Old<") {
		t.Errorf("a new row is offered an inactive value:\n%s", body)
	}

	status := func() string {
		t.Helper()
		row, _, err := api.Get("tasks", id)
		if err != nil {
			t.Fatal(err)
		}
		return row["status"]
	}
	if w := do(t, h, "POST", "/t/tasks/"+id, url.Values{"name": {"t2"}, "status": {"9"}}); w.Code != 200 || status() != "9" {
		t.Errorf("saving with the held value: code %d, status %q", w.Code, status())
	}
	if w := do(t, h, "POST", "/t/tasks/"+id, url.Values{"name": {"t2"}, "status": {"8"}}); w.Code != 200 || status() != "8" {
		t.Errorf("picking Done: code %d, status %q", w.Code, status())
	}
	if w := do(t, h, "POST", "/t/tasks/"+id, url.Values{"name": {"t2"}, "status": {"9"}}); w.Code == 200 || status() != "8" {
		t.Errorf("picking the inactive value back: code %d, status %q", w.Code, status())
	}
}
