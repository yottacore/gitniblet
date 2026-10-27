// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Every app a test builds reads the user's settings, and so the registry.
// Point the whole package at throwaway config dirs, not just the picker tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gitniblet-tui-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "AppData", "XDG_CONFIG_DIRS", "ProgramData"} {
		p := filepath.Join(dir, name)
		if err := os.Mkdir(p, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.Setenv(name, p); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
