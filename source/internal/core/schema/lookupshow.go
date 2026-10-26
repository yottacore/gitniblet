// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"slices"
	"strconv"
	"strings"

	"github.com/yottacore/gitniblet/internal/core/crud"
)

// Display modes for lookup_in_lists and lookup_in_forms.
const (
	ShowSymbol = "symbol"
	ShowTitle  = "title"
	ShowBoth   = "both"
)

// lookupSpec is one `list_type: lookup` field: the group it draws from and
// how lists and forms show it.
type lookupSpec struct {
	group      string // lookups.idx, as the view returns it
	list, form string
}

type lookupLabel struct{ symbol, title string }

// LookupShow turns the idx a lookup-backed field stores into its value's
// symbol, title or both (design.md Lookups). Load one per table render with
// Catalog.Lookups. A nil one shows every value as stored.
type LookupShow struct {
	specs  map[string]lookupSpec
	labels map[string]map[string]lookupLabel // group idx -> value idx -> label
	picks  map[string][]string               // group idx -> active value idxs, in sort order

	options map[string][]LookupChoice // literal and sql fields' picks, once FormChoices loads them

	// SymbolOK, when set, says whether a symbol can be drawn at all. The TUI
	// sets it for glyphs the terminal can't show, which then fall back to the
	// title. Nil takes every symbol.
	SymbolOK func(symbol string) bool
}

// resolveLookups finds each table's lookup-backed fields once, warning on any
// whose list_source isn't a group idx.
func (c *Catalog) resolveLookups() {
	c.lookups = make(map[string]map[string]lookupSpec, len(c.Tables))
	c.pickers = map[string]map[string]pickSpec{}
	for _, table := range c.Tables {
		if specs := c.findLookups(table); specs != nil {
			c.lookups[table] = specs
		}
		if specs := c.findPickers(table); specs != nil {
			c.pickers[table] = specs
		}
	}
}

func (c *Catalog) findLookups(table string) map[string]lookupSpec {
	var specs map[string]lookupSpec
	for _, field := range c.Fields[table] {
		ui := c.fieldUI[table][field]
		if !strings.EqualFold(ui.ListType, "lookup") {
			continue
		}
		group := strings.TrimSpace(ui.ListSource)
		if n, err := strconv.Atoi(group); err != nil || n < 1 {
			c.warnf("%s.%s: list_type lookup wants a lookups idx as list_source, got %q", table, field, ui.ListSource)
			continue
		}
		if specs == nil {
			specs = map[string]lookupSpec{}
		}
		specs[field] = lookupSpec{group: group, list: modeOr(ui.LookupInList, ShowSymbol), form: modeOr(ui.LookupInForm, ShowBoth)}
	}
	return specs
}

func modeOr(mode, dflt string) string {
	switch mode {
	case ShowSymbol, ShowTitle, ShowBoth:
		return mode
	}
	return dflt
}

// Lookups loads the values behind a table's lookup-backed fields. Inactive
// and soft-deleted values are kept, since rows that picked them still show
// them. A table with no such field costs no query.
func (c *Catalog) Lookups(api *crud.API, table string) (*LookupShow, error) {
	specs := c.lookups[table]
	show := &LookupShow{specs: specs, labels: map[string]map[string]lookupLabel{}, picks: map[string][]string{}}
	if len(specs) == 0 {
		return show, nil
	}
	groups := make([]any, 0, len(specs))
	seen := map[string]bool{}
	for _, spec := range specs {
		if !seen[spec.group] {
			seen[spec.group] = true
			groups = append(groups, spec.group)
		}
	}
	order := `"idx"`
	// a user schema that restates lookup_values may leave sort out
	if slices.Contains(c.Fields["lookup_values"], "sort") {
		order = `"sort" IS NULL, "sort", "idx"`
	}
	rows, err := api.Query(`SELECT "idx", "lookups_idx", "symbol", "title",
			"is_active"<>0 AND "is_deleted"=0 AS "live" FROM "lookup_values"
		WHERE "lookups_idx" IN (?`+strings.Repeat(",?", len(groups)-1)+`)
		ORDER BY `+order, groups...)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		group := row["lookups_idx"]
		if show.labels[group] == nil {
			show.labels[group] = map[string]lookupLabel{}
		}
		show.labels[group][row["idx"]] = lookupLabel{symbol: row["symbol"], title: row["title"]}
		if row["live"] == "1" {
			show.picks[group] = append(show.picks[group], row["idx"])
		}
	}
	return show, nil
}

// IsLookup reports whether a field is lookup-backed.
func (l *LookupShow) IsLookup(field string) bool {
	if l == nil {
		return false
	}
	_, ok := l.specs[field]
	return ok
}

// List shows a field's value as lookup_in_lists says. Anything that isn't a
// lookup field, or a value the group doesn't hold, comes back as stored.
func (l *LookupShow) List(field, val string) string {
	if l == nil {
		return val
	}
	return l.show(field, val, l.specs[field].list)
}

// Form shows a field's value as lookup_in_forms says, else as List does.
func (l *LookupShow) Form(field, val string) string {
	if l == nil {
		return val
	}
	return l.show(field, val, l.specs[field].form)
}

func (l *LookupShow) show(field, val, mode string) string {
	spec, ok := l.specs[field]
	if !ok {
		return val
	}
	label, ok := l.labels[spec.group][strings.TrimSpace(val)]
	if !ok {
		return val
	}
	symbol := label.symbol
	if symbol != "" && l.SymbolOK != nil && !l.SymbolOK(symbol) {
		symbol = ""
	}
	switch {
	case symbol == "" && label.title == "":
		return val
	case symbol == "":
		return label.title
	case label.title == "", mode == ShowSymbol:
		return symbol
	case mode == ShowTitle:
		return label.title
	}
	return symbol + " " + label.title
}

// LookupChoice is one entry in a lookup field's picker.
type LookupChoice struct {
	Value, Label string
	Selected     bool
}

// Choices is what a form offers for a picker field: none, then the options -
// for a lookup, the group's active values in sort order, labeled as Form
// shows them. A value the row already holds that isn't among them is kept,
// second, so a save that leaves the field alone doesn't drop it. Nil for a
// field with no picker.
func (l *LookupShow) Choices(field, current string) []LookupChoice {
	if l == nil {
		return nil
	}
	if opts, ok := l.options[field]; ok {
		return withHeld(opts, current, current)
	}
	if !l.IsLookup(field) {
		return nil
	}
	picks := l.picks[l.specs[field].group]
	opts := make([]LookupChoice, len(picks))
	for i, idx := range picks {
		opts[i] = LookupChoice{Value: idx, Label: l.Form(field, idx)}
	}
	return withHeld(opts, current, l.Form(field, current))
}

// HasPicker reports whether a form picks the field from a list.
func (l *LookupShow) HasPicker(field string) bool {
	if l == nil {
		return false
	}
	_, ok := l.options[field]
	return ok || l.IsLookup(field)
}

func withHeld(opts []LookupChoice, current, heldLabel string) []LookupChoice {
	held := strings.TrimSpace(current)
	choices := make([]LookupChoice, 0, len(opts)+2)
	choices = append(choices, LookupChoice{Selected: held == ""})
	at := -1
	for i, opt := range opts {
		if held != "" && strings.TrimSpace(opt.Value) == held {
			at = i
			break
		}
	}
	if held != "" && at < 0 {
		choices = append(choices, LookupChoice{Value: current, Label: heldLabel, Selected: true})
	}
	for i, opt := range opts {
		opt.Selected = i == at
		choices = append(choices, opt)
	}
	return choices
}
