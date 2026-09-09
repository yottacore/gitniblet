// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Tx-log garbage collection (design "GC is segment rotation"): a collection
// pass writes the surviving entries to a fresh write-once segment file and
// retires the old files whole. Nothing is ever edited in place, so the
// append-only property the git union merge depends on survives. Two clients
// collecting concurrently just produce duplicate segments; replay is
// idempotent, so the union converges and the next pass collapses them.
package txlog

import (
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GC returns the entries that survive a collection pass. Collectible: every
// entry of a row whose FINAL state is hard-deleted and whose newest entry is
// older than the cutoff (same timestamp format the log uses). Rows revived
// by a later create stay whole; recent deletes stay for the audit window.
//
// canon maps a table's former name to its current one, the way replay does;
// nil means no renames to account for. Without it a rename splits one row's
// history into two groups, and the half holding the delete can be collected
// while the half holding the create survives - which brings the row back.
// It is only consulted to decide what goes together: entries are never
// rewritten, because the log is immutable and a table name is part of what an
// encrypted value is sealed against.
func GC(entries []Entry, cutoff string, canon func(string) string) (keep []Entry, collected int) {
	if canon == nil {
		canon = func(table string) string { return table }
	}
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Date != sorted[j].Date {
			return sorted[i].Date < sorted[j].Date
		}
		return sorted[i].TxID < sorted[j].TxID
	})
	type rowState struct {
		dead    bool
		maxDate string
	}
	rows := map[string]*rowState{}
	for _, entry := range sorted {
		key := canon(entry.Table) + "\x00" + entry.RowID
		state := rows[key]
		if state == nil {
			state = &rowState{}
			rows[key] = state
		}
		switch entry.Op {
		case "create":
			state.dead = false
		case "delete":
			state.dead = true
		}
		if entry.Date > state.maxDate {
			state.maxDate = entry.Date
		}
	}
	for _, entry := range entries {
		state := rows[canon(entry.Table)+"\x00"+entry.RowID]
		if state.dead && state.maxDate < cutoff {
			collected++
			continue
		}
		keep = append(keep, entry)
	}
	return keep, collected
}

// CutoffDays renders the collection cutoff for entries older than n days.
func CutoffDays(n int) string {
	return time.Now().UTC().AddDate(0, 0, -n).Format("2006-01-02T15:04:05.000000000Z")
}

// Seal retires the live append file into a write-once segment, and returns its
// path (empty when there was nothing to seal). It is the first step of a
// collection pass: once the rename is done, an appending process opens a fresh
// txlog.csv, so the files the pass then reads and retires are ones nothing new
// is writing to. Without it, everything appended between reading the log and
// removing it is thrown away.
func (l *Log) Seal() (string, error) {
	if _, err := os.Stat(l.path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	seg, err := segmentPath(l.Dir())
	if err != nil {
		return "", err
	}
	if err := os.Rename(l.path, seg); err != nil {
		return "", err
	}
	return seg, nil
}

// Snapshot lists the sealed segments a collection pass works over, with the
// size each had when the pass started. Deliberately not the live txlog.csv:
// after a Seal everything written so far is in a segment, so leaving the live
// file out of the pass entirely means a write arriving mid-pass cannot be read,
// folded in, or removed. That is what keeps a pass from eating a concurrent
// write, rather than trying to race it.
func (l *Log) Snapshot() (*Snapshot, error) {
	segs, _ := filepath.Glob(filepath.Join(l.Dir(), "txlog-*.csv")) // constant pattern
	sort.Strings(segs)
	files := make([]logFile, 0, len(segs))
	for _, path := range segs {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		files = append(files, logFile{path: path, size: info.Size()})
	}
	return &Snapshot{files: files}, nil
}

// Snapshot is the set of segments one collection pass reads and retires.
type Snapshot struct {
	files []logFile
}

// ReadAll reads exactly the segments in the snapshot, in order.
func (s *Snapshot) ReadAll() ([]Entry, []string, error) {
	var out []Entry
	var warns []string
	for _, file := range s.files {
		entries, fileWarnings, err := readLogFile(file.path)
		warns = append(warns, fileWarnings...)
		if err != nil {
			return out, warns, err
		}
		out = append(out, entries...)
	}
	return out, warns, nil
}

// Rotate writes keep as a new segment and retires the segments in snap.
// Returns the segment path. It refuses if any of them changed since the
// snapshot - after a Seal the only writer that could still reach one is an
// append that was already in flight when the seal happened.
func (l *Log) Rotate(keep []Entry, snap *Snapshot) (string, error) {
	old := snap.files
	if err := unchanged(old); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return "", err
	}
	for _, entry := range keep {
		if err := w.Write(entry.record()); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}

	seg, err := segmentPath(l.Dir())
	if err != nil {
		return "", err
	}
	// Stage under a temporary name so a log that moved under us costs nothing
	// but a re-run - there is no half-rotated state to clean up by hand.
	tmp := seg + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	if err := unchanged(old); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, seg); err != nil {
		os.Remove(tmp)
		return "", err
	}
	for _, file := range old {
		if err := os.Remove(file.path); err != nil {
			return seg, err
		}
	}
	return seg, l.ensureLiveFile()
}

// segmentPath names a write-once segment. The timestamp keeps read order and
// the random suffix keeps two passes in the same second apart.
func segmentPath(dir string) (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("txlog-%s-%s.csv",
		time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(suffix))), nil
}

// logFile is one on-disk part with the size it had when the pass read it.
type logFile struct {
	path string
	size int64
}

// unchanged re-checks that nothing was appended to the files a pass is about
// to retire. The log only ever grows, so a size that moved is enough.
func unchanged(files []logFile) error {
	for _, file := range files {
		info, err := os.Stat(file.path)
		if err != nil {
			return err
		}
		if info.Size() != file.size {
			return fmt.Errorf("txlog: %s changed during collection; nothing was removed - re-run gc",
				filepath.Base(file.path))
		}
	}
	return nil
}

// ensureLiveFile puts an empty txlog.csv back after a pass retired it. Leaving
// the file in place rather than deleted matters for sync: a peer who appended
// to it on the same commit then sees an ordinary change the union driver can
// merge, instead of a modify/delete conflict no driver can resolve, which would
// wedge every later sync.
func (l *Log) ensureLiveFile() error {
	if _, err := os.Stat(l.path); err == nil {
		return nil // an append already recreated it
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return os.WriteFile(l.path, buf.Bytes(), 0o644)
}

// files lists the log's on-disk parts in read order: retired-in segments
// (name-sorted, so oldest rotation first) then the live append file.
func (l *Log) files() []string {
	segs, _ := filepath.Glob(filepath.Join(l.Dir(), "txlog-*.csv")) // the pattern is a constant, so it cannot be malformed
	sort.Strings(segs)
	if _, err := os.Stat(l.path); err == nil {
		segs = append(segs, l.path)
	}
	return segs
}
