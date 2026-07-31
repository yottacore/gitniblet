// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// Listed is one discovered registry entry. An unopenable record still appears -
// with its name (or directory) and the reason - so the picker can show it
// greyed with an explanation rather than hiding a broken database.
type Listed struct {
	Name   string
	Dir    string
	Config *DBConfig // nil when config.shcl itself could not be read
	System bool      // from a read-only system base
	Err    error     // nil means openable; else why not
}

// List discovers every registered database under the user base and the system
// bases (user first). Each record is lightly validated: its DDL must exist and
// parse. The sqlite view and attachments are not required - they rebuild from
// the log on open - so their absence is never an error.
func List() []Listed {
	var out []Listed
	if base, err := UserBase(); err == nil {
		out = append(out, scanBase(base, false)...)
	}
	for _, base := range SystemBases() {
		out = append(out, scanBase(base, true)...)
	}
	// Stable, name-first order for a predictable picker.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// scanBase lists the immediate sub-directories of one base that hold a
// config.shcl. A missing base is not an error - it just yields nothing.
func scanBase(base string, system bool) []Listed {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []Listed
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, recordFile)); err != nil {
			continue // not a registered database
		}
		out = append(out, validate(dir, entry.Name(), system))
	}
	return out
}

// validate loads and sanity-checks one record.
func validate(dir, fallbackName string, system bool) Listed {
	listed := Listed{Name: fallbackName, Dir: dir, System: system}
	cfg, err := Load(dir)
	if err != nil {
		listed.Err = fmt.Errorf("unreadable config: %w", err)
		return listed
	}
	listed.Config = cfg
	listed.Name = cfg.Name
	if cfg.DDLPath == "" {
		listed.Err = fmt.Errorf("no ddl_path set")
		return listed
	}
	if _, err := os.Stat(cfg.DDLPath); err != nil {
		listed.Err = fmt.Errorf("DDL not found: %s", cfg.DDLPath)
		return listed
	}
	schema, err := ddl.ParseFile(cfg.DDLPath)
	if err != nil {
		listed.Err = fmt.Errorf("DDL invalid: %w", err)
		return listed
	}
	// A recoverable parse still yields a schema, but one with lines missing from
	// it is not a database to open silently - flag it so the picker can say so.
	if schema.HasErrors() {
		listed.Err = fmt.Errorf("DDL has %d problem(s): %s", schema.Errors, schema.Warnings[0])
		return listed
	}
	return listed
}

// FindByName returns the registered database whose name matches, or nil. The
// match is exact and case-sensitive (a name is a directory on disk); if that
// misses and the query carries a file extension (e.g. "issues.ddl"), the base
// name is tried too, so a user can name the database with or without an
// extension. This is how the front-ends refer to a database by a single name
// instead of spelling out its ddl/sqlite/log paths.
func FindByName(name string) *DBConfig {
	if name == "" {
		return nil
	}
	listed := List()
	for _, entry := range listed {
		if entry.Config != nil && entry.Config.Name == name {
			return entry.Config
		}
	}
	if base := strings.TrimSuffix(name, filepath.Ext(name)); base != name {
		for _, entry := range listed {
			if entry.Config != nil && entry.Config.Name == base {
				return entry.Config
			}
		}
	}
	return nil
}

// FindByDDL returns the registered record whose ddl_path matches ddlPath (by
// absolute path), or nil. This lets the stateless CLI verbs, handed only a DDL,
// find that database's key file and encryption preference in the registry
// rather than guessing a key beside the DDL (which would sit in the git repo).
func FindByDDL(ddlPath string) *DBConfig {
	abs := absPath(ddlPath)
	for _, listed := range List() {
		if listed.Config != nil && absPath(listed.Config.DDLPath) == abs {
			return listed.Config
		}
	}
	return nil
}

// PWDDdl reports a schema file sitting in the current directory, so a database
// in $PWD opens without being named. It needs exactly one candidate; zero or
// several are ambiguous and yield ("", false), leaving the caller to fall back
// to the registry picker or to require explicit paths.
//
// The schema is a .shcl file. The older .ddl extension is still recognized so a
// database predating the rename still opens - but having both is ambiguous, not
// a preference, so it bails rather than guessing which one is current.
func PWDDdl() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	var candidates []string
	for _, pattern := range []string{"*.shcl", "*.ddl"} {
		matches, err := filepath.Glob(filepath.Join(wd, pattern))
		if err != nil {
			return "", false
		}
		for _, match := range matches {
			// The queries sidecar shares the .shcl extension; it is not a schema.
			if isQueriesSidecar(match) {
				continue
			}
			candidates = append(candidates, match)
		}
	}
	if len(candidates) != 1 {
		return "", false
	}
	return candidates[0], true
}

// isQueriesSidecar reports whether a path is a <name>.queries.shcl sidecar
// rather than a schema.
func isQueriesSidecar(path string) bool {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return strings.HasSuffix(base, ".queries")
}

// LocalSQLite is the view path for an unregistered open: beside the DDL, named
// after it. The view is rebuilt from the log, so co-locating it is harmless.
func LocalSQLite(ddlPath string) string {
	ext := filepath.Ext(ddlPath)
	return ddlPath[:len(ddlPath)-len(ext)] + ".sqlite"
}

// LocalKey is the encryption-key path for an unregistered open: beside the DDL,
// named after it. Registered databases use DBConfig.KeyFile instead.
func LocalKey(ddlPath string) string {
	ext := filepath.Ext(ddlPath)
	return ddlPath[:len(ddlPath)-len(ext)] + ".key"
}

// PWDTriple resolves a database in the current directory to the
// (ddl, sqlite, logDir) triple the front-ends open. ok is false when $PWD has
// no unambiguous DDL, so the caller can fall back to the picker or an error.
func PWDTriple() (ddlPath, sqlitePath, logDir string, ok bool) {
	ddlPath, ok = PWDDdl()
	if !ok {
		return "", "", "", false
	}
	return ddlPath, LocalSQLite(ddlPath), filepath.Dir(ddlPath), true
}
