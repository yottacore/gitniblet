// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const seedDDL = `database:
	tables:
		table: task
			fields:
				field: title
					type: string
	seed:
		table: lookups
			row: idx=1, title="Task status"
		table: lookup_values
			row: idx=1, lookups_idx=1, title="Open",   symbol="🟢", sort=1
			row: idx=2, lookups_idx=1, title="Closed", symbol="✅", sort=2
			row: idx=3, lookups_idx=1, title="Typo", colour="red"
`

type seedRig struct {
	t            *testing.T
	ddl, db, log string
}

func newSeedRig(t *testing.T) *seedRig {
	dir := t.TempDir()
	r := &seedRig{t: t, ddl: filepath.Join(dir, "s.shcl"), db: filepath.Join(dir, "v.sqlite"), log: filepath.Join(dir, "log")}
	if err := os.WriteFile(r.ddl, []byte(seedDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *seedRig) open() *Client {
	r.t.Helper()
	c, err := OpenClient(r.ddl, r.db, r.log, "u")
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { c.Close() })
	return c
}

// every row in a table, deleted ones too, as idx -> title
func allRows(t *testing.T, c *Client, table string) map[string]string {
	t.Helper()
	rows, err := c.API.Query(`SELECT "idx", "title", "is_deleted" FROM "` + table + `"`)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, row := range rows {
		if _, dup := out[row["idx"]]; dup {
			t.Fatalf("%s idx %s is there twice", table, row["idx"])
		}
		out[row["idx"]] = row["title"]
		if row["is_deleted"] == "1" {
			out[row["idx"]] += " (deleted)"
		}
	}
	return out
}

func idOfIdx(t *testing.T, c *Client, table, idx string) string {
	t.Helper()
	rows, err := c.API.Query(`SELECT idtext("id") AS id FROM "`+table+`" WHERE "idx"=?`, idx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("%s idx %s: %v %v", table, idx, rows, err)
	}
	return rows[0]["id"]
}

// Seed rows go in log-first when their idx is absent, and never overwrite.
// ID: dec95
func TestSeedRowsLogFirst(t *testing.T) {
	r := newSeedRig(t)
	c := r.open()
	if got := allRows(t, c, "lookups"); len(got) != 1 || got["1"] != "Task status" {
		t.Fatalf("lookups = %v", got)
	}
	got := allRows(t, c, "lookup_values")
	if len(got) != 2 || got["1"] != "Open" || got["2"] != "Closed" {
		t.Fatalf("lookup_values = %v", got)
	}
	var typo string
	for _, w := range c.Warnings {
		if strings.Contains(w, "lookup_values idx 3 not added") && strings.Contains(w, `"colour"`) {
			typo = w
		}
	}
	if typo == "" {
		t.Fatalf("no warning for the unknown field, got %v", c.Warnings)
	}

	// later edits win: a retitle and a delete both survive the next open
	if err := c.API.Update("lookup_values", idOfIdx(t, c, "lookup_values", "1"), map[string]string{"title": "Todo"}); err != nil {
		t.Fatal(err)
	}
	if err := c.API.MarkDelete("lookup_values", idOfIdx(t, c, "lookup_values", "2")); err != nil {
		t.Fatal(err)
	}
	c.Close()
	c = r.open()
	want := map[string]string{"1": "Todo", "2": "Closed (deleted)"}
	if got := allRows(t, c, "lookup_values"); len(got) != 2 || got["1"] != want["1"] || got["2"] != want["2"] {
		t.Fatalf("after reopen lookup_values = %v, want %v", got, want)
	}
	c.Close()

	// the log alone holds the seed: a fresh view has it once, edits applied
	if err := os.Remove(r.db); err != nil {
		t.Fatal(err)
	}
	c = r.open()
	if got := allRows(t, c, "lookup_values"); len(got) != 2 || got["1"] != want["1"] || got["2"] != want["2"] {
		t.Fatalf("rebuilt lookup_values = %v, want %v", got, want)
	}
	if got := allRows(t, c, "lookups"); len(got) != 1 {
		t.Fatalf("rebuilt lookups = %v", got)
	}
}

// The shipped example's seed rows all go in.
// ID: dec96
func TestSeedRowsExample(t *testing.T) {
	ddlPath := findExampleDDL(t)
	dir := t.TempDir()
	c, err := OpenClient(ddlPath, filepath.Join(dir, "v.sqlite"), filepath.Join(dir, "log"), "u")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, w := range c.Warnings {
		if strings.HasPrefix(w, "seed:") {
			t.Fatalf("example seed warns: %s", w)
		}
	}
	if got := allRows(t, c, "lookup_values"); len(got) != 2 || got["2"] != "Closed" {
		t.Fatalf("example lookup_values = %v", got)
	}
}

func findExampleDDL(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "..", "project", "example.shcl")
	if _, err := os.Stat(p); err != nil {
		t.Skip("project/example.shcl not found")
	}
	return p
}

// Two clients that seed before their first sync each log the same rows. After
// the logs merge, every view holds one copy of each, the same one everywhere
// (backlog 2026102407584270). The default groups go the same way.
// ID: dec9d
func TestSeedRowsTwoClients(t *testing.T) {
	a, b := newSeedRig(t), newSeedRig(t)
	a.open().Close()
	b.open().Close()
	logA, err := os.ReadFile(filepath.Join(a.log, "txlog.csv"))
	if err != nil {
		t.Fatal(err)
	}
	logB, err := os.ReadFile(filepath.Join(b.log, "txlog.csv"))
	if err != nil {
		t.Fatal(err)
	}
	merged := append(append([]byte{}, logA...), logB...)
	fresh := newSeedRig(t)
	for _, r := range []*seedRig{a, b, fresh} {
		if err := os.MkdirAll(r.log, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(r.log, "txlog.csv"), merged, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var firstIDs map[string]string
	for _, r := range []*seedRig{a, b, fresh} {
		c := r.open()
		if got := allRows(t, c, "lookup_values"); len(got) != 2 || got["1"] != "Open" || got["2"] != "Closed" {
			t.Fatalf("lookup_values = %v", got)
		}
		if got := allRows(t, c, "lookups"); len(got) != 1 {
			t.Fatalf("lookups = %v", got)
		}
		groups, err := c.API.Query(`SELECT "name", count(*) AS n FROM "groups" GROUP BY "name" HAVING n > 1 OR "name" IS NULL OR "name" = ''`)
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 0 {
			t.Errorf("groups with twins or no name: %v", groups)
		}
		ids := map[string]string{
			"lookups 1":       idOfIdx(t, c, "lookups", "1"),
			"lookup_values 1": idOfIdx(t, c, "lookup_values", "1"),
			"lookup_values 2": idOfIdx(t, c, "lookup_values", "2"),
		}
		if firstIDs == nil {
			firstIDs = ids
		} else {
			for k, v := range ids {
				if firstIDs[k] != v {
					t.Errorf("%s is row %s here and %s in the first view", k, v, firstIDs[k])
				}
			}
		}
		c.Close()
	}
}
