// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"testing"

	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// The default (index 0) must be a dark theme, per the requirement.
func TestDefaultThemeIsDark(t *testing.T) {
	if len(themes) == 0 || !themes[0].dark {
		t.Fatalf("theme 0 must exist and be dark, got %+v", themes)
	}
	if got := themeIndexByName(""); got != 0 {
		t.Fatalf("empty name should resolve to the default 0, got %d", got)
	}
	if got := themeIndexByName("no-such-theme"); got != 0 {
		t.Fatalf("unknown name should fall back to 0, got %d", got)
	}
}

// setTheme swaps the live styles, persists the choice, and survives a rebuild.
func TestSetThemePersistsAndApplies(t *testing.T) {
	isolateConfig(t) // keep the settings write inside a temp dir
	api, sch, bs := setup(t)
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}

	pick := len(themes) - 1 // a light theme, distinct from the default dark
	a.setTheme(pick)

	if a.themeIdx != pick {
		t.Fatalf("themeIdx = %d, want %d", a.themeIdx, pick)
	}
	if tview.Styles != themes[pick].styles {
		t.Fatal("global tview.Styles was not switched to the picked theme")
	}
	if got := config.LoadSettings().Theme; got != themes[pick].name {
		t.Fatalf("persisted theme = %q, want %q", got, themes[pick].name)
	}
	// A fresh app must come up on the saved theme.
	a2, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	if a2.themeIdx != pick {
		t.Fatalf("reloaded themeIdx = %d, want %d", a2.themeIdx, pick)
	}
}

// The picker opens as its own page without disturbing the event loop.
func TestThemePickerOpens(t *testing.T) {
	isolateConfig(t)
	api, sch, bs := setup(t)
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	a.themePicker()
	if !a.pages.HasPage("theme") {
		t.Fatal("themePicker did not add the theme page")
	}
}
