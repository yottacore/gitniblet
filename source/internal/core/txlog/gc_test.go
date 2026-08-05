// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGCEligibility(t *testing.T) {
	// dates are lexical; cutoff "5" means "older than 5"
	entries := []Entry{
		// row A: deleted long ago -> all three collectible
		{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		{TxID: "2", Date: "2", Table: "t", RowID: "aa", Field: "f", Op: "update", NewValue: "x"},
		{TxID: "3", Date: "3", Table: "t", RowID: "aa", Op: "delete"},
		// row B: alive -> kept
		{TxID: "4", Date: "1", Table: "t", RowID: "bb", Op: "create"},
		// row C: deleted, then revived by a later create -> kept whole
		{TxID: "5", Date: "1", Table: "t", RowID: "cc", Op: "create"},
		{TxID: "6", Date: "2", Table: "t", RowID: "cc", Op: "delete"},
		{TxID: "7", Date: "3", Table: "t", RowID: "cc", Op: "create"},
		// row D: deleted RECENTLY -> kept for the audit window
		{TxID: "8", Date: "1", Table: "t", RowID: "dd", Op: "create"},
		{TxID: "9", Date: "7", Table: "t", RowID: "dd", Op: "delete"},
	}
	keep, collected := GC(entries, "5", nil)
	if collected != 3 {
		t.Fatalf("collected = %d, want 3", collected)
	}
	for _, e := range keep {
		if e.RowID == "aa" {
			t.Fatalf("row aa should be gone: %+v", e)
		}
	}
	if len(keep) != 6 {
		t.Fatalf("keep = %d entries, want 6: %+v", len(keep), keep)
	}
}

func TestRotateAndMultiSegmentRead(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
		Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create", Field: "f", NewValue: "kept"},
	); err != nil {
		t.Fatal(err)
	}
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	keep, collected := GC(entries, "9", nil)
	if collected != 2 || len(keep) != 1 {
		t.Fatalf("collected=%d keep=%d", collected, len(keep))
	}
	if _, err := lg.Seal(); err != nil {
		t.Fatal(err)
	}
	snap, err := lg.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	seg, err := lg.Rotate(keep, snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(seg), "txlog-") {
		t.Fatalf("segment name %q", seg)
	}
	// The live file stays in place but is emptied. Deleting it would reach a
	// peer as a modify/delete conflict, which no merge driver can resolve.
	if _, err := os.Stat(lg.Path()); err != nil {
		t.Fatalf("live txlog.csv should still be there after a rotation: %v", err)
	}
	live, _, err := readLogFile(lg.Path())
	if err != nil || len(live) != 0 {
		t.Fatalf("live txlog.csv should hold no entries after a rotation: %+v %v", live, err)
	}

	// the segment serves the reads; the emptied live file contributes nothing
	got, warns, err := lg.ReadAll()
	if err != nil || len(warns) != 0 {
		t.Fatalf("readall after rotate: %v %v", warns, err)
	}
	if len(got) != 1 || got[0].RowID != "bb" || got[0].NewValue != "kept" {
		t.Fatalf("survivors = %+v", got)
	}
	if err := lg.Append(Entry{TxID: "4", Date: "4", Table: "t", RowID: "cc", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	got, _, err = lg.ReadAll()
	if err != nil || len(got) != 2 {
		t.Fatalf("segment+live read = %+v err=%v", got, err)
	}
}

// A GC rotation must reach other clients through the normal sync (the whole
// dir is staged: new segment, deleted live file).
func TestRotationSyncs(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	mustGit(t, base, "init", "--bare", remote)
	dirA := clone(t, base, remote, "a")

	lgA, err := Open(dirA)
	if err != nil {
		t.Fatal(err)
	}
	if err := lgA.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
		Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	entries, _, _ := lgA.ReadAll()
	keep, _ := GC(entries, "9", nil)
	if _, err := lgA.Seal(); err != nil {
		t.Fatal(err)
	}
	snapA, err := lgA.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lgA.Rotate(keep, snapA); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	dirB := clone(t, base, remote, "b")
	lgB, err := Open(dirB)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := lgB.ReadAll()
	if err != nil || len(got) != 1 || got[0].RowID != "bb" {
		t.Fatalf("client B after rotation sync = %+v err=%v", got, err)
	}
	liveB, _, err := readLogFile(lgB.Path())
	if err != nil || len(liveB) != 0 {
		t.Fatalf("client B's live txlog.csv should hold no entries after the rotation: %+v %v", liveB, err)
	}
}

// A rename splits one row's history across two table names. Grouped naively,
// the half holding the delete is collected while the half holding the create
// survives - and the row comes back on the next replay.
func TestGCGroupsARenamedTableAsOneRow(t *testing.T) {
	entries := []Entry{
		{TxID: "1", Date: "1", Table: "task", RowID: "rr", Op: "create"},
		{TxID: "2", Date: "2", Table: "task", RowID: "rr", Field: "f", Op: "update", NewValue: "x"},
		{TxID: "3", Date: "3", Table: "job", RowID: "rr", Field: "f", Op: "update", NewValue: "y"},
		{TxID: "4", Date: "4", Table: "job", RowID: "rr", Op: "delete"},
	}
	canon := func(table string) string {
		if table == "task" {
			return "job"
		}
		return table
	}

	keep, collected := GC(entries, "9", canon)
	if collected != 4 || len(keep) != 0 {
		t.Fatalf("a renamed row should be collected whole: collected=%d keep=%+v", collected, keep)
	}

	// Without the mapping the two halves are separate rows, and what survives
	// is a create with no delete left to cancel it.
	keep, _ = GC(entries, "9", nil)
	var sawCreate, sawDelete bool
	for _, e := range keep {
		switch e.Op {
		case "create":
			sawCreate = true
		case "delete":
			sawDelete = true
		}
	}
	if !sawCreate || sawDelete {
		t.Fatalf("expected the ungrouped case to strand a create: keep=%+v", keep)
	}
}

// Entries are copied through a pass untouched. A table name is part of what an
// encrypted value is sealed against, so rewriting one to its current name here
// would strand every encrypted value under it.
func TestGCNeverRewritesEntries(t *testing.T) {
	entries := []Entry{
		{TxID: "1", Date: "1", Table: "task", RowID: "aa", Op: "create"},
		{TxID: "2", Date: "2", Table: "job", RowID: "bb", Op: "create"},
	}
	canon := func(string) string { return "job" }
	keep, collected := GC(entries, "0", canon)
	if collected != 0 || len(keep) != 2 {
		t.Fatalf("nothing was due for collection: collected=%d keep=%d", collected, len(keep))
	}
	if keep[0].Table != "task" || keep[1].Table != "job" {
		t.Fatalf("table names were rewritten: %+v", keep)
	}
}

// Sealing is what makes a pass safe: once the live file is retired, an append
// opens a fresh one that the pass never reads or removes.
func TestSealDivertsLaterAppends(t *testing.T) {
	lg, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	sealed, err := lg.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "" {
		t.Fatal("a live file was there to seal")
	}
	if _, err := os.Stat(lg.Path()); !os.IsNotExist(err) {
		t.Fatal("sealing retires the live file")
	}
	if err := lg.Append(Entry{TxID: "2", Date: "2", Table: "t", RowID: "bb", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	// The new write went to a fresh file, not into the sealed one.
	sealedEntries, _, err := readLogFile(sealed)
	if err != nil || len(sealedEntries) != 1 || sealedEntries[0].RowID != "aa" {
		t.Fatalf("sealed segment = %+v err=%v", sealedEntries, err)
	}
	all, _, err := lg.ReadAll()
	if err != nil || len(all) != 2 {
		t.Fatalf("both entries should still be readable: %+v err=%v", all, err)
	}

	// Sealing an empty log is a no-op rather than an error.
	empty, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if seg, err := empty.Seal(); err != nil || seg != "" {
		t.Fatalf("sealing nothing: seg=%q err=%v", seg, err)
	}
}

// The bug this guards: a pass used to read the log, decide, then delete the
// files it read - so anything written in between went with them.
func TestWriteDuringACollectionPassSurvives(t *testing.T) {
	dir := t.TempDir()
	lg, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
	); err != nil {
		t.Fatal(err)
	}

	// A pass begins: seal, then snapshot what it will work over.
	if _, err := lg.Seal(); err != nil {
		t.Fatal(err)
	}
	snap, err := lg.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := snap.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	keep, collected := GC(entries, "9", nil)
	if collected != 2 || len(keep) != 0 {
		t.Fatalf("collected=%d keep=%d", collected, len(keep))
	}

	// Another process writes while the pass is deciding.
	if err := lg.Append(Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create"}); err != nil {
		t.Fatal(err)
	}

	if _, err := lg.Rotate(keep, snap); err != nil {
		t.Fatal(err)
	}

	got, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RowID != "bb" {
		t.Fatalf("the write made during the pass should have survived: %+v", got)
	}
}

// The one writer a seal cannot divert is an append that already had the file
// open. If that lands in a segment the pass is retiring, the pass must back out
// rather than take the write with it.
func TestRotateRefusesWhenASealedSegmentGrew(t *testing.T) {
	dir := t.TempDir()
	lg, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Append(
		Entry{TxID: "1", Date: "1", Table: "t", RowID: "aa", Op: "create"},
		Entry{TxID: "2", Date: "2", Table: "t", RowID: "aa", Op: "delete"},
	); err != nil {
		t.Fatal(err)
	}
	sealed, err := lg.Seal()
	if err != nil {
		t.Fatal(err)
	}
	snap, err := lg.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := snap.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := GC(entries, "9", nil)

	// Stand in for the in-flight append: the sealed segment grows after the
	// snapshot was taken.
	file, err := os.OpenFile(sealed, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("x,1,t,cc,,create,,u,,h\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()

	if _, err := lg.Rotate(keep, snap); err == nil {
		t.Fatal("rotating over a segment that grew should fail")
	}
	if _, err := os.Stat(sealed); err != nil {
		t.Fatalf("a refused rotation must leave the segment in place: %v", err)
	}
	strays, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(strays) != 0 {
		t.Fatalf("a refused rotation left staged files behind: %v", strays)
	}
	got, _, err := lg.ReadAll()
	if err != nil || len(got) != 3 {
		t.Fatalf("nothing should have been lost: %+v err=%v", got, err)
	}
}

// A collection pass removes content, and a peer may have appended to the same
// file meanwhile. Deleting the live file makes that a modify/delete conflict,
// which no merge driver resolves - so the peer's sync fails, and keeps failing.
func TestRotationDoesNotWedgeAPeersSync(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	mustGit(t, base, "init", "--bare", remote)

	dirA := clone(t, base, remote, "a")
	lgA, err := Open(dirA)
	if err != nil {
		t.Fatal(err)
	}
	// Enough entries that the rotation is a real shrink. Git pairs a delete
	// with an add as a rename when the two look alike, which would hide the
	// conflict on a log of two lines but not on a real one.
	var seed []Entry
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("r%02d", i)
		seed = append(seed,
			Entry{TxID: id + "c", Date: "1", Table: "t", RowID: id, Op: "create"},
			Entry{TxID: id + "d", Date: "2", Table: "t", RowID: id, Op: "delete"})
	}
	seed = append(seed, Entry{TxID: "keep", Date: "3", Table: "t", RowID: "live", Op: "create"})
	if err := lgA.Append(seed...); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	// B starts from the same commit and writes its own entry, unsynced.
	dirB := clone(t, base, remote, "b")
	lgB, err := Open(dirB)
	if err != nil {
		t.Fatal(err)
	}
	if err := lgB.Append(Entry{TxID: "3", Date: "3", Table: "t", RowID: "bb", Op: "create"}); err != nil {
		t.Fatal(err)
	}

	// A collects and publishes the rotation.
	if _, err := lgA.Seal(); err != nil {
		t.Fatal(err)
	}
	snap, err := lgA.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := snap.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	keep, collected := GC(entries, "9", nil)
	if collected != 40 {
		t.Fatalf("collected = %d, want 40", collected)
	}
	if _, err := lgA.Rotate(keep, snap); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(lgA, 0).Sync(); err != nil {
		t.Fatal(err)
	}

	// B syncs into the rotation. This is what used to abort and stay aborted.
	if _, err := NewSyncer(lgB, 0).Sync(); err != nil {
		t.Fatalf("peer sync after a rotation: %v", err)
	}
	// A second pass must work too - a wedged merge shows up here.
	if _, err := NewSyncer(lgB, 0).Sync(); err != nil {
		t.Fatalf("peer sync is wedged: %v", err)
	}

	got, _, err := lgB.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var kept bool
	for _, e := range got {
		if e.RowID == "bb" {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("the peer's own write was lost in the rotation merge: %+v", got)
	}
}
