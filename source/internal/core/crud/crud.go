// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package crud is the single internal API every front-end and the Lua host call.
// No front-end talks to store/txlog directly - it all funnels through here so
// behaviour can't drift between interfaces.
//
// Writes are log-first: each op appends field-granular entries to the tx-log
// (the source of truth) and then applies them to the SQLite view. If the apply
// step fails the log still holds the truth, so a later replay reconciles the
// view. Reads go straight to the view.
//
// Validation, access control, and DDL triggers/methods layer on top later; this
// is the plumbing.
package crud

import (
	"database/sql"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/guid"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// API is the shared CRUD surface over a store (view) and its tx-log (truth).
// Safe for concurrent use: writes serialize on an internal mutex so the
// log-then-view sequence of one commit never interleaves with another's.
type API struct {
	st  *store.Store
	log *txlog.Log

	mu       sync.Mutex
	lastDate string // last timestamp issued; see now()

	// features gates the opt-in behaviors (features.go); nil = all off.
	// access/fieldAccess carry the DDL rules access.go enforces.
	features    map[string]ddl.Features
	access      map[string]ddl.Access
	fieldAccess map[string]map[string]ddl.Access

	// UserID stamps the tx-log; front-ends set it to the current user.
	UserID string

	// HostID stamps the tx-log with the writing machine (one user can span
	// hosts). New() defaults it; front-ends rarely override.
	HostID string

	// Trigger, when set by the scripting host, fires around every write (trigger.go).
	Trigger Trigger

	// readOnly, when set, makes every write return ErrReadOnly while reads keep
	// working. A front-end flips it on to degrade a session to read-only (e.g. a
	// dismissed startup notice); the core itself never sets it.
	readOnly bool

	// Encryption state (crypt.go). cipher is the enterprise field-value cipher
	// (nil = none, and always nil in the open-source build); encPref is the
	// local on|off|auto preference; encSchema supplies the DDL always|never|auto
	// directives. encSchema != nil arms the encrypt pass on writes, so a DDL
	// `encryption: always` is enforced even with pref off / no cipher.
	cipher    enc.Cipher
	encPref   string
	encSchema *ddl.Schema
}

// New wires the API to an open store and its log.
func New(st *store.Store, lg *txlog.Log) *API {
	return &API{st: st, log: lg, HostID: DefaultHostID()}
}

// ErrReadOnly is what every write returns while the API is in read-only mode.
var ErrReadOnly = fmt.Errorf("crud: database is open read-only")

// SetReadOnly turns read-only mode on or off. A front-end sets it when a session
// is degraded to read-only; writes then return ErrReadOnly and reads keep going.
func (a *API) SetReadOnly(ro bool) { a.readOnly = ro }

// ReadOnly reports whether the API is in read-only mode.
func (a *API) ReadOnly() bool { return a.readOnly }

// DefaultUserID is the stamp when a front-end has nothing better:
// NANOGITDB_USER, else the OS username.
func DefaultUserID() string {
	if u := os.Getenv("NANOGITDB_USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

// DefaultHostID is the machine stamp: NANOGITDB_HOST, else the OS hostname.
func DefaultHostID() string {
	if h := os.Getenv("NANOGITDB_HOST"); h != "" {
		return h
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// Create inserts a new row, sets the given fields, and returns its hex id.
// The `id` field is managed here and ignored if present in fields.
func (a *API) Create(table string, fields map[string]string) (string, error) {
	if err := a.authorize(table, "", "write", fields); err != nil {
		return "", err
	}
	fields, err := a.runBefore(table, "", fields)
	if err != nil {
		return "", err
	}
	id := newID()
	entries := []txlog.Entry{a.entry(table, id, "", "create", "")}
	for _, field := range sortedKeys(fields) {
		if field == "id" {
			continue
		}
		entries = append(entries, a.entry(table, id, field, "update", fields[field]))
	}
	entries = append(entries, a.audit(table, id, "create", nil)...)
	if err := a.commit(entries); err != nil {
		return "", err
	}
	a.runAfter(table, id, fields)
	return id, nil
}

// Update sets one or more fields on an existing row.
func (a *API) Update(table, id string, fields map[string]string) error {
	if err := a.authorize(table, id, "write", fields); err != nil {
		return err
	}
	fields, err := a.runBefore(table, id, fields)
	if err != nil {
		return err
	}
	// audit next: it reads the OLD values, which the entries then overwrite
	audit := a.audit(table, id, "update", fields)
	var entries []txlog.Entry
	for _, field := range sortedKeys(fields) {
		if field == "id" {
			continue
		}
		entries = append(entries, a.entry(table, id, field, "update", fields[field]))
	}
	if len(entries) == 0 {
		return nil
	}
	if err := a.commit(append(entries, audit...)); err != nil {
		return err
	}
	a.runAfter(table, id, fields)
	return nil
}

// SetField is the one-field-at-a-time primitive.
func (a *API) SetField(table, id, field, value string) error {
	if field == "id" {
		return fmt.Errorf("crud: cannot set the id field")
	}
	if err := a.authorize(table, id, "write", map[string]string{field: value}); err != nil {
		return err
	}
	fields, err := a.runBefore(table, id, map[string]string{field: value})
	if err != nil {
		return err
	}
	value = fields[field]
	audit := a.audit(table, id, "update", fields)
	if err := a.commit(append([]txlog.Entry{a.entry(table, id, field, "update", value)}, audit...)); err != nil {
		return err
	}
	a.runAfter(table, id, fields)
	return nil
}

// SetFieldNull sets a column to SQL NULL - distinct from the empty string,
// which the log format could not express before the 🗦NULL🗧 sentinel.
func (a *API) SetFieldNull(table, id, field string) error {
	if field == "id" {
		return fmt.Errorf("crud: cannot set the id field")
	}
	if err := a.authorize(table, id, "write", map[string]string{field: ""}); err != nil {
		return err
	}
	audit := a.audit(table, id, "update", map[string]string{field: ""})
	entry := a.entry(table, id, field, "update", "")
	entry.IsNull = true
	return a.commit(append([]txlog.Entry{entry}, audit...))
}

// MarkDelete soft-deletes a row (is_deleted=1); it stays for the audit/GC window.
func (a *API) MarkDelete(table, id string) error {
	if err := a.authorize(table, id, "delete", nil); err != nil {
		return err
	}
	audit := a.audit(table, id, "mark_delete", nil)
	return a.commit(append([]txlog.Entry{a.entry(table, id, "", "mark_delete", "")}, audit...))
}

// Delete hard-deletes a row.
func (a *API) Delete(table, id string) error {
	if err := a.authorize(table, id, "delete", nil); err != nil {
		return err
	}
	audit := a.audit(table, id, "delete", nil) // snapshots the row before it goes
	return a.commit(append([]txlog.Entry{a.entry(table, id, "", "delete", "")}, audit...))
}

// Get returns one row (including system columns) keyed by column name, or
// ok=false if absent. Soft-deleted rows are still returned - callers filter.
func (a *API) Get(table, id string) (map[string]string, bool, error) {
	idBytes, err := guid.Decode(id)
	if err != nil {
		return nil, false, fmt.Errorf("crud: %w", err)
	}
	rows, err := a.st.DB().Query(`SELECT * FROM `+quoteIdent(table)+` WHERE "id"=?`, idBytes)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out, err := rowsToMaps(rows)
	if err != nil {
		return nil, false, err
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out[0], true, nil
}

// Query runs an arbitrary read against the view and returns rows as maps.
func (a *API) Query(query string, args ...any) ([]map[string]string, error) {
	rows, err := a.st.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToMaps(rows)
}

// QueryRows is Query plus the result's column order - grids rendering an
// arbitrary SELECT need it, since row maps lose it.
func (a *API) QueryRows(query string, args ...any) ([]string, []map[string]string, error) {
	rows, err := a.st.DB().Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	out, err := rowsToMaps(rows)
	return cols, out, err
}

// commit is the log-first write: append to the truth, then apply to the view.
// The mutex keeps concurrent commits from interleaving their log/view steps.
func (a *API) commit(entries []txlog.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if a.readOnly {
		return ErrReadOnly
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// The log gets encrypted copies (per the resolved policy); the view always
	// gets cleartext. A must-encrypt field with no key aborts before any write.
	toLog := entries
	if a.encSchema != nil {
		sealed, err := a.encryptEntries(entries)
		if err != nil {
			return err
		}
		toLog = sealed
	}
	if err := a.log.Append(toLog...); err != nil {
		return err
	}
	// Warnings can't happen here: these entries were just built against the
	// same schema this process opened the view with.
	_, err := txlog.Apply(a.st, entries)
	return err
}

func (a *API) entry(table, id, field, op, val string) txlog.Entry {
	return txlog.Entry{
		TxID:     newID(),
		Date:     a.now(),
		Table:    table,
		RowID:    id,
		Field:    field,
		Op:       op,
		NewValue: val,
		UserID:   a.UserID,
		HostName: a.HostID,
	}
}

// newID returns a UUID v7 in the log's wire form: time-ordered, so ids insert
// near the b-tree tail. Note the encoding does not sort in time order (base64url
// is not ASCII-ordered), so the equal-date tx_id tiebreak in txlog.Apply orders
// deterministically but arbitrarily. That costs nothing: dates are nanosecond
// and strictly increasing per client, so a tie only arises between two clients
// writing in the same nanosecond, where issue order is meaningless anyway. What
// matters is that every client breaks the tie identically, and string compare
// does.
func newID() string {
	u, err := uuid.NewV7()
	if err != nil { // entropy exhaustion only; random v4 is an acceptable fallback
		u = uuid.New()
	}
	return guid.Encode(u[:])
}

const tsLayout = "2006-01-02T15:04:05.000000000Z"

// now returns a strictly increasing UTC timestamp - a minimal hybrid logical
// clock. txlog.Apply orders by (date, tx_id); nanosecond precision separates
// sequential writes, and if the wall clock ever steps backwards (NTP), the last
// stamp is bumped by 1ns instead, so this client's own entries always replay in
// issue order.
func (a *API) now() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	stamp := time.Now().UTC().Format(tsLayout)
	if stamp <= a.lastDate {
		if prev, err := time.Parse(tsLayout, a.lastDate); err == nil {
			stamp = prev.Add(time.Nanosecond).UTC().Format(tsLayout)
		}
	}
	a.lastDate = stamp
	return stamp
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func rowsToMaps(rows *sql.Rows) ([]map[string]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(cols))
		for i, c := range cols {
			row[c] = valToString(c, vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// valToString renders a scanned value. The id column is binary, so it is shown
// as hex (matching the ids callers pass in); other columns render naturally.
func valToString(col string, v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		// The driver hands back []byte only for a BLOB column - id, a ref, or a
		// binary field - never for text, so this cannot mangle a string.
		return guid.Encode(t)
	case string:
		return t
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
