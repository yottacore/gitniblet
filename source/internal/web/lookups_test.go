// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
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

// A lookup field shows its symbol in lists and "symbol title" in forms, while
// the form input keeps the stored idx.
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
	if !strings.Contains(body, `name="status" value="7"`) || !strings.Contains(body, "<span>🔘 Open</span>") {
		t.Errorf("edit form should keep the idx and label it:\n%s", body)
	}
	if strings.Count(body, "<span>") != 1 {
		t.Errorf("only the lookup field gets a label:\n%s", body)
	}
	if body := do(t, h, "GET", "/t/tasks/new", nil).Body.String(); strings.Contains(body, "<span>") {
		t.Errorf("a new row has nothing to label:\n%s", body)
	}
}
