// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// isolateConfig points the registry at a throwaway config dir so the picker
// never sees the developer's real databases.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	t.Setenv("XDG_CONFIG_DIRS", t.TempDir())
	t.Setenv("ProgramData", t.TempDir())
}

func writeValidDDL(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "todo.ddl")
	if err := os.WriteFile(p, []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// runPicker drives pickDatabase on a simulation screen and returns a waitFor
// helper plus the result channel.
func runPicker(t *testing.T) (tcell.SimulationScreen, chan *pickResult, func(string)) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	out := make(chan *pickResult, 1)
	go func() {
		r, err := pickDatabase(screen)
		if err != nil {
			t.Errorf("pickDatabase: %v", err)
		}
		out <- r
	}()
	waitFor := func(sub string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(screenText(screen), sub) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%q never appeared on screen:\n%s", sub, screenText(screen))
	}
	return screen, out, waitFor
}

// A registered, openable database appears and selecting it returns its paths.
func TestPickerSelectsRegistered(t *testing.T) {
	isolateConfig(t)
	ddlPath := writeValidDDL(t)
	cfg, err := config.Create("todo", ddlPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	screen, out, waitFor := runPicker(t)
	waitFor("todo")                                    // registered db drew
	waitFor("Create new database")                     // menu entries drew
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // first item = todo

	select {
	case r := <-out:
		if r == nil {
			t.Fatal("expected a pick result")
		}
		if r.ddlPath != cfg.DDLPath || r.logDir != cfg.LogDir {
			t.Fatalf("wrong paths: %+v", r)
		}
		if r.cfg == nil || r.cfg.Name != "todo" {
			t.Fatalf("registry record not returned: %+v", r.cfg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("picker did not return after selecting a database")
	}
}

// Highlighting a blank spacer row hops to a real entry, in either direction, so
// arrowing through the menu never rests on an empty line.
func TestPickerSkipsSpacers(t *testing.T) {
	isolateConfig(t)
	ddlPath := writeValidDDL(t)
	if _, err := config.Create("todo", ddlPath, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	p := &picker{
		app:    tview.NewApplication(),
		pages:  tview.NewPages(),
		list:   tview.NewList(),
		status: tview.NewTextView(),
		listed: config.List(),
	}
	p.build()
	// rows: 0=todo, 1=spacer, 2=Create, 3=spacer, 4=Open
	p.list.SetCurrentItem(0)
	p.moveSelection(1) // down over spacer 1 -> Create at 2
	if got := p.list.GetCurrentItem(); got != 2 {
		t.Fatalf("down over spacer landed on %d, want 2", got)
	}
	p.moveSelection(-1) // up over spacer 1 -> todo at 0
	if got := p.list.GetCurrentItem(); got != 0 {
		t.Fatalf("up over spacer landed on %d, want 0", got)
	}
	p.moveSelection(-1) // already at the top -> stays put
	if got := p.list.GetCurrentItem(); got != 0 {
		t.Fatalf("up at top moved to %d, want 0", got)
	}
}

// Pressing d on a database, confirming Remove, then keeping its files,
// deregisters it: it leaves the list and the registry, but its files stay.
func TestPickerRemovesDatabase(t *testing.T) {
	isolateConfig(t)
	ddlPath := writeValidDDL(t)
	logDir := t.TempDir()
	if _, err := config.Create("todo", ddlPath, logDir); err != nil {
		t.Fatal(err)
	}

	screen, out, waitFor := runPicker(t)
	waitFor("todo")
	screen.InjectKey(tcell.KeyRune, 'd', tcell.ModNone) // remove the highlighted db
	waitFor("Cancel")                                   // first confirm (Remove/Cancel)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)  // default button = Remove
	waitFor("Keep files")                               // second confirm (logDir exists)
	screen.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)  // default button = Keep files
	waitFor("files kept")                               // status after the refresh

	if got := config.List(); len(got) != 0 {
		t.Fatalf("registry still has %d records after remove", len(got))
	}
	if _, err := os.Stat(logDir); err != nil {
		t.Fatalf("tx-log wrongly deleted (files were kept): %v", err)
	}

	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	<-out
}

// An unopenable record (its DDL is gone) shows with the [!] marker, cannot be
// opened, and q quits the picker with no result.
func TestPickerFlagsBrokenAndQuits(t *testing.T) {
	isolateConfig(t)
	// register a db then delete its DDL
	ddlPath := writeValidDDL(t)
	if _, err := config.Create("broken", ddlPath, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ddlPath); err != nil {
		t.Fatal(err)
	}

	screen, out, waitFor := runPicker(t)
	waitFor("[!] broken")    // flagged unopenable
	waitFor("DDL not found") // with the reason
	screen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)

	select {
	case r := <-out:
		if r != nil {
			t.Fatalf("quit should yield no result, got %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("q did not quit the picker")
	}
}
