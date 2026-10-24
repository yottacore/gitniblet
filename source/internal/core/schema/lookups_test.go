// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"sync"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

func lookupsAPI(t *testing.T) (*crud.API, *store.Store, *txlog.Log) {
	t.Helper()
	st := openStore(t)
	if _, err := Bootstrap(st, nil); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return crud.New(st, lg), st, lg
}

func idxOf(t *testing.T, api *crud.API, table, id string) string {
	t.Helper()
	row, ok, err := api.Get(table, id)
	if err != nil || !ok {
		t.Fatalf("get %s %s: ok=%v err=%v", table, id, ok, err)
	}
	return row["idx"]
}

// The lookups idx is minted max+1, kept when given, and fixed.
func TestLookupsIdx(t *testing.T) {
	api, st, lg := lookupsAPI(t)
	for _, table := range []string{"lookups", "lookup_values"} {
		if !hasTable(t, st, table) {
			t.Fatalf("%s should be always-on", table)
		}
	}
	mk := func(table string, fields map[string]string) string {
		t.Helper()
		id, err := api.Create(table, fields)
		if err != nil {
			t.Fatalf("create %s %v: %v", table, fields, err)
		}
		return id
	}

	first := mk("lookups", map[string]string{"title": "Status"})
	second := mk("lookups", map[string]string{"title": "Size"})
	if got := idxOf(t, api, "lookups", first) + idxOf(t, api, "lookups", second); got != "12" {
		t.Fatalf("want idx 1 then 2, got %q", got)
	}
	given := mk("lookups", map[string]string{"title": "Seeded", "idx": "10"})
	if got := idxOf(t, api, "lookups", given); got != "10" {
		t.Fatalf("given idx should stand, got %q", got)
	}
	after := mk("lookups", map[string]string{"title": "Next"})
	if got := idxOf(t, api, "lookups", after); got != "11" {
		t.Fatalf("want max+1 = 11, got %q", got)
	}
	for _, bad := range []string{"10", "0", "-3", "abc", "1.5"} {
		if _, err := api.Create("lookups", map[string]string{"title": "X", "idx": bad}); err == nil {
			t.Errorf("idx %q should be refused", bad)
		}
	}

	// A deleted row still holds its idx, so it is never minted again.
	if err := api.MarkDelete("lookups", after); err != nil {
		t.Fatal(err)
	}
	if got := idxOf(t, api, "lookups", mk("lookups", map[string]string{"title": "Later"})); got != "12" {
		t.Fatalf("a deleted idx must not be reused, got %q", got)
	}

	// Each table counts on its own.
	val := mk("lookup_values", map[string]string{"lookups_idx": "1", "title": "Open", "symbol": "🟢"})
	if got := idxOf(t, api, "lookup_values", val); got != "1" {
		t.Fatalf("lookup_values should start at 1, got %q", got)
	}

	if err := api.Update("lookups", first, map[string]string{"idx": "7"}); err == nil {
		t.Error("update must not change idx")
	}
	if err := api.SetField("lookups", first, "idx", "7"); err == nil {
		t.Error("SetField must not change idx")
	}
	if err := api.SetFieldNull("lookups", first, "idx"); err == nil {
		t.Error("SetFieldNull must not clear idx")
	}
	// A form save writes the unchanged idx back with the real edit.
	if err := api.Update("lookups", first, map[string]string{"idx": "1", "title": "State"}); err != nil {
		t.Fatalf("same idx written back should pass: %v", err)
	}
	if err := api.SetField("lookups", first, "idx", "1"); err != nil {
		t.Fatalf("same idx via SetField should pass: %v", err)
	}
	row, _, _ := api.Get("lookups", first)
	if row["title"] != "State" || row["idx"] != "1" {
		t.Fatalf("want title State idx 1, got %v", row)
	}

	// The idx lives in the log, so a rebuilt view has the same numbers.
	st2 := openStore(t)
	if _, err := Bootstrap(st2, nil); err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txlog.Apply(st2, entries); err != nil {
		t.Fatal(err)
	}
	if got := idxOf(t, crud.New(st2, lg), "lookups", given); got != "10" {
		t.Fatalf("rebuilt view: want idx 10, got %q", got)
	}
}

// Concurrent creates never mint the same idx.
func TestLookupsIdxConcurrent(t *testing.T) {
	api, st, _ := lookupsAPI(t)
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := api.Create("lookups", map[string]string{"title": "T"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var rows, distinct, top int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*), COUNT(DISTINCT "idx"), MAX("idx") FROM "lookups"`).Scan(&rows, &distinct, &top); err != nil {
		t.Fatal(err)
	}
	if rows != n || distinct != n || top != n {
		t.Fatalf("want %d rows with idx 1..%d, got rows=%d distinct=%d max=%d", n, n, rows, distinct, top)
	}
}
