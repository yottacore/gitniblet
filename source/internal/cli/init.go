// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// Init registers a new database in the config registry - the flag-driven twin
// of the TUI picker's "Create new database". The schema is a lone *.shcl in the
// current directory (its base name is the database name); the optional arg says
// where the git-synced tx-log goes:
//
//	--init                in a git repo -> <repo-root>/ngdb/<name>; else $PWD
//	--init <repo-root>    -> <repo-root>/ngdb/<name>
//	--init <explicit-dir> -> that dir, verbatim
//
// After registering it materializes the view (the same build/bootstrap/seed the
// UIs do), so the database is immediately usable.
func Init(args []string) error {
	ddlPath, ok := config.PWDDdl()
	if !ok {
		return fmt.Errorf("--init: run this in a directory holding exactly one .shcl (the schema to register)")
	}
	name := dbName(ddlPath)

	var location string
	if len(args) > 0 {
		location = args[0]
	}
	logDir, err := config.LogDirFor(location, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}

	cfg, err := config.Create(name, ddlPath, logDir)
	if err != nil {
		return err
	}

	// --encrypt sets the local preference; --encrypt=on also mints the per-db
	// key (once) so the database is ready to encrypt.
	if pref := config.EncryptPref(); pref != "" {
		cfg.Encryption = pref
		if pref == "on" {
			if err := ensureKey(cfg.KeyFile); err != nil {
				return err
			}
		}
		if err := cfg.Save(); err != nil {
			return err
		}
	}

	fmt.Printf("registered %q\n", name)
	fmt.Printf("  config:  %s\n", filepath.Join(cfg.Dir(), "config.shcl"))
	fmt.Printf("  ddl:     %s\n", cfg.DDLPath)
	fmt.Printf("  tx-log:  %s\n", cfg.LogDir)
	fmt.Printf("  sqlite:  %s\n", cfg.SQLitePath)
	if cfg.Encryption != "" && cfg.Encryption != "auto" {
		fmt.Printf("  encrypt: %s (key: %s)\n", cfg.Encryption, cfg.KeyFile)
	}

	// Build the view now so the database opens cleanly next time; a build error
	// leaves the registration intact (the view rebuilds from the log on open).
	c, err := schema.OpenClient(cfg.DDLPath, cfg.SQLitePath, cfg.LogDir, "")
	if err != nil {
		fmt.Printf("  note: registered, but building the view failed: %v\n", err)
		fmt.Println("        fix the DDL and reopen; the registration stands")
		return nil
	}
	c.Close()

	if !txlog.InRepo(logDir) {
		fmt.Println("  note: the tx-log dir is not in a git repo; syncing stays off until it is")
	}
	return nil
}

// ensureKey mints a fresh per-db key file if one does not already exist. Key
// material is an enterprise capability: enc.MintKey errors in the open-source
// build (no provider registered), so --init --encrypt=on there fails cleanly
// instead of half-registering a database it cannot encrypt for.
func ensureKey(path string) error {
	if err := enc.MintKey(path); err != nil {
		return err
	}
	fmt.Printf("  encryption key ready: %s (share it out-of-band; never commit it)\n", path)
	return nil
}

// dbName derives the registry name from the DDL file's base name.
func dbName(ddlPath string) string {
	base := filepath.Base(ddlPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
