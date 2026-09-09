// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// External test package: the feature behaviors need the built-in tables,
// and schema (which owns them) imports crud - so the harness lives out here
// where it can import both.
package crud_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const featDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: age\n" +
	"\t\t\t\ttype: int\n" +
	"\t\tfeatures:\n" +
	"\t\t\tcomments: y\n" +
	"\t\t\taudit_trail: y\n" +
	"\t\t\turi_attachments: y\n" +
	"\t\t\tlocal_attachments: y\n" +
	"\t\t\trow_level_access: y\n" +
	"\ttable: plain\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n"

func newFeatAPI(t *testing.T) *crud.API {
	t.Helper()
	dir := t.TempDir()
	user, err := ddl.Parse([]byte(featDDL))
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
	api.UserID = "u1"
	bs, err := schema.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	api.EnableFeatures(user, bs)
	return api
}

func auditRows(t *testing.T, api *crud.API, table, id string) []map[string]string {
	t.Helper()
	// parent_id is a ref column, so the id crosses to raw bytes via id() -
	// the same way a hand-written query would have to.
	rows, err := api.Query(`SELECT * FROM "audit_trail" WHERE "table_name"=? AND "parent_id"=id(?) ORDER BY "date"`, table, id)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestAuditCaptureAndCoalescing(t *testing.T) {
	api := newFeatAPI(t)
	id, err := api.Create("person", map[string]string{"name": "Ann", "age": "30"})
	if err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, api, "person", id)
	if len(rows) != 1 || rows[0]["action"] != "create" || rows[0]["values"] != "" {
		t.Fatalf("after create: %+v", rows)
	}

	// two updates by the same user coalesce into ONE record holding the
	// OLDEST value of each touched field
	if err := api.Update("person", id, map[string]string{"name": "Anne"}); err != nil {
		t.Fatal(err)
	}
	if err := api.SetField("person", id, "age", "31"); err != nil {
		t.Fatal(err)
	}
	if err := api.Update("person", id, map[string]string{"name": "Annette"}); err != nil {
		t.Fatal(err)
	}
	rows = auditRows(t, api, "person", id)
	if len(rows) != 2 {
		t.Fatalf("update should coalesce, got %d records: %+v", len(rows), rows)
	}
	upd := rows[1]
	if upd["action"] != "update" || !strings.Contains(upd["values"], `name:"Ann"`) ||
		!strings.Contains(upd["values"], `age:"30"`) || strings.Contains(upd["values"], "Anne") {
		t.Fatalf("coalesced update record = %+v", upd)
	}

	// a different user's update starts a fresh record
	api.UserID = "u2"
	if err := api.Update("person", id, map[string]string{"name": "Anna"}); err != nil {
		t.Fatal(err)
	}
	rows = auditRows(t, api, "person", id)
	if len(rows) != 3 || rows[2]["user_id"] != "u2" || !strings.Contains(rows[2]["values"], `name:"Annette"`) {
		t.Fatalf("other-user update = %+v", rows)
	}

	// delete snapshots what the row held
	if err := api.MarkDelete("person", id); err != nil {
		t.Fatal(err)
	}
	if err := api.Delete("person", id); err != nil {
		t.Fatal(err)
	}
	rows = auditRows(t, api, "person", id)
	last := rows[len(rows)-1]
	if last["action"] != "delete" || !strings.Contains(last["values"], `name:"Anna"`) ||
		!strings.Contains(last["values"], `age:"31"`) {
		t.Fatalf("delete snapshot = %+v", last)
	}
	if rows[len(rows)-2]["action"] != "mark_delete" {
		t.Fatalf("expected mark_delete before delete: %+v", rows)
	}
}

func TestAuditValuesQuoting(t *testing.T) {
	api := newFeatAPI(t)
	id, _ := api.Create("person", map[string]string{"name": `say "hi" now`})
	if err := api.Update("person", id, map[string]string{"name": "quiet"}); err != nil {
		t.Fatal(err)
	}
	rows := auditRows(t, api, "person", id)
	// doubled-quote (CSV-style) escaping keeps the value unambiguous
	if want := `name:"say ""hi"" now"`; rows[len(rows)-1]["values"] != want {
		t.Fatalf("values = %q, want %q", rows[len(rows)-1]["values"], want)
	}
}

func TestCommentsGatedAndOrdered(t *testing.T) {
	api := newFeatAPI(t)
	id, _ := api.Create("person", map[string]string{"name": "Ann"})
	if _, err := api.CommentAdd("person", id, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.CommentAdd("person", id, "second"); err != nil {
		t.Fatal(err)
	}
	cs, err := api.CommentsFor("person", id)
	if err != nil || len(cs) != 2 || cs[0]["comment"] != "first" || cs[1]["comment"] != "second" {
		t.Fatalf("comments = %+v err=%v", cs, err)
	}

	pid, _ := api.Create("plain", map[string]string{"name": "x"})
	if _, err := api.CommentAdd("plain", pid, "nope"); err == nil {
		t.Fatal("comment on a non-opted-in table should fail")
	}
}

func TestAttachments(t *testing.T) {
	api := newFeatAPI(t)
	id, _ := api.Create("person", map[string]string{"name": "Ann"})

	if _, err := api.AttachURI("person", id, "https://example.com/spec", "the spec"); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(src, []byte("jpeg bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	attID, err := api.AttachFile("person", id, src, "a photo")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(api.AttachmentsDir(), attID))
	if err != nil || string(stored) != "jpeg bytes" {
		t.Fatalf("stored copy: %q err=%v", stored, err)
	}

	atts, err := api.AttachmentsFor("person", id)
	if err != nil || len(atts) != 2 {
		t.Fatalf("attachments = %+v err=%v", atts, err)
	}
	if atts[0].Kind != "uri" || atts[0].Label != "https://example.com/spec" ||
		atts[1].Kind != "copy" || atts[1].Label != "photo.jpg" {
		t.Fatalf("attachments = %+v", atts)
	}

	work, err := api.AttachmentWorkingCopy(attID)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(work) // lands in the real user tmp dir by design
	got, err := os.ReadFile(work)
	if err != nil || string(got) != "jpeg bytes" {
		t.Fatalf("working copy: %q err=%v", got, err)
	}
	if base := filepath.Base(work); !strings.HasPrefix(base, "photo_") || !strings.HasSuffix(base, ".jpg") {
		t.Fatalf("working copy name = %q, want photo_<stamp>.jpg", base)
	}

	pid, _ := api.Create("plain", map[string]string{"name": "x"})
	if _, err := api.AttachURI("plain", pid, "u", ""); err == nil {
		t.Fatal("attach on a non-opted-in table should fail")
	}
}

func TestRowAccessGrants(t *testing.T) {
	api := newFeatAPI(t)
	id, _ := api.Create("person", map[string]string{"name": "Ann"})
	gid, err := api.Create("groups", map[string]string{"name": "editors"})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.GrantRowAccess("person", id, gid); err != nil {
		t.Fatal(err)
	}
	if err := api.GrantRowAccess("person", id, gid); err != nil { // idempotent
		t.Fatal(err)
	}
	gs, err := api.RowAccessGroups("person", id)
	if err != nil || len(gs) != 1 || gs[0] != gid {
		t.Fatalf("grants = %v err=%v", gs, err)
	}
	other, _ := api.Create("person", map[string]string{"name": "Bob"})
	if none, err := api.RowAccessGroups("person", other); err != nil || none != nil {
		t.Fatalf("ungranted row should have no groups: %v err=%v", none, err)
	}
	// A malformed id is an error, not an empty result - a ref column compared
	// against the wrong shape would otherwise just look like "no grants".
	if _, err := api.RowAccessGroups("person", "00ff"); err == nil {
		t.Fatal("malformed id was accepted")
	}
}
