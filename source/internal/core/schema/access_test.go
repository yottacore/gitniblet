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

const aclDDL = `
tables:
	table: secrets
		access:
			read:
				whitelist: spies
		fields:
			field: code
				type: string
	table: people
		fields:
			field: name
				type: string
			field: ssn
				type: string
				access:
					read:
						whitelist: hr
		features:
			row_level_access: y

views:
	view: "open"
		layout:
			block: "b1"
				table: people
				type: grid
	view: "classified"
		access:
			whitelist: spies
		layout:
			block: "b2"
				table: people
				type: grid
	view: "over-secrets"
		layout:
			block: "b3"
				table: secrets
				type: grid
`

// aclSetup opens the ACL schema and returns a fresh catalog builder bound to
// the current api.UserID - access filtering happens at catalog build.
func aclSetup(t *testing.T) (*crud.API, func() *Catalog) {
	t.Helper()
	sch, err := ddl.Parse([]byte(aclDDL))
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
	api.UserID = "setup"
	bs, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	api.EnableFeatures(sch, bs)
	return api, func() *Catalog {
		cat, err := NewCatalog(api, sch, bs)
		if err != nil {
			t.Fatal(err)
		}
		return cat
	}
}

func addUser(t *testing.T, api *crud.API, name string, groups ...string) {
	t.Helper()
	uid, err := api.Create("users", map[string]string{"screen_name": name})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		rows, err := api.Query(`SELECT "id" FROM "groups" WHERE "name"=? AND "is_deleted"=0`, g)
		if err != nil {
			t.Fatal(err)
		}
		var gid string
		if len(rows) > 0 {
			gid = rows[0]["id"]
		} else if gid, err = api.Create("groups", map[string]string{"name": g}); err != nil {
			t.Fatal(err)
		}
		if _, err := api.Link("users", uid, "groups", gid); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCatalogHidesDeniedReads(t *testing.T) {
	api, build := aclSetup(t)
	addUser(t, api, "carol") // no memberships
	addUser(t, api, "dave", "spies")

	api.UserID = "carol"
	cat := build()
	if cat.Has("secrets") {
		t.Error("secrets must be hidden from non-spies")
	}
	fields := strings.Join(cat.Fields["people"], ",")
	if strings.Contains(fields, "ssn") {
		t.Errorf("ssn must be hidden from non-hr: %s", fields)
	}
	if cat.View("classified") != nil {
		t.Error("classified view must be hidden (flat whitelist)")
	}
	if cat.View("over-secrets") != nil {
		t.Error("a view whose main table is unreadable must be hidden")
	}
	if cat.View("open") == nil || cat.DefaultView != "open" {
		t.Errorf("open view should remain and be the default, got %q", cat.DefaultView)
	}
	joined := strings.Join(cat.Warnings, "\n")
	for _, want := range []string{`table "secrets" hidden`, `view "classified" hidden`, `view "over-secrets" hidden`} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}

	api.UserID = "dave"
	cat = build()
	if !cat.Has("secrets") || cat.View("classified") == nil || cat.View("over-secrets") == nil {
		t.Error("spies should see everything spy-gated")
	}
}

func TestLiveRowsHidesUngrantedRows(t *testing.T) {
	api, build := aclSetup(t)
	addUser(t, api, "carol", "staff")
	addUser(t, api, "harry", "hr")

	open, err := api.Create("people", map[string]string{"name": "visible"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := api.Create("people", map[string]string{"name": "hr-only"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := api.Query(`SELECT "id" FROM "groups" WHERE "name"='hr'`)
	if err != nil || len(rows) == 0 {
		t.Fatalf("hr group: %v %v", rows, err)
	}
	if err := api.GrantRowAccess("people", hidden, rows[0]["id"]); err != nil {
		t.Fatal(err)
	}

	api.UserID = "carol"
	cat := build()
	got, err := cat.LiveRows(api, "people")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["id"] != open {
		t.Fatalf("carol should see only the ungranted row: %+v", got)
	}

	api.UserID = "harry"
	got, err = build().LiveRows(api, "people")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("hr should see both rows: %+v", got)
	}
}
