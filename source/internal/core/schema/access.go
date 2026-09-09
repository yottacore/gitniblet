// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Read-side access filtering (design "Optional granular access model"): a
// process serves one user, so the catalog simply hides what that user may
// not read - tables, fields, views, and row-granted rows. Write enforcement
// lives in crud (access.go there); this is the UI half.
package schema

import (
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// applyAccess prunes the catalog to the user's read access. tableAccess is
// the first-wins table rule map gathered while walking the schemas. Runs
// after resolveViews (views bind tables) and before resolveDefaultView (the
// default must be a view the user can open).
func (c *Catalog) applyAccess(api *crud.API, tableAccess map[string]ddl.Access, fieldAccess map[string]map[string]ddl.Access) {
	groups, _, err := api.UserGroups()
	if err != nil {
		// no users/groups tables (bare store): nothing is configured, stay open
		return
	}
	readable := func(table string) bool { return tableAccess[table].AllowedFor("read", groups) }

	kept := c.Tables[:0]
	for _, table := range c.Tables {
		if !readable(table) {
			c.warnf("table %q hidden: read access denied", table)
			delete(c.Fields, table) // Has() now refuses it, so the front-ends 404
			continue
		}
		kept = append(kept, table)
	}
	c.Tables = kept

	for table, fields := range c.Fields {
		keptFields := fields[:0]
		for _, field := range fields {
			if access, ok := fieldAccess[table][field]; ok && !access.AllowedFor("read", groups) {
				continue // silently: a hidden column is not an error
			}
			keptFields = append(keptFields, field)
		}
		c.Fields[table] = keptFields
	}

	// A view needs its own flat rule to pass AND read access to its main
	// (first) table, per the design.
	keptViews := c.Views[:0]
	for i := range c.Views {
		view := c.Views[i]
		open := groups["owners"] || groups["admins"] || view.Access.Allows(groups)
		if open && len(view.Leaves) > 0 {
			open = readable(view.Leaves[0].Table)
		}
		if !open {
			c.warnf("view %q hidden: access denied", view.Name)
			continue
		}
		keptViews = append(keptViews, view)
	}
	c.Views = keptViews
}

// liveRowsAllowed filters a row-level-access table's rows down to the ones
// the user may see: rows with no access_rows entry (or an entry with no
// grants) are open, granted rows need a shared group. Owners/admins see all.
func (c *Catalog) liveRowsAllowed(api *crud.API, table string, rows []map[string]string) []map[string]string {
	if !c.Features[table].RowLevelAccess {
		return rows
	}
	groups, gids, err := api.UserGroups()
	if err != nil || groups["owners"] || groups["admins"] {
		return rows // hiding is UI-level; the hard gate is crud's
	}
	accessRows, err := api.Query(`SELECT "id","parent_id" FROM "access_rows"
		WHERE "table_name"=? AND "is_deleted"=0`, table)
	if err != nil || len(accessRows) == 0 {
		return rows
	}
	blocked := map[string]bool{}
	for _, entry := range accessRows {
		granted, err := api.Links("access_rows", entry["id"], "groups")
		if err != nil || len(granted) == 0 {
			continue
		}
		ok := false
		for _, g := range granted {
			if gids[g] {
				ok = true
				break
			}
		}
		if !ok {
			blocked[entry["parent_id"]] = true
		}
	}
	if len(blocked) == 0 {
		return rows
	}
	kept := rows[:0]
	for _, r := range rows {
		if !blocked[r["id"]] {
			kept = append(kept, r)
		}
	}
	return kept
}
