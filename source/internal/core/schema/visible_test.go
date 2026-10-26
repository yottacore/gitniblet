// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yottacore/gitniblet/internal/core/crud"
	"github.com/yottacore/gitniblet/internal/core/ddl"
	"github.com/yottacore/gitniblet/internal/core/store"
	"github.com/yottacore/gitniblet/internal/core/txlog"
)

const visibleDDL = `
tables:
	table: gadgets
		fields:
			field: id
				ui:
					visible_list: n
			field: date_created
				ui:
					visible_list: y
			field: name
				type: string
			field: secret_sauce
				type: string
				ui:
					visible_list: no
			field: notes
				type: string
				ui:
					visible_form: no
			field: hidden_everywhere
				type: string
				ui:
					visible_form: n
					visible_list: n
	table: plain
		fields:
			field: label
				type: string
	table: ghosts
		fields:
			field: id
				ui:
					visible_list: n
			field: boo
				type: string
				ui:
					visible_list: n
`

func catalogFor(t *testing.T, src string) *Catalog {
	t.Helper()
	sch, err := ddl.Parse([]byte(src))
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
	api.UserID = "test"
	bs, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// visible_list and visible_form trim lists and forms only. Fields, which
// gates what a save may write, keeps every editable field.
// ID: dec83
func TestFieldVisibility(t *testing.T) {
	cat := catalogFor(t, visibleDDL)

	cases := []struct{ what, got, want string }{
		{"gadgets list", strings.Join(cat.ColumnsFor("gadgets"), ","), "name,notes,date_created"},
		{"gadgets form", strings.Join(cat.FormFields("gadgets"), ","), "name,secret_sauce"},
		{"gadgets fields", strings.Join(cat.Fields["gadgets"], ","), "name,secret_sauce,notes,hidden_everywhere"},
		{"plain list", strings.Join(cat.ColumnsFor("plain"), ","), "id,label"},
		{"plain form", strings.Join(cat.FormFields("plain"), ","), "label"},
		// everything hidden still leaves a column to pick a row by
		{"ghosts list", strings.Join(cat.ColumnsFor("ghosts"), ","), "id"},
		// no system columns at all, so nothing to opt in
		{"audit_trail list", strings.Join(cat.ColumnsFor("audit_trail"), ","), "id," + strings.Join(cat.Fields["audit_trail"], ",")},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.what, c.got, c.want)
		}
	}
}
