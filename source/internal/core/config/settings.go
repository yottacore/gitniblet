// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"

	shcl "github.com/jim-collier/shcl/source/go"
)

// settingsFile holds user-global (not per-database) preferences. It sits at the
// registry root beside the per-db <name>/ dirs; discovery only scans dirs, so a
// plain file here is never mistaken for a database.
const settingsFile = "settings.shcl"

// Settings is the user-global preferences record. Kept deliberately small - only
// things that are the same across every database this user opens.
type Settings struct {
	Theme   string // TUI theme name; empty = the built-in default
	WebMode string // "local" (default) or "proxied"; see WebModeProxied
}

// WebModeProxied reports whether the web UI should require a login. It is the one
// explicit switch between the two deployment shapes: "local" (default) auto-
// identifies the single local user with no password; "proxied" requires a
// username and password because the server sits behind a reverse proxy. Anything
// but the exact word "proxied" is treated as local, so a typo fails safe toward
// the passwordless-but-header-guarded local mode rather than a broken login wall.
func (s *Settings) WebModeProxied() bool { return s.WebMode == "proxied" }

func settingsPath() (string, error) {
	base, err := UserBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, settingsFile), nil
}

// LoadSettings reads the global settings, returning zero-value defaults when the
// file is missing or unreadable (a first run, or a fresh --config dir). It never
// errors, so callers can use the result directly.
func LoadSettings() *Settings {
	s := &Settings{}
	path, err := settingsPath()
	if err != nil {
		return s
	}
	doc := loadLenient(path)
	s.Theme = doc.GetStringOr("theme", "")
	s.WebMode = doc.GetStringOr("web_mode", "")
	return s
}

// Save writes the global settings, creating the registry root if needed.
func (s *Settings) Save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	doc := shcl.New()
	setIfSet(doc, "theme", s.Theme)
	setIfSet(doc, "web_mode", s.WebMode)
	return save(path, doc, 0o644)
}
