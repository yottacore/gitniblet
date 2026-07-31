// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"
)

// Deregister removes just the registry record (config.shcl) so the database no
// longer appears in discovery. The tx-log, local sqlite view, key file, and DDL
// are all left in place; DeleteFiles removes the on-disk footprint when asked.
func (c *DBConfig) Deregister() error {
	if c.dir == "" {
		return nil
	}
	if err := os.Remove(filepath.Join(c.dir, recordFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasFiles reports whether any of the database's own on-disk files exist - the
// tx-log dir, the local sqlite view, or the key - so a caller knows whether to
// even offer the "also delete the files" step. The DDL is excluded: it is the
// user's authored schema, not something this tool created.
func (c *DBConfig) HasFiles() bool {
	for _, path := range []string{c.LogDir, c.SQLitePath, c.KeyFile} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// DeleteFiles removes the database's on-disk footprint: the record directory
// (config.shcl, sqlite view, key file) and the tx-log directory. The DDL schema
// is deliberately kept - it is the user's authored source, often shared or in a
// repo. Missing paths are not an error. This implies Deregister, since the
// record directory goes with it.
func (c *DBConfig) DeleteFiles() error {
	if c.LogDir != "" {
		if err := os.RemoveAll(c.LogDir); err != nil {
			return err
		}
	}
	if c.dir != "" {
		if err := os.RemoveAll(c.dir); err != nil {
			return err
		}
	}
	return nil
}
