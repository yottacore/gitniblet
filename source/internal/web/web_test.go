// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const personDDL = `
tables:
	table: person
		fields:
			field: name
				type: string
			field: nick
				type: string
		features:
			comments: y
			uri_attachments: y
			audit_trail: y
`

func newTestServer(t *testing.T) (*server, *crud.API) {
	t.Helper()
	sch, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "view.sqlite"))
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
	if err := schema.SeedDefaults(api); err != nil {
		t.Fatal(err)
	}
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

func do(t *testing.T, h http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestIndexListsTables(t *testing.T) {
	s, _ := newTestServer(t)
	w := do(t, s.routes(), "GET", "/", nil)
	if w.Code != 200 {
		t.Fatalf("GET /: %d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{"person", "users", "groups", "htmx.min.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
	if !strings.Contains(body, "No records are shown until you ask") {
		t.Error("index must not render any rows by default")
	}
}

func TestRowsAndUnknownTable(t *testing.T) {
	s, api := newTestServer(t)
	if _, err := api.Create("person", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	w := do(t, s.routes(), "GET", "/t/person", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ann") {
		t.Fatalf("rows: %d\n%s", w.Code, w.Body)
	}
	if w := do(t, s.routes(), "GET", "/t/nope", nil); w.Code != 404 {
		t.Fatalf("unknown table should 404, got %d", w.Code)
	}
	// path traversal / injection shaped names must not reach SQL
	if w := do(t, s.routes(), "GET", "/t/person%22%3B%20DROP%20TABLE", nil); w.Code != 404 {
		t.Fatalf("hostile table name should 404, got %d", w.Code)
	}
}

func TestCreateUpdateDeleteRoundTrip(t *testing.T) {
	s, api := newTestServer(t)
	h := s.routes()

	w := do(t, h, "POST", "/t/person", url.Values{"name": {"Ann"}, "nick": {"A"}, "evil": {"x"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ann") {
		t.Fatalf("create: %d\n%s", w.Code, w.Body)
	}
	rows, err := api.Query(`SELECT * FROM "person" WHERE "is_deleted"=0`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows after create: %v %v", rows, err)
	}
	id := rows[0]["id"]

	// edit form shows current values
	w = do(t, h, "GET", "/t/person/"+id+"/edit", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `value="Ann"`) {
		t.Fatalf("edit form: %d\n%s", w.Code, w.Body)
	}

	w = do(t, h, "POST", "/t/person/"+id, url.Values{"name": {"Anne"}, "nick": {"A"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Anne") {
		t.Fatalf("update: %d\n%s", w.Code, w.Body)
	}

	w = do(t, h, "POST", "/t/person/"+id+"/delete", url.Values{})
	if w.Code != 200 {
		t.Fatalf("soft delete: %d\n%s", w.Code, w.Body)
	}
	rows, _ = api.Query(`SELECT * FROM "person" WHERE "is_deleted"=0`)
	if len(rows) != 0 {
		t.Fatal("soft delete left a live row")
	}

	w = do(t, h, "POST", "/t/person/"+id+"/delete", url.Values{"hard": {"1"}})
	if w.Code != 200 {
		t.Fatalf("hard delete: %d\n%s", w.Code, w.Body)
	}
	rows, _ = api.Query(`SELECT * FROM "person"`)
	if len(rows) != 0 {
		t.Fatal("hard delete left the row behind")
	}
}

// The edit form of an opted-in table carries the comments and attachments
// sections; posting to them returns to the refreshed form.
func TestRowExtrasCommentsAndAttachments(t *testing.T) {
	s, api := newTestServer(t)
	id, err := api.Create("person", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	h := s.routes()

	w := do(t, h, "GET", "/t/person/"+id+"/edit", nil)
	body := w.Body.String()
	if !strings.Contains(body, "add a comment") || !strings.Contains(body, "Attach") {
		t.Fatalf("edit form missing extras sections: %s", body)
	}

	w = do(t, h, "POST", "/t/person/"+id+"/comment", url.Values{"comment": {"<b>hello</b>"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "&lt;b&gt;hello&lt;/b&gt;") {
		t.Fatalf("comment post: %d %s", w.Code, w.Body)
	}

	w = do(t, h, "POST", "/t/person/"+id+"/attach",
		url.Values{"kind": {"uri"}, "target": {"https://example.com"}, "description": {"spec"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://example.com") {
		t.Fatalf("attach post: %d %s", w.Code, w.Body)
	}

	// a new-row form has no row yet, so no extras
	w = do(t, h, "GET", "/t/person/new", nil)
	if strings.Contains(w.Body.String(), "add a comment") {
		t.Fatal("new-row form should not show extras")
	}
}

func TestValuesAreHTMLEscaped(t *testing.T) {
	s, api := newTestServer(t)
	if _, err := api.Create("person", map[string]string{"name": `<script>alert(1)</script>`}); err != nil {
		t.Fatal(err)
	}
	w := do(t, s.routes(), "GET", "/t/person", nil)
	if strings.Contains(w.Body.String(), "<script>alert") {
		t.Fatal("row values must be HTML-escaped")
	}
	if !regexp.MustCompile(`&lt;script&gt;`).MatchString(w.Body.String()) {
		t.Fatal("escaped value missing entirely")
	}
}

func TestAssetsServed(t *testing.T) {
	s, _ := newTestServer(t)
	for _, p := range []string{"/assets/htmx.min.js", "/assets/style.css"} {
		if w := do(t, s.routes(), "GET", p, nil); w.Code != 200 || w.Body.Len() == 0 {
			t.Errorf("%s: %d len=%d", p, w.Code, w.Body.Len())
		}
	}
}
