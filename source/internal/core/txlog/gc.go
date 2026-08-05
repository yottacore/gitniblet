// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

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
func GC(entries []Entry, cutoff string) (keep []Entry, collected int) {
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
		key := entry.Table + "\x00" + entry.RowID
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
		state := rows[entry.Table+"\x00"+entry.RowID]
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

// Rotate writes keep as a new segment and removes the files it replaces.
// Returns the segment path. Future appends recreate txlog.csv; ReadAll walks
// segments plus the live file.
func (l *Log) Rotate(keep []Entry) (string, error) {
	old := l.files()
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
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	seg := filepath.Join(l.Dir(), fmt.Sprintf("txlog-%s-%s.csv",
		time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(suffix)))
	if err := os.WriteFile(seg, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	for _, path := range old {
		if err := os.Remove(path); err != nil {
			return seg, err
		}
	}
	return seg, nil
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
