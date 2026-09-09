// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/donate"
	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// pickResult is the picker's outcome: the paths to open, plus the registry
// record to stamp as opened (nil for an ad-hoc "open existing").
type pickResult struct {
	ddlPath, sqlitePath, logDir string
	cfg                         *config.DBConfig
}

// picker is the startup "which database?" screen shown when no DDL was named:
// the registered databases (unopenable ones flagged with why) plus entries to
// create or open one. It is its own tview app, run before the main App over the
// chosen database.
type picker struct {
	app     *tview.Application
	pages   *tview.Pages
	list    *tview.List
	status  *tview.TextView
	listed  []config.Listed
	actions []func()         // per list row; nil for a blank spacer row
	rowDB   []*config.Listed // per list row; the record behind a removable db row, else nil
	result  *pickResult
}

// pickDatabase runs the picker and returns the chosen database, or nil if the
// user quit without choosing. screen is nil for the real terminal (tests pass a
// simulation screen).
func pickDatabase(screen tcell.Screen) (*pickResult, error) {
	p := &picker{
		app:    tview.NewApplication(),
		pages:  tview.NewPages(),
		list:   tview.NewList(),
		status: tview.NewTextView(),
		listed: config.List(),
	}
	p.build()
	if screen != nil {
		p.app.SetScreen(screen)
	}
	if err := p.app.SetRoot(p.pages, true).EnableMouse(true).Run(); err != nil {
		return nil, err
	}
	return p.result, nil
}

func (p *picker) build() {
	// Each entry is two lines (name + path/hint), the border padding keeps the
	// block off the frame, and blank spacer rows sit between entries, so it
	// reads as a menu instead of a packed grid.
	p.list.ShowSecondaryText(true).
		SetSelectedTextColor(tcell.ColorBlack).
		SetBorder(true).SetTitle(" nano-git-db ").
		SetBorderPadding(1, 1, 3, 3)

	p.populate()
	p.list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i >= 0 && i < len(p.actions) && p.actions[i] != nil {
			p.actions[i]()
		}
	})
	// Up/Down step over the blank spacer rows so the highlight only ever rests
	// on a real entry (first and last rows are always real); d removes the
	// highlighted database.
	p.list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyDown || ev.Rune() == 'j':
			p.moveSelection(1)
			return nil
		case ev.Key() == tcell.KeyUp || ev.Rune() == 'k':
			p.moveSelection(-1)
			return nil
		case ev.Rune() == 'd' || ev.Key() == tcell.KeyDelete:
			p.removeCurrent()
			return nil
		}
		return ev
	})

	p.status.SetDynamicColors(true)
	p.setStatus("enter=open | d=remove | Create/Open to make or open one | q=quit")

	// Center the menu as a bordered panel with margins around it, instead of
	// filling the whole screen. Proportional spacers keep it centered at any
	// terminal size while leaving generous room for long paths.
	panel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true).
		AddItem(p.status, 1, 0, false)
	root := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(panel, 0, 5, true).
			AddItem(nil, 0, 1, false), 0, 4, true).
		AddItem(nil, 0, 1, false)
	p.pages.AddPage("main", root, true, true)

	p.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// q quits only outside input fields (where q is data).
		if ev.Rune() == 'q' {
			if _, ok := p.app.GetFocus().(*tview.InputField); !ok {
				p.app.Stop()
				return nil
			}
		}
		return ev
	})
}

func (p *picker) setStatus(msg string) { p.status.SetText(" " + msg) }

// moveSelection advances the highlight by dir (+1/-1), skipping spacer rows and
// stopping at the ends rather than wrapping.
func (p *picker) moveSelection(dir int) {
	count := p.list.GetItemCount()
	i := p.list.GetCurrentItem() + dir
	for i >= 0 && i < count && p.actions[i] == nil {
		i += dir
	}
	if i >= 0 && i < count {
		p.list.SetCurrentItem(i)
	}
}

// populate fills the list from p.listed: each registered database (unopenable
// ones flagged), then Create/Open, with a blank spacer row between entries.
// actions[i] and rowDB[i] track list rows 1:1 - a nil action marks a spacer;
// rowDB holds the record behind a removable database row (nil for spacers, the
// Create/Open rows, and read-only system records).
func (p *picker) populate() {
	p.list.Clear()
	p.actions = p.actions[:0]
	p.rowDB = p.rowDB[:0]
	type entry struct {
		main, sub string
		act       func()
		db        *config.Listed
	}
	var entries []entry
	for i := range p.listed {
		listed := &p.listed[i]
		main := listed.Name
		sub := listed.Dir
		if listed.System {
			main += "  (system)"
		}
		if listed.Err != nil {
			main = "[!] " + main
			sub = listed.Err.Error()
		}
		chosen := *listed
		// A record is removable only when it loaded and is not a read-only
		// system record; a broken ([!]) user record still is - that's the point.
		var rec *config.Listed
		if listed.Config != nil && !listed.System {
			rec = listed
		}
		entries = append(entries, entry{main, sub, func() { p.chooseListed(chosen) }, rec})
	}
	entries = append(entries,
		entry{"Create new database", "register a new database and open it", p.createForm, nil},
		entry{"Open existing ...", "open a DDL + tx-log without registering", p.openForm, nil})
	if donate.Enabled { // open-source-only feature
		entries = append(entries, entry{"Support nano-git-db", "ways to support the project", p.donateModal, nil})
	}
	for i, ent := range entries {
		if i > 0 {
			p.list.AddItem("", "", 0, nil)
			p.actions = append(p.actions, nil)
			p.rowDB = append(p.rowDB, nil)
		}
		p.list.AddItem(ent.main, ent.sub, 0, nil)
		p.actions = append(p.actions, ent.act)
		p.rowDB = append(p.rowDB, ent.db)
	}
}

// removeCurrent starts the remove flow for the highlighted row, if it is a
// removable database.
func (p *picker) removeCurrent() {
	i := p.list.GetCurrentItem()
	if i >= 0 && i < len(p.rowDB) && p.rowDB[i] != nil {
		p.confirmRemove(p.rowDB[i].Config)
	}
}

// confirmRemove asks before deregistering a database, then - only if it has
// files on disk - offers a second confirm to delete those too (default keep).
// Deregister leaves the tx-log/sqlite/key/DDL in place.
func (p *picker) confirmRemove(cfg *config.DBConfig) {
	msg := fmt.Sprintf("Remove %q from the list?\n(its files stay on disk)", cfg.Name)
	p.modal("remove", msg, []string{"Remove", "Cancel"}, func(label string) {
		if label != "Remove" {
			return
		}
		if err := cfg.Deregister(); err != nil {
			p.setStatus("remove failed: " + err.Error())
			return
		}
		if cfg.HasFiles() {
			p.confirmDeleteFiles(cfg)
			return
		}
		p.refresh("removed " + cfg.Name)
	})
}

// confirmDeleteFiles offers to also delete the deregistered database's files.
// The default (first button) keeps them; the DDL is never deleted.
func (p *picker) confirmDeleteFiles(cfg *config.DBConfig) {
	msg := fmt.Sprintf("Also delete its files? (kept by default)\n  tx-log: %s\n  record: %s\n(the schema file is kept)", cfg.LogDir, cfg.Dir())
	p.modal("delfiles", msg, []string{"Keep files", "Delete files"}, func(label string) {
		if label == "Delete files" {
			if err := cfg.DeleteFiles(); err != nil {
				p.setStatus("delete failed: " + err.Error())
				p.refresh("removed " + cfg.Name)
				return
			}
			p.refresh("deleted " + cfg.Name)
			return
		}
		p.refresh("removed " + cfg.Name + " (files kept)")
	})
}

// modal shows a confirm dialog; the first button is the default. It closes
// itself and refocuses the list before calling fn, so fn may open a follow-up.
func (p *picker) modal(name, msg string, buttons []string, fn func(label string)) {
	dialog := tview.NewModal().SetText(msg).AddButtons(buttons).
		SetDoneFunc(func(_ int, label string) {
			p.pages.RemovePage(name)
			p.app.SetFocus(p.list)
			fn(label)
		})
	p.pages.AddPage(name, dialog, true, true)
	p.app.SetFocus(dialog)
}

// refresh re-reads the registry and rebuilds the list after a change.
func (p *picker) refresh(status string) {
	p.listed = config.List()
	p.populate()
	p.setStatus(status)
}

// chooseListed opens a registered database, or reports why it can't open.
func (p *picker) chooseListed(listed config.Listed) {
	if listed.Err != nil {
		p.setStatus("cannot open " + listed.Name + ": " + listed.Err.Error())
		return
	}
	p.finish(&pickResult{
		ddlPath:    listed.Config.DDLPath,
		sqlitePath: listed.Config.SQLitePath,
		logDir:     listed.Config.LogDir,
		cfg:        listed.Config,
	})
}

// finish records the result and ends the picker so the caller can open it.
func (p *picker) finish(result *pickResult) {
	p.result = result
	p.app.Stop()
}

// createForm registers a new database (name required, refuses duplicates) and
// opens it. The DDL is an existing file the record points at; "location" is just
// a repo or folder - the tx-log goes under `ngdb/<name>` inside a repo, or in
// the folder as-is otherwise - so there is no separate tx-log path to enter.
func (p *picker) createForm() {
	form := tview.NewForm()
	form.AddInputField("name", "", 0, nil, nil)
	form.AddInputField("ddl path", "", 0, nil, nil)
	form.AddInputField("location (repo or folder)", "", 0, nil, nil)
	closeForm := func() {
		p.pages.RemovePage("create")
		p.app.SetFocus(p.list)
	}
	form.AddButton("Create", func() {
		name := strings.TrimSpace(text(form, "name"))
		ddlPath := strings.TrimSpace(text(form, "ddl path"))
		location := strings.TrimSpace(text(form, "location (repo or folder)"))
		if name == "" || ddlPath == "" || location == "" {
			p.setStatus("name, ddl path and location are all required")
			return
		}
		logDir, err := config.LogDirFor(location, name)
		if err != nil {
			p.setStatus("error: " + err.Error())
			return
		}
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			p.setStatus("error: " + err.Error())
			return
		}
		cfg, err := config.Create(name, ddlPath, logDir)
		if err != nil {
			p.setStatus("error: " + err.Error())
			return
		}
		closeForm()
		p.finish(&pickResult{ddlPath: cfg.DDLPath, sqlitePath: cfg.SQLitePath, logDir: cfg.LogDir, cfg: cfg})
	})
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(" create new database ")
	p.pages.AddPage("create", modal(form, 64, 12), true, true)
	p.app.SetFocus(form)
}

// openForm opens an existing DDL + tx-log without registering it. The sqlite
// view defaults beside the DDL; it rebuilds from the log on open.
func (p *picker) openForm() {
	form := tview.NewForm()
	form.AddInputField("ddl path", "", 0, nil, nil)
	form.AddInputField("tx-log dir", "", 0, nil, nil)
	closeForm := func() {
		p.pages.RemovePage("open")
		p.app.SetFocus(p.list)
	}
	form.AddButton("Open", func() {
		ddlPath := strings.TrimSpace(text(form, "ddl path"))
		logDir := strings.TrimSpace(text(form, "tx-log dir"))
		if ddlPath == "" || logDir == "" {
			p.setStatus("ddl path and tx-log dir are required")
			return
		}
		closeForm()
		p.finish(&pickResult{ddlPath: ddlPath, sqlitePath: config.LocalSQLite(ddlPath), logDir: logDir})
	})
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(" open existing database ")
	p.pages.AddPage("open", modal(form, 64, 10), true, true)
	p.app.SetFocus(form)
}

// text reads a form input field by label.
func text(form *tview.Form, label string) string {
	return form.GetFormItemByLabel(label).(*tview.InputField).GetText()
}
