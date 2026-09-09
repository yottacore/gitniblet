// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Access enforcement tests - external package for the same reason as
// features_test.go (needs schema.Bootstrap).
package crud_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const aclDDL = "tables:\n" +
	"\ttable: docs\n" +
	"\t\taccess:\n" +
	"\t\t\twrite:\n" +
	"\t\t\t\twhitelist: editors, bosses\n" +
	"\t\t\tdelete:\n" +
	"\t\t\t\twhitelist: bosses\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: title\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: salary\n" +
	"\t\t\t\ttype: int\n" +
	"\t\t\t\taccess:\n" +
	"\t\t\t\t\twrite:\n" +
	"\t\t\t\t\t\twhitelist: bosses\n" +
	"\t\tfeatures:\n" +
	"\t\t\trow_level_access: y\n" +
	"\ttable: notes\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: body\n" +
	"\t\t\t\ttype: string\n"

func newACLAPI(t *testing.T) *crud.API {
	t.Helper()
	dir := t.TempDir()
	user, err := ddl.Parse([]byte(aclDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "v.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(user); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Bootstrap(st, user); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(filepath.Join(dir, "log"))
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	bs, err := schema.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	api.EnableFeatures(user, bs)
	return api
}

// mkUser creates a users row and its group memberships (groups made on
// demand), acting as an unrestricted setup user.
func mkUser(t *testing.T, api *crud.API, name string, groups ...string) {
	t.Helper()
	was := api.UserID
	api.UserID = "setup"
	defer func() { api.UserID = was }()
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

func wantDenied(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("%s: want access denied, got %v", what, err)
	}
}

func TestTableAndFieldAccess(t *testing.T) {
	api := newACLAPI(t)

	api.UserID = "alice" // no user row, no groups
	if _, err := api.Create("notes", map[string]string{"body": "hi"}); err != nil {
		t.Fatalf("no rules on notes, must stay open: %v", err)
	}
	_, err := api.Create("docs", map[string]string{"title": "x"})
	wantDenied(t, err, "create docs without membership")

	mkUser(t, api, "alice", "editors")
	id, err := api.Create("docs", map[string]string{"title": "plan"})
	if err != nil {
		t.Fatalf("editor create: %v", err)
	}
	if err := api.Update("docs", id, map[string]string{"title": "plan v2"}); err != nil {
		t.Fatalf("editor update: %v", err)
	}
	wantDenied(t, api.Update("docs", id, map[string]string{"salary": "9"}), "editor writing bosses-only field")
	wantDenied(t, api.MarkDelete("docs", id), "editor soft-delete")

	mkUser(t, api, "bob", "bosses")
	api.UserID = "bob"
	if err := api.Update("docs", id, map[string]string{"salary": "9"}); err != nil {
		t.Fatalf("boss salary write: %v", err)
	}
	if err := api.MarkDelete("docs", id); err != nil {
		t.Fatalf("boss delete: %v", err)
	}
}

func TestBuiltinGroupOverrides(t *testing.T) {
	api := newACLAPI(t)
	mkUser(t, api, "adm", "admins")
	mkUser(t, api, "own", "owners")

	api.UserID = "adm"
	id, err := api.Create("docs", map[string]string{"title": "a", "salary": "1"})
	if err != nil {
		t.Fatalf("admins always read/write: %v", err)
	}
	// delete is NOT part of the admin override; the rule (bosses) applies
	wantDenied(t, api.Delete("docs", id), "admin hard-delete")

	api.UserID = "own"
	if err := api.Delete("docs", id); err != nil {
		t.Fatalf("owners pass everything: %v", err)
	}
}

func TestRowLevelAccess(t *testing.T) {
	api := newACLAPI(t)
	mkUser(t, api, "alice", "editors", "hr")
	mkUser(t, api, "bob", "editors")

	api.UserID = "alice"
	granted, err := api.Create("docs", map[string]string{"title": "hr only"})
	if err != nil {
		t.Fatal(err)
	}
	open, err := api.Create("docs", map[string]string{"title": "anyone"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := api.Query(`SELECT "id" FROM "groups" WHERE "name"='hr'`)
	if err != nil || len(rows) == 0 {
		t.Fatalf("hr group: %v %v", rows, err)
	}
	if err := api.GrantRowAccess("docs", granted, rows[0]["id"]); err != nil {
		t.Fatal(err)
	}

	api.UserID = "bob"
	if err := api.Update("docs", open, map[string]string{"title": "fine"}); err != nil {
		t.Fatalf("ungranted row stays open: %v", err)
	}
	wantDenied(t, api.Update("docs", granted, map[string]string{"title": "nope"}), "bob writing an hr-granted row")

	api.UserID = "alice"
	if err := api.Update("docs", granted, map[string]string{"title": "ok"}); err != nil {
		t.Fatalf("hr member writes the granted row: %v", err)
	}
}
