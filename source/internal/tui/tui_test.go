// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

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
`

func setup(t *testing.T) (*crud.API, *ddl.Schema, *ddl.Schema) {
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
	return api, sch, bs
}

func TestAppTableOrderAndFields(t *testing.T) {
	api, sch, bs := setup(t)
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.cat.Tables) == 0 || a.cat.Tables[0] != "person" {
		t.Fatalf("user tables must list first, got %v", a.cat.Tables)
	}
	listed := map[string]bool{}
	for _, n := range a.cat.Tables {
		listed[n] = true
	}
	for _, want := range []string{"users", "groups", "audit_trail"} {
		if !listed[want] {
			t.Errorf("built-in %q missing from the table list %v", want, a.cat.Tables)
		}
	}
	if got := a.cat.ColumnsFor("person"); strings.Join(got, ",") != "id,name,nick" {
		t.Errorf("person columns = %v", got)
	}
	if a.cat.HasDeleted["audit_trail"] {
		t.Error("audit_trail must not be treated as soft-deletable")
	}
	if !a.cat.HasDeleted["person"] {
		t.Error("person should filter on is_deleted")
	}
}

func TestLiveRowsHidesSoftDeleted(t *testing.T) {
	api, sch, bs := setup(t)
	id1, err := api.Create("person", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Create("person", map[string]string{"name": "Bob"}); err != nil {
		t.Fatal(err)
	}
	if err := api.MarkDelete("person", id1); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.cat.LiveRows(api, "person")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Bob" {
		t.Fatalf("want just live Bob, got %v", rows)
	}
	groups, err := a.cat.LiveRows(api, "groups")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 4 {
		t.Fatalf("want the 4 seeded groups, got %d", len(groups))
	}
}

// TestAppBootsOnSimulationScreen drives the real event loop on a simulated
// terminal: boot, check the table list drew, open a table, quit with q.
func TestAppBootsOnSimulationScreen(t *testing.T) {
	api, sch, bs := setup(t)
	if _, err := api.Create("person", map[string]string{"name": "Ann"}); err != nil {
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
	screen.SetSize(100, 30)
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

	waitFor("person") // table list drew
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	waitFor("Ann") // grid loaded the row
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

func screenText(s tcell.SimulationScreen) string {
	cells, w, _ := s.GetContents()
	var b strings.Builder
	for i, c := range cells {
		if len(c.Runes) > 0 {
			b.WriteRune(c.Runes[0])
		} else {
			b.WriteByte(' ')
		}
		if (i+1)%w == 0 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
