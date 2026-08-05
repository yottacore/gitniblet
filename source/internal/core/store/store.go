// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package store manages the local SQLite view. The git tx-log is the source of
// truth; this SQLite file is a derived, rebuildable cache.
//
// Build turns a parsed ddl.Schema into tables, and migrates existing ones
// additively (new DDL fields become ALTER TABLE ADD COLUMN). Every table gets
// the immutable system columns (id, is_active, date_created, is_deleted);
// unique indexes are partial (WHERE is_deleted=0) so soft-deleted rows never
// collide with live ones - or with each other.
package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// Store is the handle to the local SQLite view.
type Store struct {
	db   *sql.DB
	path string
	// refs are the ref-typed columns per table, learned as tables are built.
	// Replay needs them to bind a row reference as raw bytes rather than as its
	// text form; a plain binary column is a BLOB too, so the SQL type alone
	// cannot tell them apart. Build can run more than once (user schema, then
	// the built-ins), so this accumulates rather than resets.
	refs map[string]map[string]bool
}

// Open opens or creates the SQLite view at path.
//
// The pragmas go in the DSN rather than through Exec: the driver replays them on
// every pooled connection, while an Exec would only reach whichever one it
// happened to get. busy_timeout matters most - a second ngdb process holding the
// file would otherwise fail a write immediately instead of waiting its turn.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, path: path, refs: map[string]map[string]bool{}}, nil
}

// IsRef reports whether a column holds a row reference. False for a store whose
// tables were not built from a schema (the value then binds as text, which is
// what those callers already expect).
func (s *Store) IsRef(table, field string) bool { return s.refs[table][field] }

// Close releases the view.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB exposes the underlying handle (used by the CRUD layer and tests).
func (s *Store) DB() *sql.DB { return s.db }

// Build creates every table in the schema and additively migrates existing
// ones: fields added to the DDL reach a live view as ALTER TABLE ADD COLUMN,
// so a schema edit never strands an existing database (type changes and drops
// still need a rebuild-from-log). Relationship-driven indexes and the auto
// feature tables (m:m, comments, audit, ...) are handled separately.
func (s *Store) Build(schema *ddl.Schema) error {
	for _, table := range schema.Tables {
		if err := s.buildTable(table); err != nil {
			return fmt.Errorf("table %q: %w", table.Name, err)
		}
	}
	return nil
}

func (s *Store) buildTable(table ddl.Table) error {
	for _, f := range table.Fields {
		if f.Type == "ref" {
			if s.refs[table.Name] == nil {
				s.refs[table.Name] = map[string]bool{}
			}
			s.refs[table.Name][f.Name] = true
		}
	}
	stmts := []string{createTableSQL(table)}
	existing, err := s.columns(table.Name)
	if err != nil {
		return err
	}
	if existing != nil { // table already there - add whatever the DDL grew
		for _, f := range table.Fields {
			if !existing[f.Name] {
				stmts = append(stmts, "ALTER TABLE "+quoteIdent(table.Name)+
					" ADD COLUMN "+columnDef(f, true)+";")
			}
		}
	}
	stmts = append(stmts, indexStatements(table)...)
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("%w\n%s", err, stmt)
		}
	}
	return nil
}

// columns returns the existing column set of a table, or nil if it is absent.
func (s *Store) columns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out map[string]bool
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]bool{}
		}
		out[name] = true
	}
	return out, rows.Err()
}

func createTableSQL(table ddl.Table) string {
	cols := []string{`"id" BLOB PRIMARY KEY`}
	for _, f := range table.Fields {
		cols = append(cols, columnDef(f, false))
	}
	// system_fields: no (built-in audit_trail) keeps id but skips the rest.
	if !table.NoSystemFields {
		cols = append(cols,
			`"is_active" INTEGER NOT NULL DEFAULT 1`,
			`"date_created" TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			`"is_deleted" INTEGER NOT NULL DEFAULT 0`,
		)
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\t%s\n);",
		quoteIdent(table.Name), strings.Join(cols, ",\n\t"))
}

func indexStatements(table ddl.Table) []string {
	var stmts []string
	for _, group := range table.Uniques {
		if len(group) == 0 {
			continue
		}
		// Uniques used to fold is_deleted into the key, which capped each
		// natural key at ONE soft-deleted row; drop that shape if present.
		legacy := "ux_" + table.Name + "__" + strings.Join(group, "_") + "_is_deleted"
		stmts = append(stmts,
			"DROP INDEX IF EXISTS "+quoteIdent(legacy)+";",
			indexSQL(table.Name, group, true, !table.NoSystemFields))
	}
	for _, group := range table.Indexes {
		if len(group) == 0 {
			continue
		}
		stmts = append(stmts, indexSQL(table.Name, group, false, false))
	}
	return stmts
}

// columnDef renders one column. forAlter drops NOT NULL when there is no
// default: SQLite refuses ADD COLUMN NOT NULL without a non-null default, and
// existing rows need a value anyway.
func columnDef(field ddl.Field, forAlter bool) string {
	var b strings.Builder
	b.WriteString(quoteIdent(field.Name))
	b.WriteByte(' ')
	b.WriteString(sqlType(field.Type))
	def, hasDef := defaultLiteral(field)
	// null_ok defaults true, so only an explicit "no" makes a column NOT NULL.
	if field.NullOK != nil && !*field.NullOK && (!forAlter || hasDef) {
		b.WriteString(" NOT NULL")
	}
	if hasDef {
		b.WriteString(" DEFAULT ")
		b.WriteString(def)
	}
	return b.String()
}

func sqlType(typ string) string {
	switch typ {
	case "int", "bool":
		return "INTEGER"
	case "float":
		return "REAL"
	case "binary", "ref":
		return "BLOB"
	default: // string, datetime_*, unspecified
		return "TEXT"
	}
}

// defaultLiteral renders a field's DDL default as a SQL literal. Sentinels and
// function refs are skipped: @null is the column default anyway, and @previous
// and functions are computed by the app, not by SQLite. Testing the whole
// sentinel set matters - checking only @null let @previous through as the
// literal text on any column type SQLite would accept a string for.
func defaultLiteral(field ddl.Field) (string, bool) {
	def := strings.TrimSpace(field.Default)
	if def == "" || ddl.IsSentinel(def) || ddl.IsNull(def) {
		return "", false
	}
	if _, _, ok := ddl.AsFunc(def); ok {
		return "", false
	}
	switch field.Type {
	case "bool":
		if b, ok := ddl.AsBool(def); ok {
			if b {
				return "1", true
			}
			return "0", true
		}
	case "int":
		if i, ok := ddl.AsInt(def); ok {
			return strconv.Itoa(i), true
		}
	case "float":
		if n, ok := ddl.AsFloat(def); ok {
			return strconv.FormatFloat(n, 'g', -1, 64), true
		}
	default:
		s, _ := ddl.Unquote(def)
		return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
	}
	return "", false
}

// indexSQL renders one index. Uniques are partial - they only police live rows,
// so any number of soft-deleted rows can share a natural key. partial is off
// for system_fields:no tables, which have no is_deleted column.
func indexSQL(table string, fields []string, unique, partial bool) string {
	kind, prefix, where := "INDEX", "ix", ""
	if unique {
		kind, prefix = "UNIQUE INDEX", "ux"
		if partial {
			where = ` WHERE "is_deleted"=0`
		}
	}
	name := prefix + "_" + table + "__" + strings.Join(fields, "_")
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = quoteIdent(f)
	}
	return fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s (%s)%s;",
		kind, quoteIdent(name), quoteIdent(table), strings.Join(cols, ", "), where)
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
