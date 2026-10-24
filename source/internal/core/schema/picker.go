// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"fmt"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
)

// pickSpec is a `list_type: literal` or `sql` field's list_source.
type pickSpec struct {
	sql    bool
	source string
}

func (c *Catalog) findPickers(table string) map[string]pickSpec {
	var specs map[string]pickSpec
	for _, field := range c.Fields[table] {
		ui := c.fieldUI[table][field]
		kind := strings.ToLower(strings.TrimSpace(ui.ListType))
		if kind != "literal" && kind != "sql" {
			continue
		}
		source := strings.TrimSpace(ui.ListSource)
		if kind == "sql" {
			source = strings.TrimSpace(strings.Trim(source, "`"))
		}
		if source == "" {
			c.warnf("%s.%s: list_type %s has no list_source", table, field, kind)
			continue
		}
		if specs == nil {
			specs = map[string]pickSpec{}
		}
		specs[field] = pickSpec{sql: kind == "sql", source: source}
	}
	return specs
}

// FormChoices is Lookups plus the options of the table's literal and sql
// fields, for an edit form. An sql list_source gives the value in its first
// column and the label in its second, or the value again when there's only
// one. It runs on every form open, so it sees the view as it is now.
func (c *Catalog) FormChoices(api *crud.API, table string) (*LookupShow, error) {
	show, err := c.Lookups(api, table)
	if err != nil {
		return nil, err
	}
	specs := c.pickers[table]
	if len(specs) == 0 {
		return show, nil
	}
	show.options = make(map[string][]LookupChoice, len(specs))
	for field, spec := range specs {
		if !spec.sql {
			for _, val := range splitLiterals(spec.source) {
				show.options[field] = append(show.options[field], LookupChoice{Value: val, Label: val})
			}
			continue
		}
		cols, rows, err := api.QueryRows(spec.source)
		if err != nil {
			return nil, fmt.Errorf("%s.%s list_source: %w", table, field, err)
		}
		opts := make([]LookupChoice, 0, len(rows))
		for _, row := range rows {
			opt := LookupChoice{Value: row[cols[0]], Label: row[cols[0]]}
			if len(cols) > 1 {
				opt.Label = row[cols[1]]
			}
			opts = append(opts, opt)
		}
		show.options[field] = opts
	}
	return show, nil
}

// splitLiterals reads `"Bob", 'Sally', Ann` as its 3 values: commas inside
// quotes don't split, and one outer quote pair is dropped.
func splitLiterals(source string) []string {
	var vals []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		val := strings.TrimSpace(cur.String())
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if val != "" {
			vals = append(vals, val)
		}
		cur.Reset()
	}
	for _, r := range source {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote = r
		case quote == 0 && r == ',':
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return vals
}
