// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package txlog is the git-backed CSV transaction log - the source of truth the
// SQLite view is rebuilt from.
//
// The log is append-only and field-granular: each entry records one write. The
// SQLite database is a derived view, produced by replaying entries in order
// (Apply). Git sync and garbage collection are separate concerns.
//
// The original design's field list omits a row identifier, but field-level ops
// can't be applied without one, so RowID is part of every entry (the id of the
// affected row; see the guid package for the wire form).
package txlog

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/guid"
	"github.com/jim-collier/nano-git-db/internal/core/store"
)

// Entry is one transaction-log row. Field is blank for record-level ops
// (create with no value, mark_delete, delete).
type Entry struct {
	TxID     string // per-entry id; see the guid package for the wire form
	Date     string // GMT, RFC3339
	Table    string
	RowID    string // id of the affected row; see the guid package for the wire form
	Field    string
	Op       string // create, update, mark_delete, delete
	NewValue string
	IsNull   bool // set the field to SQL NULL; NewValue is ignored
	UserID   string
	GCReady  bool   // ok_to_garbage_collect
	HostName string // machine that wrote the entry; one user can span hosts
	// Enc marks NewValue as an encrypted token (crypt-layer base64), stored
	// behind encMarker. The txlog itself does no crypto - it only carries the
	// flag and the opaque payload; the crud/crypt layer seals and opens it.
	Enc bool
}

var header = []string{
	"tx_id", "date", "table_name", "row_id", "field_name",
	"operation", "new_value", "user_id", "ok_to_garbage_collect", "host_name",
}

// canonicalCols is the name->index map for the built-in header, used as the
// default before a file's own header row is seen.
var canonicalCols = colsFrom(header)

// colsFrom builds a column-name -> index map from a header record (first wins on
// a duplicate name). Reading maps every field through this map, so the on-disk
// column order can change, and columns can be added or dropped, without breaking
// older or newer clients: a missing column defaults to empty and an unknown one
// is simply ignored. tx_id stays the first column by convention - it is the
// marker readLogFile uses to spot a header row.
func colsFrom(rec []string) map[string]int {
	m := make(map[string]int, len(rec))
	for i, name := range rec {
		if _, dup := m[name]; !dup {
			m[name] = i
		}
	}
	return m
}

// field reads a named column from a record via its header map, empty if the
// column is absent (a newer/older layout) or the record is short.
func field(rec []string, cols map[string]int, name string) string {
	if i, ok := cols[name]; ok && i < len(rec) {
		return rec[i]
	}
	return ""
}

// requiredCols are the columns a data record must carry to be usable; a row too
// short to hold them all (under the active header's layout) is treated as torn.
// The trailing optional columns (ok_to_garbage_collect, host_name) default when
// absent, which is how a narrower legacy row reads under a wider header.
var requiredCols = []string{
	"tx_id", "date", "table_name", "row_id", "field_name", "operation", "new_value", "user_id",
}

// mergedCols is the read map for a header row: the header's own names win, and
// any canonical column the header omits falls back to its canonical index. So a
// column added in place (a wider row under an older, narrower header) is still
// read, reorders follow the header, and unknown extra columns are ignored.
func mergedCols(rec []string) map[string]int {
	m := colsFrom(rec)
	for name, idx := range canonicalCols {
		if _, ok := m[name]; !ok {
			m[name] = idx
		}
	}
	return m
}

// isHeaderRow spots a header line without depending on column order: a data row
// can never hold all four of these reserved column names at once (its table and
// field names plus its one value give at most three user-controlled cells, and
// the operation cell is a fixed verb), so the test is collision-proof.
func isHeaderRow(rec []string) bool {
	var tx, date, table, op bool
	for _, s := range rec {
		switch s {
		case "tx_id":
			tx = true
		case "date":
			date = true
		case "table_name":
			table = true
		case "operation":
			op = true
		}
	}
	return tx && date && table && op
}

// minWidth is how many fields a record needs to carry every required column
// under the given layout.
func minWidth(cols map[string]int) int {
	w := 0
	for _, name := range requiredCols {
		if i, ok := cols[name]; ok && i+1 > w {
			w = i + 1
		}
	}
	return w
}

// Log is an append-only CSV transaction log in a directory.
type Log struct {
	path string
}

// Open attaches to (creating if needed) the log directory; the log file is
// txlog.csv within it.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Log{path: filepath.Join(dir, "txlog.csv")}, nil
}

// Path returns the CSV file path.
func (l *Log) Path() string { return l.path }

// Dir returns the log directory - the sync/attachments base path.
func (l *Log) Dir() string { return filepath.Dir(l.path) }

// Append writes entries to the end of the log, adding the header to a new file.
// The batch is rendered in memory and goes out as a single write(2) on an
// O_APPEND handle, so concurrent appenders (another process, the web server)
// cannot interleave partial records on a local filesystem.
func (l *Log) Append(entries ...Entry) error {
	_, statErr := os.Stat(l.path)
	newFile := errors.Is(statErr, os.ErrNotExist)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if newFile {
		if err := w.Write(header); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := w.Write(entry.record()); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(buf.Bytes())
	return err
}

// ReadAll returns every entry - retired GC segments first, then the live
// file - plus a warning per line it had to skip. A missing log is not an
// error.
func (l *Log) ReadAll() ([]Entry, []string, error) {
	var out []Entry
	var warns []string
	for _, path := range l.files() {
		fileEntries, fileWarnings, err := readLogFile(path)
		warns = append(warns, fileWarnings...)
		if err != nil {
			return out, warns, err
		}
		out = append(out, fileEntries...)
	}
	return out, warns, nil
}

// readLogFile reads one CSV part. Deliberately forgiving: the log is the
// single source of truth, so one mangled line (a torn union merge, a stray
// edit) must not make the whole log unreadable. Malformed records skip with
// a warning; repeated header rows (a union merge can leave extras) skip
// silently; only I/O errors abort.
func readLogFile(path string) ([]Entry, []string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	// cols/minw track the layout of the most recent header row, so a file (or a
	// union merge of files) whose columns were reordered, added, or dropped
	// still reads correctly - each data row is decoded against the header above
	// it, and a row only needs enough fields to carry the required columns.
	cols := canonicalCols
	minw := minWidth(cols)

	var out []Entry
	var warns []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var perr *csv.ParseError
		if errors.As(err, &perr) {
			warns = append(warns, fmt.Sprintf(
				"log %s line %d: unreadable, skipped (%v)", path, perr.Line, perr.Err))
			continue // one bad line must not take the log hostage
		}
		if err != nil {
			return nil, warns, err
		}
		if isHeaderRow(rec) {
			cols = mergedCols(rec) // a header row: adopt its column layout
			minw = minWidth(cols)
			continue
		}
		if len(rec) < minw {
			line, _ := r.FieldPos(0)
			warns = append(warns, fmt.Sprintf(
				"log %s line %d: torn record (%d fields, need at least %d), skipped", path, line, len(rec), minw))
			continue
		}
		out = append(out, entryFrom(rec, cols))
	}
	return out, warns, nil
}

func (e Entry) record() []string {
	var val string
	switch {
	case e.IsNull:
		val = nullMarker
	case e.Enc:
		val = encMarker + e.NewValue // token is line-safe base64; stored raw
	default:
		val = encodeSym(e.NewValue)
	}
	return []string{
		e.TxID, e.Date, encodeSym(e.Table), e.RowID, encodeSym(e.Field),
		e.Op, val, encodeSym(e.UserID), strconv.FormatBool(e.GCReady), encodeSym(e.HostName),
	}
}

// entryFrom decodes a data record against its header's column map, so every
// field is read by name rather than a fixed position.
func entryFrom(rec []string, cols map[string]int) Entry {
	gc, _ := strconv.ParseBool(field(rec, cols, "ok_to_garbage_collect"))
	entry := Entry{
		TxID:     field(rec, cols, "tx_id"),
		Date:     field(rec, cols, "date"),
		Table:    decodeSym(field(rec, cols, "table_name")),
		RowID:    field(rec, cols, "row_id"),
		Field:    decodeSym(field(rec, cols, "field_name")),
		Op:       field(rec, cols, "operation"),
		UserID:   decodeSym(field(rec, cols, "user_id")),
		HostName: decodeSym(field(rec, cols, "host_name")),
		GCReady:  gc,
	}
	// The NULL and ENC markers are checked on the raw text: a user literal
	// 🗦NULL🗧/🗦ENC🗧 was encoded with escaped delimiters, so it cannot collide.
	val := field(rec, cols, "new_value")
	switch {
	case val == nullMarker:
		entry.IsNull = true
	case strings.HasPrefix(val, encMarker):
		entry.Enc = true
		entry.NewValue = val[len(encMarker):] // opaque token; crypt opens it later
	default:
		entry.NewValue = decodeSym(val)
	}
	return entry
}

// Apply replays entries into the store's SQLite view, in one transaction. The
// view is derived, so this is how the log's truth reaches the queryable copy.
//
// Entries are applied in canonical (date, tx_id) order, not file order. That is
// what makes the view converge across clients: a git union-merge of two clients'
// appends can interleave lines arbitrarily, but every client sorts to the same
// total order before applying.
//
// Replay is the most forgiving code in the tree - the log must always be able
// to rebuild a view. Entries that cannot apply (schema drift, unique collisions
// from merged concurrent writes) are skipped and reported as warnings, never
// fatal; every client skips the same losers because the order is total. A
// hard-deleted row stays dead: later update/mark_delete entries for it are
// skipped rather than resurrecting a ghost via the upsert path (only a later
// create revives the row id).
func Apply(st *store.Store, entries []Entry) ([]string, error) {
	ordered := make([]Entry, len(entries))
	copy(ordered, entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Date != ordered[j].Date {
			return ordered[i].Date < ordered[j].Date
		}
		return ordered[i].TxID < ordered[j].TxID
	})

	tx, err := st.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // no-op after a successful Commit

	var warns []string
	dead := map[string]bool{} // (table, row) pairs hard-deleted so far
	for _, entry := range ordered {
		key := entry.Table + "\x00" + entry.RowID
		switch entry.Op {
		case "create":
			delete(dead, key)
		case "delete":
			dead[key] = true
		default:
			if dead[key] {
				continue
			}
		}
		if err := applyOne(tx, st, entry); err != nil {
			if skippable(err) {
				warns = append(warns, fmt.Sprintf("tx %s (%s/%s) skipped: %v", entry.TxID, entry.Table, entry.Op, err))
				continue
			}
			return warns, fmt.Errorf("tx %s (%s/%s): %w", entry.TxID, entry.Table, entry.Op, err)
		}
	}
	return warns, tx.Commit()
}

// errBadEntry tags data errors local to one log entry (mangled row id, an
// operation this build does not know - possibly written by a newer client).
// They skip with a warning like schema drift does: one bad or foreign entry
// must never stop the whole log from rebuilding the view.
var errBadEntry = errors.New("bad log entry")

// skippable classifies per-entry errors replay tolerates: unknown tables or
// columns (the design says DDL drift is ignored, and Build only migrates
// additively), unique violations (two clients created the same natural key;
// the deterministic order picks the same winner everywhere), and entry-local
// data errors. SQLite rolls back only the failed statement, not the enclosing
// transaction.
func skippable(err error) bool {
	if errors.Is(err, errBadEntry) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

func applyOne(tx *sql.Tx, st *store.Store, entry Entry) error {
	id, err := guid.Decode(entry.RowID)
	if err != nil {
		return fmt.Errorf("%w: %w", errBadEntry, err)
	}
	tbl := quoteIdent(entry.Table)

	switch entry.Op {
	case "create":
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+tbl+` ("id") VALUES (?)`, id); err != nil {
			return err
		}
		if entry.Field != "" {
			return setField(tx, tbl, entry, id, st.IsRef(entry.Table, entry.Field))
		}
		return nil
	case "update":
		// Upsert: replay must tolerate the create entry having been GC'd.
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+tbl+` ("id") VALUES (?)`, id); err != nil {
			return err
		}
		return setField(tx, tbl, entry, id, st.IsRef(entry.Table, entry.Field))
	case "mark_delete":
		_, err := tx.Exec(`UPDATE `+tbl+` SET "is_deleted"=1 WHERE "id"=?`, id)
		return err
	case "delete":
		_, err := tx.Exec(`DELETE FROM `+tbl+` WHERE "id"=?`, id)
		return err
	default:
		return fmt.Errorf("%w: unknown operation %q", errBadEntry, entry.Op)
	}
}

// setField writes one column. Values bind as text (SQLite coerces by column
// affinity), or as NULL when the entry carries the 🗦NULL🗧 sentinel. A still-
// encrypted entry (Enc: the decrypt pass had no key) also binds NULL - the view
// must never hold ciphertext. A ref column binds the raw id bytes instead, so a
// reference is stored the same way the row's own primary key is.
func setField(tx *sql.Tx, quotedTable string, entry Entry, id []byte, isRef bool) error {
	var val any = entry.NewValue
	switch {
	case entry.IsNull || entry.Enc:
		val = nil
	case isRef && entry.NewValue != "":
		raw, err := guid.Decode(entry.NewValue)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", errBadEntry, entry.Field, err)
		}
		val = raw
	}
	_, err := tx.Exec(`UPDATE `+quotedTable+` SET `+quoteIdent(entry.Field)+`=? WHERE "id"=?`, val, id)
	return err
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
