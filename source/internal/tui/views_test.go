// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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

func orgSetup(t *testing.T) (*crud.API, *ddl.Schema, *ddl.Schema) {
	return ddlSetup(t, orgDDL)
}

func ddlSetup(t *testing.T, src string) (*crud.API, *ddl.Schema, *ddl.Schema) {
	t.Helper()
	sch, err := ddl.Parse([]byte(src))
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
	return api, sch, bs
}

func TestViewsListedBeforeTables(t *testing.T) {
	api, sch, bs := orgSetup(t)
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.cat.Views) != 1 || a.cat.Views[0].Name != "org" {
		t.Fatalf("want the org view resolved, got %+v", a.cat.Views)
	}
	if a.cat.DefaultView != "org" {
		t.Fatalf("default view = %q want org", a.cat.DefaultView)
	}
	if got, want := a.list.GetItemCount(), len(a.cat.Views)+len(a.cat.Tables); got != want {
		t.Fatalf("list items = %d, want views+tables = %d", got, want)
	}
	main, _ := a.list.GetItemText(0)
	if main != "org (view)" {
		t.Fatalf("first list item should be the view, got %q", main)
	}
}

// TestViewOpensAndLoadsOnSimulationScreen: the default view (first defined,
// no explicit default_view) auto-opens at boot; Esc back to the list, reopen
// it with Enter, load the tree_grid with 'a', hop to the second block with
// Tab and load it too, then back out and quit.
func TestViewOpensAndLoadsOnSimulationScreen(t *testing.T) {
	api, sch, bs := orgSetup(t)
	boss, err := api.Create("employees", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee", "boss_id": boss}); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 32)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()

	waitForN := func(sub string, n int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Count(screenText(screen), sub) >= n {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q (x%d) never appeared on screen:\n%s", sub, n, screenText(screen))
	}
	waitFor := func(sub string) { t.Helper(); waitForN(sub, 1) }

	waitFor("chain: employees (tree_grid)")             // default view opened itself
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) // back to the list
	waitFor("org (view)")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // first item = the view
	waitFor("chain: employees (tree_grid)")
	waitFor("flat: employees (grid, ro)")
	screen.InjectKey(tcell.KeyRune, 'a', tcell.ModNone) // load focused tree_grid
	waitFor("Bee")
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'a', tcell.ModNone) // load the flat grid too
	waitForN("Bee", 2)
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone) // back to the list
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit the app")
	}
}

// TestStartupQueryAndPicker: a view with startup_named_query loads that
// query's dataset the moment it opens; 'p' offers the view's queries.
func TestStartupQueryAndPicker(t *testing.T) {
	src := strings.Replace(orgDDL, "view: \"org\"\n",
		"view: \"org\"\n\t\tstartup_named_query: \"Just Ann\"\n", 1)
	api, sch, bs := ddlSetup(t, src)
	if _, err := api.Create("employees", map[string]string{"name": "Ann"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("employees", map[string]string{"name": "Bee"}); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	a.cat.Queries = []ddl.NamedQuery{
		{Name: "Just Ann", View: "org", Active: true,
			SQL: `SELECT "name" FROM "employees" WHERE "name"='Ann' AND "is_deleted"=0`},
		{Name: "Everyone", Active: true, // no view = offered everywhere
			SQL: `SELECT "name" FROM "employees" WHERE "is_deleted"=0`},
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 32)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()

	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q never appeared on screen:\n%s", sub, screenText(screen))
	}

	waitFor("Just Ann: 1 rows") // startup query ran on auto-open
	waitFor("Ann")
	screen.InjectKey(tcell.KeyRune, 'p', tcell.ModNone) // query picker
	waitFor("Everyone")
	screen.InjectKey(tcell.KeyDown, 0, tcell.ModNone) // pick the global query
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitFor("Everyone: 2 rows")
	waitFor("Bee")
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit the app")
	}
}

const boardDDL = `
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
`

// The comments pane follows the tree_grid selection and can append a comment;
// none of it shows as a board column.
func TestCommentsPaneLinksAndAdds(t *testing.T) {
	api, sch, bs := ddlSetup(t, boardDDL)
	api.EnableFeatures(sch, bs)
	id, err := api.Create("task", map[string]string{"title": "Fix login bug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.CommentAdd("task", id, "seeded thread"); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 40)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()

	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q never appeared:\n%s", sub, screenText(screen))
	}
	waitForButton := func(label string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if button, ok := a.app.GetFocus().(*tview.Button); ok && button.GetLabel() == label {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("focus never reached the %q button", label)
	}
	typeText := func(s string) {
		for _, r := range s {
			screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
			time.Sleep(5 * time.Millisecond)
		}
	}

	waitFor("1: task (tree_grid)")                      // default board auto-opens
	waitFor("2: task (comments)")                       // the detail pane is present
	screen.InjectKey(tcell.KeyRune, 'a', tcell.ModNone) // load the tree_grid
	waitFor("Fix login bug")
	waitFor("seeded thread") // auto-selected row's thread loaded into the pane

	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)   // focus the comments pane
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // open the add form
	waitFor("new comment")
	typeText("shipped it")
	waitFor("shipped it") // text is in the input field
	screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitForButton("Add")                               // Enter in the field only moves focus
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // press Add
	waitFor("comments: 2")                             // pane reloaded after the append

	comments, _ := api.CommentsFor("task", id)
	if len(comments) != 2 {
		a.Stop()
		t.Fatalf("want 2 comments after add, got %d", len(comments))
	}

	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit")
	}
}
