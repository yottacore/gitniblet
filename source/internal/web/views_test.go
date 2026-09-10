// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const orgDDL = `
tables:
	table: employees
		fields:
			field: name
				type: string
			field: boss_id
				type: string

views:
	view: "org"
		layout:
			block: "chain"
				table: employees
				type: tree_grid
				parent_field: boss_id
			block: "flat"
				table: employees
				location: chain, right, 50%
				type: grid
				readonly: yes
`

func newViewServer(t *testing.T) (*server, *crud.API) {
	t.Helper()
	sch, err := ddl.Parse([]byte(orgDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "org.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Bootstrap(st, sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	api.UserID = "test"
	bs, err := schema.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := schema.NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(api, cat)
	if err != nil {
		t.Fatal(err)
	}
	return s, api
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestViewListedAndPageRenders(t *testing.T) {
	s, _ := newViewServer(t)
	h := s.routes()

	rec := get(t, h, "/")
	if !strings.Contains(rec.Body.String(), `hx-get="/v/org"`) {
		t.Errorf("sidebar should list the view:\n%s", rec.Body.String())
	}
	// the default view ("org", first defined) auto-loads into #main
	if !strings.Contains(rec.Body.String(), `hx-trigger="load"`) {
		t.Errorf("index should auto-load the default view:\n%s", rec.Body.String())
	}

	rec = get(t, h, "/v/org")
	if rec.Code != http.StatusOK {
		t.Fatalf("view page = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"chain: employees (tree_grid)",
		"flat: employees (grid, ro)",
		`hx-get="/v/org/b/0/rows"`,
		`hx-get="/v/org/b/1/rows"`,
		"flex-direction:row", // the right-hint split
	} {
		if !strings.Contains(body, want) {
			t.Errorf("view page missing %q:\n%s", want, body)
		}
	}

	if rec = get(t, h, "/v/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown view = %d, want 404", rec.Code)
	}
	if rec = get(t, h, "/v/org/b/9/rows"); rec.Code != http.StatusNotFound {
		t.Errorf("out-of-range block = %d, want 404", rec.Code)
	}
}

func TestViewBlockRows(t *testing.T) {
	s, api := newViewServer(t)
	h := s.routes()
	boss, err := api.Create("employees", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee", "boss_id": boss}); err != nil {
		t.Fatal(err)
	}

	// block 0: the tree_grid - Bee indented one level under Ann, editable
	body := get(t, h, "/v/org/b/0/rows").Body.String()
	if !strings.Contains(body, "padding-left:1em") {
		t.Errorf("child row should be indented:\n%s", body)
	}
	if strings.Index(body, "Ann") > strings.Index(body, "Bee") {
		t.Errorf("parent must render before child:\n%s", body)
	}
	if !strings.Contains(body, "/edit") {
		t.Errorf("writable block should offer edit:\n%s", body)
	}

	// block 1: the readonly flat grid - no edit affordance
	body = get(t, h, "/v/org/b/1/rows").Body.String()
	if !strings.Contains(body, "Ann") || !strings.Contains(body, "Bee") {
		t.Errorf("flat grid should list both rows:\n%s", body)
	}
	if strings.Contains(body, "/edit") {
		t.Errorf("readonly block must not offer edit:\n%s", body)
	}
}

// Named queries: the view page offers the dropdown, /v/{view}/q runs one,
// and a resolvable startup_named_query auto-loads on open.
func TestViewNamedQueries(t *testing.T) {
	s, api := newViewServer(t)
	if _, err := api.Create("employees", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	s.cat.Queries = []ddl.NamedQuery{
		{Name: "Just Ann", View: "org", Active: true,
			SQL: `SELECT "name" FROM "employees" WHERE "name"='Ann' AND "is_deleted"=0`},
		{Name: "Inactive", View: "org", SQL: `SELECT 1`},
	}
	h := s.routes()

	rec := get(t, h, "/v/org")
	body := rec.Body.String()
	if !strings.Contains(body, "<option>Just Ann</option>") {
		t.Fatalf("view page missing query dropdown:\n%s", body)
	}
	if strings.Contains(body, "Inactive") {
		t.Fatal("inactive queries must not be offered")
	}
	if strings.Contains(body, "hx-trigger=\"load\" hx-target=\"#vb-0\"") {
		t.Fatal("no startup query configured, nothing should auto-load")
	}

	rec = get(t, h, "/v/org/q?name=Just+Ann")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ann") {
		t.Fatalf("query run = %d %s", rec.Code, rec.Body)
	}
	if rec := get(t, h, "/v/org/q?name=nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown query = %d", rec.Code)
	}
	if rec := get(t, h, "/v/org/q?name=Inactive"); rec.Code != http.StatusNotFound {
		t.Fatalf("inactive query = %d", rec.Code)
	}

	// startup_named_query resolvable -> auto-load div appears
	// (html/template renders the urlquery-escaped '+' as &#43; in attributes)
	s.cat.Views[0].StartupNamedQuery = "Just Ann"
	body = get(t, h, "/v/org").Body.String()
	if !strings.Contains(body, `hx-trigger="load" hx-target="#vb-0"`) ||
		!strings.Contains(body, "name=Just&#43;Ann") {
		t.Fatalf("startup query should auto-load:\n%s", body)
	}
}

const commentsDDL = `
tables:
	table: task
		fields:
			field: title
				type: string
			field: parent_task
				type: string
		features:
			comments: yes

views:
	view: "board"
		layout:
			block: 1
				table: task
				type: tree_grid
				parent_field: parent_task
			block: 2
				table: task
				type: comments
				location: 1, below, 35%
	default_view: "board"
`

func newCommentsServer(t *testing.T) (*server, *crud.API) {
	t.Helper()
	sch, err := ddl.Parse([]byte(commentsDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Build(sch); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Bootstrap(st, sch); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := crud.New(st, lg)
	api.UserID = "test"
	bs, err := schema.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	api.EnableFeatures(sch, bs)
	cat, err := schema.NewCatalog(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(api, cat)
	if err != nil {
		t.Fatal(err)
	}
	return s, api
}

func TestViewCommentsBlock(t *testing.T) {
	s, api := newCommentsServer(t)
	h := s.routes()
	id, err := api.Create("task", map[string]string{"title": "Fix bug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.CommentAdd("task", id, "first note"); err != nil {
		t.Fatal(err)
	}

	// the tree_grid rows carry a link that loads a row's thread into the pane
	rows := get(t, h, "/v/board/b/0/rows").Body.String()
	if !strings.Contains(rows, "/v/board/b/1/comments?id="+id) {
		t.Errorf("list row missing its comments link:\n%s", rows)
	}
	if !strings.Contains(rows, `hx-target="#vb-1"`) {
		t.Errorf("comments link should target the pane:\n%s", rows)
	}

	// the comments block itself is empty until a row is picked
	pane := get(t, h, "/v/board/b/1/rows").Body.String()
	if !strings.Contains(pane, "Select a row") {
		t.Errorf("empty comments pane should prompt for a selection:\n%s", pane)
	}

	// loading a row's thread shows its comments and an add form
	thread := get(t, h, "/v/board/b/1/comments?id="+id).Body.String()
	if !strings.Contains(thread, "first note") {
		t.Errorf("thread missing the seeded comment:\n%s", thread)
	}
	if !strings.Contains(thread, `hx-post="/v/board/b/1/comments?id=`+id+`"`) {
		t.Errorf("thread missing the add form:\n%s", thread)
	}

	// posting appends and the fragment comes back with the new comment
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v/board/b/1/comments?id="+id,
		strings.NewReader("comment=second+note"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post comment = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "second note") {
		t.Errorf("posted comment not shown back:\n%s", rec.Body.String())
	}
	comments, _ := api.CommentsFor("task", id)
	if len(comments) != 2 {
		t.Errorf("want 2 comments after add, got %d", len(comments))
	}

	// a bad block index / non-comments block is a 404
	if rec := get(t, h, "/v/board/b/0/comments?id="+id); rec.Code != http.StatusNotFound {
		t.Errorf("comments on a non-comments block = %d, want 404", rec.Code)
	}
}
