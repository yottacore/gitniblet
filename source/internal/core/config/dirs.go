// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package config is the on-disk database registry: per-database TOML records
// under a user (and optional system) config directory, plus the discovery that
// startup uses to offer a "which database?" picker when no DDL was named.
//
// A registered database is a directory <base>/ngdb/<name>/ holding config.shcl.
// The DDL, tx-log and attachments are the git-synced artifacts the record
// points at; the SQLite view and encryption key are local, unsynced files that
// default to living beside the record (both are rebuildable / re-fetchable).
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// appDir is the registry sub-directory under every config base.
const appDir = "ngdb"

// baseOverride, when set (by the --config flag via SetBaseOverride), replaces
// the OS-derived registry root entirely - used verbatim, no appDir appended.
var baseOverride string

// SetBaseOverride redirects the registry root for this run. Empty restores the
// default. Set it once at startup, before any List/Create/UserBase call.
func SetBaseOverride(path string) { baseOverride = path }

// UserBase is the writable registry root: the --config override if set, else
// <os-user-config>/ngdb. On Linux the OS part honours $XDG_CONFIG_HOME (else
// ~/.config); on Windows it is %AppData%; on macOS ~/Library/Application
// Support - os.UserConfigDir picks the per-OS location, we only append appDir.
func UserBase() (string, error) {
	if baseOverride != "" {
		return baseOverride, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

// SystemBases are read-only registry roots shared across users, searched after
// the user base during discovery. On Linux these come from $XDG_CONFIG_DIRS
// (default /etc/xdg); on Windows from %ProgramData%. Missing dirs are harmless -
// discovery just skips them.
func SystemBases() []string {
	if runtime.GOOS == "windows" {
		if programData := os.Getenv("ProgramData"); programData != "" {
			return []string{filepath.Join(programData, appDir)}
		}
		return nil
	}
	dirs := os.Getenv("XDG_CONFIG_DIRS")
	if dirs == "" {
		dirs = "/etc/xdg"
	}
	var out []string
	for _, dir := range strings.Split(dirs, string(os.PathListSeparator)) {
		if dir != "" {
			out = append(out, filepath.Join(dir, appDir))
		}
	}
	return out
}
