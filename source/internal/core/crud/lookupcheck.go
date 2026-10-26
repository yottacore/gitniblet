// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/yottacore/gitniblet/internal/core/ddl"
)

// lookupGroup is the lookups idx a `list_type: lookup` field draws from, or 0
// when it has none. A bad list_source is the catalog's to warn about.
func lookupGroup(field ddl.Field) int {
	if !strings.EqualFold(field.UI.ListType, "lookup") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(field.UI.ListSource))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// checkLookups refuses a lookup-backed value that isn't an active value in
// its group (design.md, Lookups). Only here, never at replay, so a value
// picked while active still applies after it's retired. A row that already
// holds an inactive or deleted value may write it back, as a form save does.
// id is "" on create. Values come back as the bare idx, which is what the
// display matches.
func (a *API) checkLookups(table, id string, fields map[string]string) (map[string]string, error) {
	groups := a.lookups[table]
	if len(groups) == 0 {
		return fields, nil
	}
	var out map[string]string
	var current map[string]string
	for field, val := range fields {
		group, ok := groups[field]
		if !ok || strings.TrimSpace(val) == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("crud: %s.%s: %q isn't a value in lookup group %d", table, field, val, group)
		}
		var live sql.NullInt64
		if err := a.st.DB().QueryRow(
			`SELECT MAX("is_active"<>0 AND "is_deleted"=0) FROM "lookup_values" WHERE "idx"=? AND "lookups_idx"=?`,
			n, group).Scan(&live); err != nil {
			return nil, fmt.Errorf("crud: %s.%s: %w", table, field, err)
		}
		if !live.Valid {
			return nil, fmt.Errorf("crud: %s.%s: %d isn't a value in lookup group %d", table, field, n, group)
		}
		if live.Int64 == 0 {
			if current == nil && id != "" {
				if current, _, err = a.Get(table, id); err != nil {
					return nil, err
				}
			}
			if strings.TrimSpace(current[field]) != strconv.Itoa(n) {
				return nil, fmt.Errorf("crud: %s.%s: %d isn't active in lookup group %d", table, field, n, group)
			}
		}
		if bare := strconv.Itoa(n); bare != val {
			if out == nil {
				out = make(map[string]string, len(fields))
				for k, v := range fields {
					out[k] = v
				}
			}
			out[field] = bare
		}
	}
	if out != nil {
		return out, nil
	}
	return fields, nil
}
