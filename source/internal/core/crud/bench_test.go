// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// seedLog builds n rows' worth of entries and writes them to a fresh log in one
// batched append, then reads them back - the input to a replay. It uses the same
// entry shape a.Create produces (a create entry plus a field update per column),
// but batches the write so seeding isn't dominated by one fsync per row.
func seedLog(tb testing.TB, n int) ([]txlog.Entry, *ddl.Schema) {
	tb.Helper()
	dir := tb.TempDir()
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		tb.Fatal(err)
	}
	lg, err := txlog.Open(filepath.Join(dir, "log"))
	if err != nil {
		tb.Fatal(err)
	}
	a := New(nil, lg)
	a.UserID = "u1"
	all := make([]txlog.Entry, 0, n*3)
	for i := 0; i < n; i++ {
		id := newID()
		all = append(all,
			a.entry("person", id, "", "create", ""),
			a.entry("person", id, "name", "update", fmt.Sprintf("n%d", i)),
			a.entry("person", id, "age", "update", fmt.Sprintf("%d", i%100)),
		)
	}
	if err := lg.Append(all...); err != nil {
		tb.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		tb.Fatal(err)
	}
	return entries, schema
}

// BenchmarkReplay rebuilds a fresh SQLite view from the tx-log - the CPU-hot
// startup path (log parse + decode + ordered SQL apply). It is also what the
// profiler stage samples.
func BenchmarkReplay(b *testing.B) {
	entries, schema := seedLog(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st, err := store.Open(":memory:")
		if err != nil {
			b.Fatal(err)
		}
		if err := st.Build(schema); err != nil {
			b.Fatal(err)
		}
		if _, err := txlog.Apply(st, entries); err != nil {
			b.Fatal(err)
		}
		st.Close()
	}
}

// BenchmarkCreate measures the write path itself (log append + SQL insert).
func BenchmarkCreate(b *testing.B) {
	dir := b.TempDir()
	schema, err := ddl.Parse([]byte(personDDL))
	if err != nil {
		b.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "v.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	if err := st.Build(schema); err != nil {
		b.Fatal(err)
	}
	lg, err := txlog.Open(filepath.Join(dir, "log"))
	if err != nil {
		b.Fatal(err)
	}
	a := New(st, lg)
	a.UserID = "u1"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.Create("person", map[string]string{"name": fmt.Sprintf("n%d", i), "age": fmt.Sprintf("%d", i%100)}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	st.Close()
}
