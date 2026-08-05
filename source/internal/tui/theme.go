// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Selectable, high-readability color themes. Colors are explicit RGB, not the
// named terminal-palette colors (ColorBlack/White) tview defaults to - those
// map to whatever the terminal theme happens to be, which is exactly what makes
// the default hard to read. Fixed RGB keeps every theme legible regardless of
// the surrounding terminal. 'T' opens the picker; the choice persists globally.
package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

type theme struct {
	name         string
	dark         bool
	styles       tview.Theme
	selBg, selFg tcell.Color // list/grid selection; tview.Theme has no slot for it
}

func hx(hex int32) tcell.Color { return tcell.NewHexColor(hex) }

// mk builds a theme from a small, readable set of colors; the rest of
// tview.Theme is derived so callers only pick the ones that matter here.
func mk(name string, dark bool, bg, text, dim, border, title, fieldBg, selBg, selFg int32) theme {
	return theme{
		name: name, dark: dark,
		selBg: hx(selBg), selFg: hx(selFg),
		styles: tview.Theme{
			PrimitiveBackgroundColor:    hx(bg),
			ContrastBackgroundColor:     hx(fieldBg), // form fields, buttons
			MoreContrastBackgroundColor: hx(selBg),
			BorderColor:                 hx(border),
			TitleColor:                  hx(title),
			GraphicsColor:               hx(border),
			PrimaryTextColor:            hx(text),
			SecondaryTextColor:          hx(text), // labels/status: keep readable, not accented
			TertiaryTextColor:           hx(dim),
			InverseTextColor:            hx(selFg),
			ContrastSecondaryTextColor:  hx(text),
		},
	}
}

// themes: darks first (index 0 is the default), then lights. Each is picked for
// contrast - text against background stays comfortably legible.
//
//	name              dark    bg        text      dim       border    title     fieldBg   selBg     selFg
var themes = []theme{
	mk("Dark", true, 0x1c1c1c, 0xe4e4e4, 0x9e9e9e, 0x5f8787, 0x87d7ff, 0x303030, 0x005f87, 0xffffff),
	mk("Nightfall", true, 0x0f1420, 0xd7e0f5, 0x8892b0, 0x2e4a6b, 0x64d8cb, 0x1b2436, 0x2d6cdf, 0xffffff),
	mk("High-contrast dark", true, 0x000000, 0xffffff, 0xbcbcbc, 0xffd700, 0xffd700, 0x1a1a1a, 0xffd700, 0x000000),
	mk("Light", false, 0xf4f4f4, 0x1c1c1c, 0x6c6c6c, 0x8a8a8a, 0x005f87, 0xe4e4e4, 0x005fd7, 0xffffff),
	mk("Paper", false, 0xf7f3e8, 0x2b2b2b, 0x7a7264, 0xb8a988, 0x8a5a00, 0xece5d3, 0x8a5a00, 0xfff8e8),
	mk("High-contrast light", false, 0xffffff, 0x000000, 0x444444, 0x000000, 0x0000cc, 0xededed, 0x0000cc, 0xffffff),
}

// themeIndexByName resolves a saved theme name to its index; an unknown or empty
// name falls back to the default (0).
func themeIndexByName(name string) int {
	for i := range themes {
		if themes[i].name == name {
			return i
		}
	}
	return 0
}

// applyTheme makes theme i the global tview default, so every primitive built
// afterwards (forms, modals) inherits it.
func applyTheme(i int) { tview.Styles = themes[i].styles }

// styleWidgets sets the long-lived primitives' colors explicitly - tview
// captures Styles at construction, and these three outlive a theme switch, so
// they need repainting directly (and selection has no Styles slot).
func (a *App) styleWidgets() {
	thm := themes[a.themeIdx]
	styles := thm.styles
	a.list.SetMainTextColor(styles.PrimaryTextColor).
		SetSelectedBackgroundColor(thm.selBg).
		SetSelectedTextColor(thm.selFg)
	a.list.SetBackgroundColor(styles.PrimitiveBackgroundColor)
	a.list.SetBorderColor(styles.BorderColor).SetTitleColor(styles.TitleColor)
	a.grid.SetBackgroundColor(styles.PrimitiveBackgroundColor)
	a.grid.SetBorderColor(styles.BorderColor).SetTitleColor(styles.TitleColor)
	a.grid.SetSelectedStyle(tcell.StyleDefault.Background(thm.selBg).Foreground(thm.selFg))
	a.status.SetBackgroundColor(styles.PrimitiveBackgroundColor)
	a.status.SetTextColor(styles.SecondaryTextColor)
}

// setTheme applies theme i live: swap the global styles, persist the choice,
// then rebuild the UI so existing primitives are recreated under it, reopening
// whatever was on screen.
func (a *App) setTheme(i int) {
	if i < 0 || i >= len(themes) {
		return
	}
	a.themeIdx = i
	applyTheme(i)
	settings := config.LoadSettings()
	settings.Theme = themes[i].name
	_ = settings.Save() // best-effort; a failed save must not break the running UI

	reopen, tbl := a.curView, a.cur
	a.buildUI()
	a.app.SetRoot(a.pages, true)
	switch {
	case reopen != nil:
		a.openView(reopen)
	case tbl != "":
		a.openTable(tbl)
	default:
		a.app.SetFocus(a.list)
	}
}

// themePicker lists the themes and applies the pick live. Esc restores focus.
func (a *App) themePicker() {
	prev := a.app.GetFocus()
	list := tview.NewList().ShowSecondaryText(false)
	closeForm := func() {
		a.pages.RemovePage("theme")
		if prev != nil {
			a.app.SetFocus(prev)
		}
	}
	for i := range themes {
		mark := "  "
		if i == a.themeIdx {
			mark = "* "
		}
		kind := "light"
		if themes[i].dark {
			kind = "dark"
		}
		list.AddItem(fmt.Sprintf("%s%s  (%s)", mark, themes[i].name, kind), "", 0, func() {
			a.pages.RemovePage("theme")
			a.setTheme(i)
		})
	}
	list.SetDoneFunc(closeForm)
	list.SetBorder(true).SetTitle(" theme (enter=apply, esc=cancel) ")
	a.pages.AddPage("theme", modal(list, 44, len(themes)+4), true, true)
	a.app.SetFocus(list)
}
