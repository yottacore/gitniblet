// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package web is the local web UI: stdlib net/http + html/template + embedded
// assets, htmx (vendored, pinned) for interactivity. Zero third-party Go deps.
// Like every front-end it only talks to the shared CRUD API; per the design no
// records render until a table is asked for. Binding to 127.0.0.1 is the
// access control - this is a single-user local UI, not a service.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/jim-collier/nano-git-db/donate"
	"github.com/jim-collier/nano-git-db/gate"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/script"
)

// resolveArgs turns the front-end args into the open triple: a registered
// database name, else an explicit <ddl> <sqlite> <logdir>, else a lone DDL in
// the current directory. The web UI has no interactive picker, so with none of
// these it errors and asks for a name.
func resolveArgs(args []string) (ddlPath, sqlitePath, logDir string, err error) {
	if len(args) == 1 {
		cfg := config.FindByName(args[0])
		if cfg == nil {
			return "", "", "", config.UnknownDatabase(args[0])
		}
		return cfg.DDLPath, cfg.SQLitePath, cfg.LogDir, nil
	}
	if len(args) == 3 {
		return args[0], args[1], args[2], nil
	}
	if len(args) == 0 {
		if d, s, l, ok := config.PWDTriple(); ok {
			return d, s, l, nil
		}
	}
	return "", "", "", fmt.Errorf("usage: ngdb --serve <db> (or run from a directory containing a .shcl schema)")
}

//go:embed assets
var assetsFS embed.FS

//go:embed templates
var tplFS embed.FS

// Run serves the web UI: ngdb --serve <db>. With no name it uses a DDL in the
// current directory; the web front-end has no picker, so with neither it
// requires a database name.
func Run(args []string) error {
	ddlPath, sqlitePath, logDir, err := resolveArgs(args)
	if err != nil {
		return err
	}
	keyFile, pref := config.ResolveEncryption(ddlPath, nil)
	client, err := schema.OpenClientWith(schema.OpenOpts{
		DDLPath: ddlPath, DBPath: sqlitePath, LogDir: logDir, KeyFile: keyFile, EncryptPref: pref,
	})
	if err != nil {
		return err
	}
	defer client.Close()
	for _, warning := range client.Warnings {
		fmt.Println("warning:", warning)
	}
	builtins, err := schema.Builtins()
	if err != nil {
		return err
	}
	attachWarnings, err := script.Attach(client.API, ddlPath, logDir, client.Schema, builtins)
	if err != nil {
		return err
	}
	for _, warning := range attachWarnings {
		fmt.Println("warning:", warning)
	}
	cat, err := schema.NewCatalog(client.API, client.Schema, builtins)
	if err != nil {
		return err
	}
	cat.Queries = client.Queries
	for _, warning := range cat.Warnings {
		fmt.Println("warning:", warning)
	}
	srv, err := newServer(client.API, cat)
	if err != nil {
		return err
	}
	srv.auth = newAuth(config.LoadSettings(), logDir, client.API)
	srv.applyGate(gate.Evaluate())

	stop := client.StartAutoSync(client.Schema.TunableInt("git_sync_frequency", 60),
		func(warning string) { fmt.Println("warning:", warning) })
	defer stop()

	addr := "127.0.0.1:8765" // local UI - do not bind to all interfaces
	srv.hostPin = addr       // reject requests arriving under any other name
	// Explicit timeouts: the stdlib defaults are infinite, so a stalled client
	// could pin connections open forever.
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	fmt.Printf("ngdb web UI: http://%s\n", addr)
	return httpSrv.ListenAndServe()
}

type server struct {
	api *crud.API
	cat *schema.Catalog
	tpl *template.Template

	// auth is the login layer (auth.go). umu serializes the per-request acting
	// user in proxied mode, so concurrent requests never race on api.UserID.
	auth *authState
	umu  sync.Mutex

	// hostPin is the address Run bound to; the Host header must match it in
	// local mode. Empty when no listener was started (tests, embedders).
	hostPin string

	// startup-notice state (gate.go), read on every request and written by the
	// /gate/ handlers, so gmu guards it. now is injectable for tests.
	gmu      sync.Mutex
	now      func() time.Time
	gated    bool      // holding on the start screen until continue/dismiss
	unlockAt time.Time // read/write is withheld until this time
	gateMsg  string    // message shown on the start screen and banner
	banner   string    // non-empty -> a banner bar in the layout
}

func newServer(api *crud.API, cat *schema.Catalog) (*server, error) {
	tpl := template.New("").Funcs(template.FuncMap{
		"short": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
	})
	tpl, err := tpl.ParseFS(tplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &server{api: api, cat: cat, tpl: tpl, now: time.Now}, nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(assetsFS, "assets") // embedded at build time, so it is always there
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /v/{view}", s.viewPage)
	mux.HandleFunc("GET /v/{view}/b/{i}/rows", s.viewBlockRows)
	mux.HandleFunc("GET /v/{view}/b/{i}/comments", s.viewBlockComments)
	mux.HandleFunc("POST /v/{view}/b/{i}/comments", s.viewBlockComments)
	mux.HandleFunc("GET /v/{view}/q", s.viewQuery)
	if donate.Enabled { // open-source-only feature
		mux.HandleFunc("GET /donate", s.donate)
	}
	mux.HandleFunc("GET /t/{table}", s.rows)
	mux.HandleFunc("GET /t/{table}/new", s.form)
	mux.HandleFunc("GET /t/{table}/{id}/edit", s.form)
	mux.HandleFunc("POST /t/{table}", s.create)
	mux.HandleFunc("POST /t/{table}/{id}", s.update)
	mux.HandleFunc("POST /t/{table}/{id}/delete", s.del)
	mux.HandleFunc("POST /t/{table}/{id}/comment", s.comment)
	mux.HandleFunc("POST /t/{table}/{id}/attach", s.attach)
	mux.HandleFunc("POST /gate/continue", s.gateContinue)
	mux.HandleFunc("POST /gate/dismiss", s.gateDismiss)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.loginSubmit)
	mux.HandleFunc("POST /logout", s.logout)
	// originGuard is outermost - a request from somewhere else never reaches the
	// login layer. Then authGuard decides who may reach anything, then the gate
	// layer (license notice), then the app routes.
	return s.originGuard(s.authGuard(s.gateGuard(mux)))
}

// table pulls and validates the table path segment; "" means already handled.
func (s *server) table(w http.ResponseWriter, r *http.Request) string {
	name := r.PathValue("table")
	if !s.cat.Has(name) {
		http.Error(w, "no such table", http.StatusNotFound)
		return ""
	}
	return name
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, "layout.html", map[string]any{
		"Tables": s.cat.Tables, "Views": s.cat.Views, "DefaultView": s.cat.DefaultView,
		"Banner": s.bannerText(), "ReadOnly": s.api.ReadOnly(),
		"Proxied": s.auth != nil && s.auth.proxied, "User": s.api.UserID,
		"Donate": donate.Enabled,
	})
}

// rows renders the grid partial - also the response to every write, so the
// user always returns to the refreshed table.
func (s *server) rows(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	rows, err := s.cat.LiveRows(s.api, table)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "rows.html", map[string]any{
		"Table": table, "Cols": s.cat.ColumnsFor(table), "Rows": rows,
	})
}

func (s *server) form(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	id := r.PathValue("id")
	row := map[string]string{}
	if id != "" {
		var ok bool
		var err error
		row, ok, err = s.api.Get(table, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "no such row", http.StatusNotFound)
			return
		}
	}
	data := map[string]any{
		"Table": table, "ID": id, "Fields": s.cat.Fields[table], "Row": row,
	}
	// the row's opt-in extras only exist once the row does
	if id != "" {
		feats := s.cat.Features[table]
		data["CanComment"] = feats.Comments
		data["CanAttachURI"] = feats.URIAttachments
		data["CanAttachFile"] = feats.LocalAttachments
		if feats.Comments {
			comments, err := s.api.CommentsFor(table, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data["Comments"] = comments
		}
		if feats.URIAttachments || feats.LocalAttachments {
			attachments, err := s.api.AttachmentsFor(table, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data["Attachments"] = attachments
		}
	}
	s.render(w, "form.html", data)
}

// comment adds a comment and returns to the refreshed form.
func (s *server) comment(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if text := r.PostForm.Get("comment"); text != "" {
		if _, err := s.api.CommentAdd(table, r.PathValue("id"), text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.form(w, r)
}

// attach adds a URI (kind=uri) or copied-file (kind=file, server-local path -
// this UI is 127.0.0.1-only, the browser runs on the same machine) attachment.
func (s *server) attach(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, desc := r.PostForm.Get("target"), r.PostForm.Get("description")
	if target != "" {
		var err error
		if r.PostForm.Get("kind") == "file" {
			_, err = s.api.AttachFile(table, r.PathValue("id"), target, desc)
		} else {
			_, err = s.api.AttachURI(table, r.PathValue("id"), target, desc)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.form(w, r)
}

// formFields reads only the table's declared fields from the request - posted
// extras (or system columns) never reach the CRUD layer.
func (s *server) formFields(r *http.Request, table string) map[string]string {
	vals := map[string]string{}
	for _, f := range s.cat.Fields[table] {
		if _, ok := r.PostForm[f]; ok {
			vals[f] = r.PostForm.Get(f)
		}
	}
	return vals
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.api.Create(table, s.formFields(r, table)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rows(w, r)
}

// update writes only the changed fields, so the tx-log stays field-granular.
func (s *server) update(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	cur, ok, err := s.api.Get(table, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "no such row", http.StatusNotFound)
		return
	}
	changed := map[string]string{}
	for f, v := range s.formFields(r, table) {
		if cur[f] != v {
			changed[f] = v
		}
	}
	if len(changed) > 0 {
		if err := s.api.Update(table, id, changed); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.rows(w, r)
}

func (s *server) del(w http.ResponseWriter, r *http.Request) {
	table := s.table(w, r)
	if table == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	var err error
	if r.PostForm.Get("hard") == "1" {
		err = s.api.Delete(table, id)
	} else {
		err = s.api.MarkDelete(table, id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rows(w, r)
}
