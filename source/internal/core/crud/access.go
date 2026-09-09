// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Access enforcement (design "Optional granular access model"). The write
// side is gated here, in the one API everything funnels through; the read
// side (hiding tables/fields/views/rows) lives in schema.Catalog, since a
// process serves one user. Zero configured lists = zero queries = today's
// open behavior; identity is the stamp user resolved through the users table.
package crud

import (
	"fmt"
	"sort"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// UserGroups resolves the stamp user (UserID) to its group-name and group-id
// sets: a live users row whose screen_name or github_name matches, then the
// users<->groups m:m. No user row means no memberships - with no access
// lists defined everything stays open, so a userless setup keeps working.
func (a *API) UserGroups() (names, ids map[string]bool, err error) {
	names, ids = map[string]bool{}, map[string]bool{}
	rows, err := a.Query(`SELECT "id" FROM "users"
		WHERE ("screen_name"=? OR "github_name"=?) AND "is_deleted"=0`, a.UserID, a.UserID)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return names, ids, nil
	}
	groupIDs, err := a.Links("users", rows[0]["id"], "groups")
	if err != nil {
		return nil, nil, err
	}
	for _, groupID := range groupIDs {
		group, ok, err := a.Get("groups", groupID)
		if err != nil {
			return nil, nil, err
		}
		if ok && group["is_deleted"] != "1" {
			names[group["name"]] = true
			ids[groupID] = true
		}
	}
	return names, ids, nil
}

// authorize gates one write-side operation (op "write" or "delete"). fields
// are the columns a write touches, for field-level rules. Row-level grants
// apply when the table opted in and the row already exists.
func (a *API) authorize(table, id, op string, fields map[string]string) error {
	if a.access == nil { // EnableFeatures never ran: bare API, everything open
		return nil
	}
	tableAccess := a.access[table]
	var fieldChecks []string
	for f := range fields {
		if fieldAcc, ok := a.fieldAccess[table][f]; ok && !emptyAccess(fieldAcc) {
			fieldChecks = append(fieldChecks, f)
		}
	}
	rowLevel := a.features[table].RowLevelAccess && id != ""
	if emptyAccess(tableAccess) && len(fieldChecks) == 0 && !rowLevel {
		return nil
	}
	groups, groupIDs, err := a.UserGroups()
	if err != nil {
		return fmt.Errorf("crud: access check: %w", err)
	}
	if !tableAccess.AllowedFor(op, groups) {
		return fmt.Errorf("crud: access denied: %s on table %q", op, table)
	}
	sort.Strings(fieldChecks) // deterministic error
	for _, f := range fieldChecks {
		if !a.fieldAccess[table][f].AllowedFor("write", groups) {
			return fmt.Errorf("crud: access denied: write to %s.%s", table, f)
		}
	}
	if rowLevel && !groups["owners"] && !groups["admins"] {
		granted, err := a.RowAccessGroups(table, id)
		if err != nil {
			return fmt.Errorf("crud: access check: %w", err)
		}
		if len(granted) > 0 && !anyIn(granted, groupIDs) {
			return fmt.Errorf("crud: access denied: row %s of %q is restricted", id, table)
		}
	}
	return nil
}

func emptyAccess(access ddl.Access) bool {
	return len(access.Read.Whitelist)+len(access.Read.Blacklist)+
		len(access.Write.Whitelist)+len(access.Write.Blacklist)+
		len(access.Delete.Whitelist)+len(access.Delete.Blacklist) == 0
}

func anyIn(list []string, set map[string]bool) bool {
	for _, s := range list {
		if set[s] {
			return true
		}
	}
	return false
}
