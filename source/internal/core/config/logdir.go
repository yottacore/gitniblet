// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import (
	"os"
	"path/filepath"

	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// LogDirFor resolves where a database's tx-log should live, given a user-entered
// location and the database name. Point it at a git repo's top level and it gets
// the auto `ngdb/<name>` subfolder; any other path (a plain folder, or a spot
// inside a repo but not its root) is taken verbatim and created as-is. An empty
// location falls back to the current directory, still preferring the enclosing
// repo's subfolder. So the caller only ever asks for one location, never the
// tx-log path itself.
func LogDirFor(location, name string) (string, error) {
	if location != "" {
		path, err := filepath.Abs(location)
		if err != nil {
			return "", err
		}
		if root, ok := txlog.RepoRoot(path); ok && sameDir(root, path) {
			return filepath.Join(root, "ngdb", name), nil
		}
		return path, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root, ok := txlog.RepoRoot(wd); ok {
		return filepath.Join(root, "ngdb", name), nil
	}
	return wd, nil
}

// sameDir reports whether two paths denote the same directory, resolving
// symlinks where it can (a repo root reported by git may be canonicalized).
func sameDir(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
