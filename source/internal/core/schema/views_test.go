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

const viewDDL = `
tables:
	table: employees
		fields:
			field: name
				type: string
			field: boss_id
				type: string
	table: pets
		fields:
			field: name
				type: string

views:
	view: "staff"
		readonly: no
		layout:
			block: "top"
				block: "who"
					table: employees
					type: form
				block: "their-pets"
					table: pets
					location: who, right, 25%
					type: grid
			block: "chain"
				table: employees
				location: "top", below, 40%
				type: tree_grid
				parent_field: boss_id
				readonly: yes
	view: "broken"
		layout:
			block: "gone"
				table: no_such_table
				type: grid
	view: "bad-tree"
		layout:
			block: "t"
				table: employees
				type: tree_grid
				parent_field: nope
`

func viewSetup(t *testing.T) (*crud.API, *Catalog) {
	t.Helper()
	sch, err := ddl.Parse([]byte(viewDDL))
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
	return api, cat
}

func TestViewResolution(t *testing.T) {
	_, cat := viewSetup(t)

	if len(cat.Views) != 2 {
		t.Fatalf("want 2 surviving views, got %d: %+v", len(cat.Views), cat.Views)
	}
	if cat.View("broken") != nil {
		t.Error("view over an unknown table should have been dropped")
	}
	warns := strings.Join(cat.Warnings, "\n")
	for _, want := range []string{"no_such_table", `view "broken"`, "parent_field"} {
		if !strings.Contains(warns, want) {
			t.Errorf("warnings missing %q:\n%s", want, warns)
		}
	}

	v := cat.View("staff")
	if v == nil {
		t.Fatal("view staff not resolved")
	}
	if got := len(v.Leaves); got != 3 {
		t.Fatalf("staff: want 3 leaves, got %d", got)
	}
	if v.Root.Dir != "row" {
		t.Errorf("root splits vertically (below hint), got %q", v.Root.Dir)
	}
	top := v.Root.Children[0]
	if top.Dir != "col" {
		t.Errorf("block top splits horizontally (right hint), got %q", top.Dir)
	}
	if p := v.Root.Proportions(); p[0] != 60 || p[1] != 40 {
		t.Errorf("root proportions: want [60 40], got %v", p)
	}
	chain := v.Leaves[2]
	if chain.Type != "tree_grid" || chain.ParentField != "boss_id" || !chain.Readonly {
		t.Errorf("chain leaf wrong: %+v", chain)
	}
	if v.Leaves[0].Readonly || v.Leaves[1].Readonly {
		t.Error("view-level readonly:no should leave unmarked blocks writable")
	}

	// tree_grid with a bogus parent_field degrades to a plain grid
	bt := cat.View("bad-tree")
	if bt == nil || bt.Leaves[0].Type != "grid" || bt.Leaves[0].ParentField != "" {
		t.Errorf("bad-tree should degrade to grid: %+v", bt)
	}

	// no ui: default_view in the DDL -> the first resolved view is the default
	if cat.DefaultView != "staff" {
		t.Errorf("default view: want first resolved view, got %q", cat.DefaultView)
	}
}

// An explicit default_view wins when usable; one naming a dropped or unknown
// view falls back to the first resolved view with a warning.
func TestDefaultViewResolution(t *testing.T) {
	api, _ := viewSetup(t)
	parse := func(dv string) *ddl.Schema {
		t.Helper()
		sch, err := ddl.Parse([]byte(viewDDL + "\nui:\n\tdefault_view: " + dv + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}
	bs, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}

	cat, err := NewCatalog(api, parse(`"bad-tree"`), bs)
	if err != nil {
		t.Fatal(err)
	}
	if cat.DefaultView != "bad-tree" {
		t.Errorf("explicit default view: want bad-tree, got %q", cat.DefaultView)
	}

	cat, err = NewCatalog(api, parse(`"broken"`), bs) // resolves to nothing: dropped view
	if err != nil {
		t.Fatal(err)
	}
	if cat.DefaultView != "staff" {
		t.Errorf("unusable default view should fall back to first, got %q", cat.DefaultView)
	}
	if !strings.Contains(strings.Join(cat.Warnings, "\n"), "default_view") {
		t.Errorf("no default_view warning in %v", cat.Warnings)
	}
}

func TestTreeRows(t *testing.T) {
	api, cat := viewSetup(t)
	mk := func(name, boss string) string {
		id, err := api.Create("employees", map[string]string{"name": name, "boss_id": boss})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ceo := mk("ceo", "")
	vp := mk("vp", ceo)
	mk("eng", vp)
	mk("orphan", "gone-id") // unknown parent -> root
	// two-row cycle: unreachable from any root, must still render
	c1 := mk("c1", "")
	c2 := mk("c2", c1)
	if err := api.Update("employees", c1, map[string]string{"boss_id": c2}); err != nil {
		t.Fatal(err)
	}

	rows, err := cat.TreeRows(api, "employees", "boss_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 {
		t.Fatalf("want all 6 rows, got %d", len(rows))
	}
	got := make([]string, len(rows))
	depths := map[string]int{}
	for i, r := range rows {
		got[i] = r.Row["name"]
		depths[r.Row["name"]] = r.Depth
	}
	order := strings.Join(got, ",")
	if !strings.Contains(order, "ceo,vp,eng") {
		t.Errorf("chain not depth-first: %s", order)
	}
	for name, want := range map[string]int{"ceo": 0, "vp": 1, "eng": 2, "orphan": 0} {
		if depths[name] != want {
			t.Errorf("%s: want depth %d, got %d", name, want, depths[name])
		}
	}
	if depths["c1"] != 0 || depths["c2"] != 1 {
		t.Errorf("cycle rows should ground at a synthetic root: c1=%d c2=%d", depths["c1"], depths["c2"])
	}

	if _, err := cat.TreeRows(api, "employees", "nope"); err == nil {
		t.Error("bad parent field should error")
	}
}

// A comments-enabled table with a tree_grid list + a comments detail pane, plus
// a comments block over a table that never opted in (must be dropped).
const commentsViewDDL = `
tables:
	table: task
		fields:
			field: title
				type: string
			field: parent_task
				type: string
		features:
			comments: yes
	table: plain
		fields:
			field: label
				type: string

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
	view: "no-feature"
		layout:
			block: 1
				table: plain
				type: grid
			block: 2
				table: plain
				type: comments
`

func catalogFrom(t *testing.T, src string) *Catalog {
	t.Helper()
	sch, err := ddl.Parse([]byte(src))
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

func TestResolveCommentsBlock(t *testing.T) {
	cat := catalogFrom(t, commentsViewDDL)

	board := cat.View("board")
	if board == nil {
		t.Fatal("board view not resolved")
	}
	if got := len(board.Leaves); got != 2 {
		t.Fatalf("board: want 2 leaves (list + comments), got %d", got)
	}
	if board.Leaves[1].Type != "comments" || board.Leaves[1].Table != "task" {
		t.Fatalf("leaf 1 should be the comments pane over task: %+v", board.Leaves[1])
	}
	if got := board.PrimaryForComments(1); got != 0 {
		t.Errorf("comments pane should follow leaf 0, got %d", got)
	}
	if got := board.CommentsLeavesFor(0); len(got) != 1 || got[0] != 1 {
		t.Errorf("list leaf 0 should feed comments leaf 1, got %v", got)
	}
	// stacked below with a 35% share
	if board.Root.Dir != "row" {
		t.Errorf("root should stack (below hint), got %q", board.Root.Dir)
	}

	// a comments block over a table with no comments feature is dropped, leaving
	// just the grid; the drop is warned.
	nf := cat.View("no-feature")
	if nf == nil {
		t.Fatal("no-feature view should survive on its grid alone")
	}
	if len(nf.Leaves) != 1 || nf.Leaves[0].Type != "grid" {
		t.Fatalf("no-feature: comments block should be dropped, leaving 1 grid leaf, got %+v", nf.Leaves)
	}
	warns := strings.Join(cat.Warnings, "\n")
	if !strings.Contains(warns, "no comments feature") {
		t.Errorf("expected a dropped-comments-block warning:\n%s", warns)
	}
}
