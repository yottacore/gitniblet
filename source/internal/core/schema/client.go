// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Client startup and table metadata shared by the interactive front-ends
// (TUI, web, Lua) so their bring-up sequence cannot drift.
package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// Client is one fully-opened database: view migrated, built-ins bootstrapped,
// log replayed, defaults seeded. Callers must Close it.
type Client struct {
	Schema   *ddl.Schema
	Store    *store.Store
	API      *crud.API
	Queries  []ddl.NamedQuery // the .queries sidecar, dropdown-ordered
	Warnings []string

	log *txlog.Log // for StartAutoSync
}

// OpenOpts parameterizes the bring-up. The zero value (only paths + userID set)
// means "no encryption": KeyFile empty -> no key loaded, EncryptPref empty ->
// auto. Registered databases pass their KeyFile and stored preference; ad-hoc
// opens pass config.LocalKey and the --encrypt override.
type OpenOpts struct {
	DDLPath, DBPath, LogDir, UserID string
	KeyFile                         string // encryption key file ("" = none)
	EncryptPref                     string // on|off|auto ("" = auto)
}

// OpenClient is the standard front-end bring-up with no encryption options.
func OpenClient(ddlPath, dbPath, logDir, userID string) (*Client, error) {
	return OpenClientWith(OpenOpts{DDLPath: ddlPath, DBPath: dbPath, LogDir: logDir, UserID: userID})
}

// OpenClientWith parses the DDL, opens and additively migrates the view,
// bootstraps the built-in tables, loads the encryption key (if any), replays
// the tx-log (decrypting field values along the way), then seeds the default
// groups (after the replay, so an already-seeded log is a no-op).
func OpenClientWith(opts OpenOpts) (*Client, error) {
	sch, err := ddl.ParseFile(opts.DDLPath)
	if err != nil {
		return nil, err
	}
	queries, queryWarns, err := ddl.ParseQueriesFile(ddl.QueriesPath(opts.DDLPath))
	if err != nil {
		return nil, err
	}
	st, err := store.Open(opts.DBPath)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Client, error) {
		st.Close()
		return nil, e
	}
	if err := st.Build(sch); err != nil {
		return fail(err)
	}
	bootstrapWarns, err := Bootstrap(st, sch)
	if err != nil {
		return fail(err)
	}
	logFile, err := txlog.Open(opts.LogDir)
	if err != nil {
		return fail(err)
	}
	builtins, err := Builtins()
	if err != nil {
		return fail(err)
	}
	cipher, hasKey, err := enc.LoadCipher(opts.KeyFile) // a corrupt key file is fatal
	if err != nil {
		return fail(err)
	}
	entries, readWarns, err := logFile.ReadAll()
	if err != nil {
		return fail(err)
	}
	ApplyAliases(entries, sch, builtins)               // pre-rename entries -> current names
	unreadable := crud.DecryptEntries(entries, cipher) // before Apply: view holds cleartext
	applyWarns, err := txlog.Apply(st, entries)
	if err != nil {
		return fail(err)
	}
	applyWarns = append(readWarns, applyWarns...)
	if opts.UserID == "" {
		opts.UserID = crud.DefaultUserID()
	}
	pref := opts.EncryptPref
	if pref == "" {
		pref = "auto"
	}
	api := crud.New(st, logFile)
	api.UserID = opts.UserID
	api.EnableEncryption(cipher, pref, sch)
	api.EnableFeatures(sch, builtins) // before seeding, so the seeded groups get audited
	if err := SeedDefaults(api); err != nil {
		return fail(err)
	}
	applyWarns = append(applyWarns, encryptionWarnings(sch, hasKey, pref, unreadable)...)
	return &Client{
		Schema:   sch,
		Store:    st,
		API:      api,
		Queries:  queries,
		Warnings: append(append(append(sch.Warnings, queryWarns...), bootstrapWarns...), applyWarns...),
		log:      logFile,
	}, nil
}

// encryptionWarnings reports the degraded/awareness states the design calls
// for: encrypted data the current run cannot read, an always-encrypted schema
// with no key (writes to those fields will be refused), and the "you have the
// key but turned encryption off" case.
func encryptionWarnings(sch *ddl.Schema, hasKey bool, pref string, unreadable int) []string {
	var warns []string
	if unreadable > 0 {
		if hasKey {
			warns = append(warns, fmt.Sprintf("%d encrypted value(s) failed to decrypt (wrong key or tampering); shown empty", unreadable))
		} else {
			warns = append(warns, fmt.Sprintf("%d encrypted value(s) unreadable - no key file found; shown empty. Obtain the key to read them", unreadable))
		}
	}
	if !hasKey && sch.HasAlwaysEncryption() {
		warns = append(warns, "this database has always-encrypted fields but no key file; those fields cannot be written until the key is provided")
	}
	if hasKey && pref == "off" {
		warns = append(warns, "encryption is available (key present) but disabled for you (--encrypt=off); new data is written in the clear")
	}
	return warns
}

// Close releases the client's store.
func (c *Client) Close() error { return c.Store.Close() }

// Catalog is the UI-facing table metadata: which tables exist, in what order,
// which fields are user-editable, which tables soft-delete, and the resolved
// renderable views.
type Catalog struct {
	Tables      []string
	Fields      map[string][]string     // editable (DDL-declared) fields per table
	HasDeleted  map[string]bool         // table has is_deleted (audit_trail does not)
	Features    map[string]ddl.Features // per-table opt-ins, for UI gating
	Views       []ViewSpec              // resolved views, DDL order
	DefaultView string                  // view the UIs open on start; "" only when no views resolved
	Queries     []ddl.NamedQuery        // predefined queries; front-ends copy Client.Queries in
	Warnings    []string                // view-resolution problems worth surfacing
}

// QueriesFor lists the active queries offered on a view, dropdown order.
// Queries with no view are offered everywhere.
func (c *Catalog) QueriesFor(view string) []ddl.NamedQuery {
	var out []ddl.NamedQuery
	for _, query := range c.Queries {
		if query.Active && (query.View == "" || query.View == view) {
			out = append(out, query)
		}
	}
	return out
}

// NamedQuery finds an active query by name, or nil.
func (c *Catalog) NamedQuery(name string) *ddl.NamedQuery {
	for i := range c.Queries {
		if c.Queries[i].Name == name && c.Queries[i].Active {
			return &c.Queries[i]
		}
	}
	return nil
}

// NewCatalog builds the catalog from the view plus one or more schemas.
// Earlier schemas win the ordering and field lists - pass the user schema
// first, then Builtins(). Only tables that exist in the view are listed.
func NewCatalog(api *crud.API, schemas ...*ddl.Schema) (*Catalog, error) {
	c := &Catalog{
		Fields:     map[string][]string{},
		HasDeleted: map[string]bool{},
		Features:   map[string]ddl.Features{},
	}
	rows, err := api.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, r := range rows {
		existing[r["name"]] = true
	}

	seen := map[string]bool{}
	tableAccess := map[string]ddl.Access{}
	fieldAccess := map[string]map[string]ddl.Access{}
	for _, s := range schemas {
		for _, table := range s.Tables {
			if _, dup := c.Fields[table.Name]; dup {
				continue // earlier schema (the user's) wins
			}
			names := make([]string, len(table.Fields))
			fieldAcc := map[string]ddl.Access{}
			for i, field := range table.Fields {
				names[i] = field.Name
				fieldAcc[field.Name] = field.Access
			}
			c.Fields[table.Name] = names
			c.Features[table.Name] = table.Features
			tableAccess[table.Name] = table.Access
			fieldAccess[table.Name] = fieldAcc
			if existing[table.Name] && !seen[table.Name] {
				seen[table.Name] = true
				c.Tables = append(c.Tables, table.Name)
			}
		}
	}
	var rest []string
	for name := range existing {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	c.Tables = append(c.Tables, rest...)

	for _, table := range c.Tables {
		cols, err := api.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			return nil, err
		}
		var plain []string
		for _, r := range cols {
			switch r["name"] {
			case "is_deleted":
				c.HasDeleted[table] = true
			case "id", "is_active", "date_created":
			default:
				plain = append(plain, r["name"])
			}
		}
		if len(c.Fields[table]) == 0 { // view-only fallback: everything but system cols
			c.Fields[table] = plain
		}
	}
	c.resolveViews(schemas...) // after Fields: tree_grid parent_field checks need them
	c.applyAccess(api, tableAccess, fieldAccess)
	c.resolveDefaultView(schemas...) // after access: the default must be openable
	return c, nil
}

// Has reports whether the catalog lists the table - the front-ends' guard
// against a table name arriving from user input.
func (c *Catalog) Has(table string) bool {
	_, ok := c.Fields[table]
	for _, t := range c.Tables {
		if t == table {
			return ok
		}
	}
	return false
}

// ColumnsFor is the grid column set: id first, then the editable fields.
func (c *Catalog) ColumnsFor(table string) []string {
	return append([]string{"id"}, c.Fields[table]...)
}

// LiveRows loads a table's visible rows; soft-deleted ones are hidden when
// the table has is_deleted, and row-level grants the user lacks hide theirs.
func (c *Catalog) LiveRows(api *crud.API, table string) ([]map[string]string, error) {
	query := `SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`
	if c.HasDeleted[table] {
		query += ` WHERE "is_deleted"=0`
	}
	rows, err := api.Query(query)
	if err != nil {
		return nil, err
	}
	return c.liveRowsAllowed(api, table, rows), nil
}
