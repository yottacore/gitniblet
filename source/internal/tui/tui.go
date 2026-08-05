// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package tui is the terminal UI front-end: tview/tcell, both pure Go. Like
// every front-end it only talks to the shared CRUD API. Left pane lists the
// tables; per the design no records show until asked (Enter opens a table).
package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/script"
)

// Run is the TUI entry point. Given a registered database name it opens that
// database directly; with an explicit <ddl> <sqlite> <logdir> triple it opens
// those paths; with nothing it uses a DDL sitting in the current directory,
// else shows the registry picker (choose / create / open).
func Run(args []string) error {
	applyTheme(themeIndexByName(config.LoadSettings().Theme)) // so the picker is themed too
	dec := runGate(nil)
	if !dec.proceed { // quit at the startup notice
		return nil
	}
	if len(args) == 1 {
		cfg := config.FindByName(args[0])
		if cfg == nil {
			return config.UnknownDatabase(args[0])
		}
		return runOpen(cfg.DDLPath, cfg.SQLitePath, cfg.LogDir, cfg, dec)
	}
	if len(args) == 3 {
		return runOpen(args[0], args[1], args[2], nil, dec)
	}
	if len(args) != 0 { // 2 or 4+ args matches neither form
		return fmt.Errorf("usage: ngdb --tui [<db>], or the explicit --tui <schema> <sqlite> <logdir>")
	}
	if ddlPath, sqlitePath, logDir, ok := config.PWDTriple(); ok {
		return runOpen(ddlPath, sqlitePath, logDir, nil, dec)
	}
	res, err := pickDatabase(nil)
	if err != nil || res == nil { // nil result = user quit the picker
		return err
	}
	return runOpen(res.ddlPath, res.sqlitePath, res.logDir, res.cfg, dec)
}

// runOpen brings up one database and runs the main UI over it. cfg, when set
// (a registry pick), is stamped with the open time. dec carries the startup
// notice outcome: an optional banner and whether to open read-only.
func runOpen(ddlPath, sqlitePath, logDir string, cfg *config.DBConfig, dec gateDecision) error {
	keyFile, pref := config.ResolveEncryption(ddlPath, cfg)
	client, err := schema.OpenClientWith(schema.OpenOpts{
		DDLPath: ddlPath, DBPath: sqlitePath, LogDir: logDir, KeyFile: keyFile, EncryptPref: pref,
	})
	if err != nil {
		return err
	}
	defer client.Close()
	if cfg != nil {
		if override := config.EncryptPref(); override != "" {
			cfg.Encryption = override // persist a --encrypt toggle for this host
		}
		_ = cfg.Touch() // best-effort last-opened stamp; open already succeeded
	}
	builtins, err := schema.Builtins()
	if err != nil {
		return err
	}
	triggerWarns, err := script.Attach(client.API, ddlPath, logDir, client.Schema, builtins)
	if err != nil {
		return err
	}
	app, err := NewApp(client.API, client.Schema, builtins)
	if err != nil {
		return err
	}
	app.banner = dec.banner
	app.api.SetReadOnly(dec.readOnly)
	app.cat.Queries = client.Queries
	// stderr: the alternate screen is about to cover stdout, and these should
	// survive in the scrollback / a redirect after quit.
	for _, w := range append(append(client.Warnings, triggerWarns...), app.cat.Warnings...) {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	stop := client.StartAutoSync(client.Schema.TunableInt("git_sync_frequency", 60),
		func(w string) { fmt.Fprintln(os.Stderr, "warning:", w) })
	defer stop()
	return app.Run()
}

// App is the TUI over one open database.
type App struct {
	api     *crud.API
	cat     *schema.Catalog
	app     *tview.Application
	pages   *tview.Pages
	list    *tview.List
	grid    *tview.Table
	status  *tview.TextView
	vstatus *tview.TextView // open view page's own status line, else nil

	cur      string
	curView  *schema.ViewSpec    // open view (mutually exclusive with cur), else nil
	rows     []map[string]string // rows behind the grid, index = grid row-1
	themeIdx int
	banner   string // startup-notice banner shown atop the UI, empty = none
}

// NewApp builds the UI over an API. Schemas supply table order and editable
// fields: pass the user schema first, then the built-ins.
func NewApp(api *crud.API, schemas ...*ddl.Schema) (*App, error) {
	cat, err := schema.NewCatalog(api, schemas...)
	if err != nil {
		return nil, err
	}
	a := &App{
		api:      api,
		cat:      cat,
		app:      tview.NewApplication(),
		themeIdx: themeIndexByName(config.LoadSettings().Theme),
	}
	applyTheme(a.themeIdx) // set global styles before buildUI creates primitives
	a.buildUI()
	return a, nil
}

// Run starts the event loop on the real terminal.
func (a *App) Run() error { return a.RunWith(nil) }

// RunWith starts the event loop on a given screen (tests pass a simulation
// screen; nil means the real terminal).
func (a *App) RunWith(screen tcell.Screen) error {
	if screen != nil {
		a.app.SetScreen(screen)
	}
	// The default view opens on start (here, not buildUI, so the caller can
	// hand the catalog its named queries first); Esc drops back to the list.
	if v := a.cat.View(a.cat.DefaultView); v != nil {
		a.openView(v)
	}
	return a.app.SetRoot(a.pages, true).EnableMouse(true).Run()
}

// Stop ends the event loop (tests).
func (a *App) Stop() { a.app.Stop() }

func (a *App) buildUI() {
	// Fresh primitives each call so a theme switch rebuilds them under the new
	// global styles (tview captures colors at construction).
	a.pages = tview.NewPages()
	a.list = tview.NewList()
	a.grid = tview.NewTable()
	a.status = tview.NewTextView()
	a.vstatus = nil

	a.list.ShowSecondaryText(false).SetBorder(true).SetTitle(" views + tables ")
	for i := range a.cat.Views {
		a.list.AddItem(a.cat.Views[i].Name+" (view)", "", 0, nil)
	}
	for _, table := range a.cat.Tables {
		a.list.AddItem(table, "", 0, nil)
	}
	a.list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i < len(a.cat.Views) {
			a.openView(&a.cat.Views[i])
			return
		}
		a.openTable(a.cat.Tables[i-len(a.cat.Views)])
	})

	a.grid.SetSelectable(true, false).SetFixed(1, 0)
	a.grid.SetBorder(true).SetTitle(" rows ")
	a.grid.SetSelectedFunc(func(row, _ int) {
		if row >= 1 && row-1 < len(a.rows) {
			a.editForm(a.cur, a.rows[row-1])
		}
	})
	a.grid.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEscape:
			a.app.SetFocus(a.list)
			return nil
		case ev.Rune() == 'n':
			a.editForm(a.cur, nil)
			return nil
		case ev.Rune() == 'r':
			a.openTable(a.cur)
			return nil
		case ev.Rune() == 'd':
			a.deleteSelected(false)
			return nil
		case ev.Rune() == 'x':
			a.deleteSelected(true)
			return nil
		case ev.Rune() == 'c':
			a.extrasPanel()
			return nil
		case ev.Rune() == 'u':
			a.attachForm(false)
			return nil
		case ev.Rune() == 'f':
			a.attachForm(true)
			return nil
		}
		return ev
	})

	a.status.SetDynamicColors(true)
	a.setStatus("enter=open table | in rows: enter=edit n=new d=del x=hard-del c=comments/attachments u/f=attach uri/file r=reload esc=back | T=theme q=quit")

	main := tview.NewFlex().
		AddItem(a.list, 24, 0, true).
		AddItem(a.grid, 0, 1, false)
	root := tview.NewFlex().SetDirection(tview.FlexRow)
	if a.banner != "" {
		bannerView := tview.NewTextView().SetDynamicColors(true).
			SetText("[black:yellow] " + a.banner + " [-:-]")
		root.AddItem(bannerView, 1, 0, false)
	}
	root.AddItem(main, 0, 1, true).
		AddItem(a.status, 1, 0, false)
	a.pages.AddPage("main", root, true, true)

	a.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// q/T are browsing keys. Anywhere else - a form, a confirm dialog - the
		// keystroke belongs to that screen, and quitting from a half-filled form
		// would throw the work away without asking.
		if !a.browsing() {
			return ev
		}
		switch ev.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'T':
			a.themePicker()
			return nil
		}
		return ev
	})

	a.styleWidgets()
}

// browsing reports whether the top page is a list or view rather than a form,
// dialog or picker stacked over one.
func (a *App) browsing() bool {
	name, _ := a.pages.GetFrontPage()
	return name == "main" || name == "view"
}

func (a *App) setStatus(msg string) {
	a.status.SetText(" " + msg)
	if a.vstatus != nil {
		a.vstatus.SetText(" " + msg)
	}
}

func (a *App) openTable(table string) {
	if table == "" {
		return
	}
	rows, err := a.cat.LiveRows(a.api, table)
	if err != nil {
		a.setStatus("error: " + err.Error())
		return
	}
	a.cur = table
	a.curView = nil
	a.rows = rows
	cols := a.cat.ColumnsFor(table)

	a.grid.Clear()
	a.grid.SetTitle(fmt.Sprintf(" %s (%d) ", table, len(rows)))
	for c, name := range cols {
		a.grid.SetCell(0, c, tview.NewTableCell(name).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
	for r, row := range rows {
		for c, name := range cols {
			val := row[name]
			if name == "id" && len(val) > 8 {
				val = val[:8]
			}
			a.grid.SetCell(r+1, c, tview.NewTableCell(val))
		}
	}
	if len(rows) > 0 {
		a.grid.Select(1, 0)
	}
	a.app.SetFocus(a.grid)
}

// editForm opens the create (row == nil) or edit form for one row. Saving
// writes only the changed fields, so the tx-log stays field-granular.
func (a *App) editForm(table string, row map[string]string) {
	a.editFormThen(table, row, a.grid, func() { a.openTable(table) })
}

// editFormThen is the reusable core: back gets focus on cancel, reload runs
// after a save - the table grid and view blocks refresh differently.
func (a *App) editFormThen(table string, row map[string]string, back tview.Primitive, reload func()) {
	if table == "" {
		return
	}
	form := tview.NewForm()
	fields := a.cat.Fields[table]
	for _, field := range fields {
		form.AddInputField(field, row[field], 0, nil, nil)
	}
	title := " new " + table + " "
	if row != nil {
		title = " edit " + table + " " + row["id"][:min(8, len(row["id"]))] + " "
	}
	form.AddButton("Save", func() {
		vals := map[string]string{}
		for _, field := range fields {
			val := form.GetFormItemByLabel(field).(*tview.InputField).GetText()
			if row == nil || row[field] != val {
				vals[field] = val
			}
		}
		var err error
		if row == nil {
			_, err = a.api.Create(table, vals)
		} else if len(vals) > 0 {
			err = a.api.Update(table, row["id"], vals)
		}
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		a.pages.RemovePage("form")
		reload()
		a.app.SetFocus(back)
	})
	form.AddButton("Cancel", func() {
		a.pages.RemovePage("form")
		a.app.SetFocus(back)
	})
	form.SetCancelFunc(func() {
		a.pages.RemovePage("form")
		a.app.SetFocus(back)
	})
	form.SetBorder(true).SetTitle(title)
	a.pages.AddPage("form", modal(form, 60, 4+2*len(fields)+2), true, true)
	a.app.SetFocus(form)
}

// selectedRow is the row behind the grid cursor, or nil.
func (a *App) selectedRow() map[string]string {
	r, _ := a.grid.GetSelection()
	if r < 1 || r-1 >= len(a.rows) {
		return nil
	}
	return a.rows[r-1]
}

// extrasPanel shows the selected row's opt-in extras: comments (with add)
// and attachments of both kinds.
func (a *App) extrasPanel() {
	row := a.selectedRow()
	if row == nil {
		return
	}
	feats := a.cat.Features[a.cur]
	if !feats.Comments && !feats.URIAttachments && !feats.LocalAttachments {
		a.setStatus(a.cur + " has no comments/attachments enabled in its DDL")
		return
	}
	form := tview.NewForm()
	rowsShown := 0
	if feats.Comments {
		var sb strings.Builder
		comments, err := a.api.CommentsFor(a.cur, row["id"])
		if err != nil {
			sb.WriteString("error: " + err.Error())
		}
		for _, comment := range comments {
			fmt.Fprintf(&sb, "%s  %s\n", comment["date_created"], comment["comment"])
		}
		if sb.Len() == 0 {
			sb.WriteString("(none)")
		}
		form.AddTextView("comments", sb.String(), 0, 5, false, true)
		form.AddInputField("new comment", "", 0, nil, nil)
		rowsShown += 7
	}
	if feats.URIAttachments || feats.LocalAttachments {
		var sb strings.Builder
		attachments, err := a.api.AttachmentsFor(a.cur, row["id"])
		if err != nil {
			sb.WriteString("error: " + err.Error())
		}
		for _, attachment := range attachments {
			fmt.Fprintf(&sb, "[%s] %s  %s\n", attachment.Kind, attachment.Label, attachment.Description)
		}
		if sb.Len() == 0 {
			sb.WriteString("(none)")
		}
		form.AddTextView("attachments", sb.String(), 0, 4, false, true)
		rowsShown += 5
	}
	closeForm := func() {
		a.pages.RemovePage("extras")
		a.app.SetFocus(a.grid)
	}
	if feats.Comments {
		form.AddButton("Add comment", func() {
			text := form.GetFormItemByLabel("new comment").(*tview.InputField).GetText()
			if text == "" {
				return
			}
			if _, err := a.api.CommentAdd(a.cur, row["id"], text); err != nil {
				a.setStatus("error: " + err.Error())
				return
			}
			closeForm()
			a.extrasPanel() // reopen refreshed
		})
	}
	form.AddButton("Close", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(" " + a.cur + " " + row["id"][:min(8, len(row["id"]))] + " extras ")
	a.pages.AddPage("extras", modal(form, 70, rowsShown+4), true, true)
	a.app.SetFocus(form)
}

// attachForm adds a URI (copyIn=false) or copied-file attachment to the
// selected row.
func (a *App) attachForm(copyIn bool) {
	row := a.selectedRow()
	if row == nil {
		return
	}
	feats := a.cat.Features[a.cur]
	label, title := "uri", " attach uri to "+a.cur+" "
	if copyIn {
		label, title = "path", " attach file to "+a.cur+" "
	}
	if (copyIn && !feats.LocalAttachments) || (!copyIn && !feats.URIAttachments) {
		a.setStatus(a.cur + " has not opted in to " + label + " attachments")
		return
	}
	form := tview.NewForm()
	form.AddInputField(label, "", 0, nil, nil)
	form.AddInputField("description", "", 0, nil, nil)
	closeForm := func() {
		a.pages.RemovePage("attach")
		a.app.SetFocus(a.grid)
	}
	form.AddButton("Attach", func() {
		target := form.GetFormItemByLabel(label).(*tview.InputField).GetText()
		desc := form.GetFormItemByLabel("description").(*tview.InputField).GetText()
		if target == "" {
			return
		}
		var err error
		if copyIn {
			_, err = a.api.AttachFile(a.cur, row["id"], target, desc)
		} else {
			_, err = a.api.AttachURI(a.cur, row["id"], target, desc)
		}
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		closeForm()
		a.setStatus("attached")
	})
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)
	form.SetBorder(true).SetTitle(title)
	a.pages.AddPage("attach", modal(form, 70, 10), true, true)
	a.app.SetFocus(form)
}

func (a *App) deleteSelected(hard bool) {
	row := a.selectedRow()
	if row == nil {
		return
	}
	verb := "Soft-delete"
	if hard {
		verb = "PERMANENTLY delete"
	}
	a.confirm(fmt.Sprintf("%s %s %s?", verb, a.cur, row["id"][:min(8, len(row["id"]))]), func() {
		var err error
		if hard {
			err = a.api.Delete(a.cur, row["id"])
		} else {
			err = a.api.MarkDelete(a.cur, row["id"])
		}
		if err != nil {
			a.setStatus("error: " + err.Error())
			return
		}
		a.openTable(a.cur)
	})
}

func (a *App) confirm(msg string, fn func()) {
	dialog := tview.NewModal().SetText(msg).AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			a.pages.RemovePage("confirm")
			a.app.SetFocus(a.grid)
			if label == "Yes" {
				fn()
			}
		})
	a.pages.AddPage("confirm", dialog, true, true)
}

// modal centers p at a fixed size over the main page.
func modal(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}
