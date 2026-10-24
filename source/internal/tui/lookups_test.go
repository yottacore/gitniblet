// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

const lookupDDL = `
tables:
	table: tasks
		fields:
			field: name
				type: string
			field: status
				type: int
				ui:
					list_type: lookup
					list_source: 1
			field: size
				type: int
				ui:
					list_type: lookup
					list_source: 2
					lookup_in_lists: title
`

// runLookups opens the tasks table on a simulation screen with the given
// charset and returns the screen once the first row shows.
func runLookups(t *testing.T, charset string) (*simScreen, *App, func(string), chan error) {
	t.Helper()
	api, sch, bs := ddlSetup(t, lookupDDL)
	for _, row := range []map[string]string{{"idx": "1", "title": "Status"}, {"idx": "2", "title": "Size"}} {
		if _, err := api.Create("lookups", row); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []map[string]string{
		{"idx": "1", "lookups_idx": "1", "title": "Open", "symbol": "🔘"},
		{"idx": "2", "lookups_idx": "2", "title": "Big", "symbol": "B"},
	} {
		if _, err := api.Create("lookup_values", row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := api.Create("tasks", map[string]string{"name": "t1", "status": "1", "size": "2"}); err != nil {
		t.Fatal(err)
	}
	a, err := NewApp(api, sch, bs)
	if err != nil {
		t.Fatal(err)
	}
	screen := newSimScreen(t, charset, 120, 32)
	done := make(chan error, 1)
	go func() { done <- a.RunWith(screen) }()
	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		a.Stop()
		t.Fatalf("%q never appeared on screen:\n%s", sub, screenText(screen))
	}
	waitFor("tasks")
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // first table
	waitFor("t1")
	return screen, a, waitFor, done
}

func quitLookups(t *testing.T, screen *simScreen, a *App, done chan error) {
	t.Helper()
	screen.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.Stop()
		t.Fatal("q did not quit the app")
	}
}

// lineWith is the screen line holding sub, as runes, one per cell.
func lineWith(screen *simScreen, sub string) []rune {
	for _, line := range strings.Split(screenText(screen), "\n") {
		if strings.Contains(line, sub) {
			return []rune(line)
		}
	}
	return nil
}

func runeIndex(line []rune, sub string) int {
	i := strings.Index(string(line), sub)
	if i < 0 {
		return -1
	}
	return utf8.RuneCountInString(string(line)[:i])
}

// A lookup field lists as its symbol, the double-width symbol keeps the next
// column in line, and the edit form says what the stored idx stands for.
func TestLookupsShowInTUI(t *testing.T) {
	screen, a, waitFor, done := runLookups(t, "UTF-8")
	header, row := lineWith(screen, "status"), lineWith(screen, "t1")
	if !strings.Contains(string(row), "🔘") || !strings.Contains(string(row), "Big") {
		t.Fatalf("want the symbol and the title in the row:\n%s", screenText(screen))
	}
	if h, r := runeIndex(header, "size"), runeIndex(row, "Big"); h < 0 || h != r {
		t.Fatalf("size column off by a wide symbol: header at %d, value at %d:\n%s", h, r, screenText(screen))
	}

	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // edit t1
	waitFor("status (🔘")
	// Saving after a label change must still read every field by position.
	screen.InjectKey(tcell.KeyRune, 'x', tcell.ModNone)
	for range 3 {
		screen.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	}
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // Save
	// The input shows t1x before the save runs, so wait for the form to go.
	for deadline := time.Now().Add(3 * time.Second); strings.Contains(screenText(screen), "status (🔘"); {
		if time.Now().After(deadline) {
			a.Stop()
			t.Fatalf("edit form never closed:\n%s", screenText(screen))
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitFor("t1x")
	rows, err := a.api.Query(`SELECT name, status, size FROM tasks`)
	if err != nil || len(rows) != 1 || rows[0]["name"] != "t1x" || rows[0]["status"] != "1" || rows[0]["size"] != "2" {
		t.Fatalf("save after the edit form: %v %v", rows, err)
	}
	quitLookups(t, screen, a, done)
}

// A terminal whose charset can't draw the symbol gets the title instead.
func TestLookupsFallBackToTitleInTUI(t *testing.T) {
	screen, a, _, done := runLookups(t, "US-ASCII")
	row := string(lineWith(screen, "t1"))
	if !strings.Contains(row, "Open") || strings.Contains(row, "🔘") {
		t.Fatalf("want the title in place of an undrawable symbol:\n%s", screenText(screen))
	}
	quitLookups(t, screen, a, done)
}
