// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yottacore/gitniblet/internal/core/crud"
	"github.com/yottacore/gitniblet/internal/core/ddl"
)

// SeedRows creates each DDL seed: row whose idx no row holds yet, through CRUD
// so it goes into the log. Call it after the replay, as with SeedDefaults.
// A deleted row still holds its idx, so deleting a seeded value sticks, the
// same as any other edit to it (design.md, Lookups).
//
// A row that can't go in only warns. The schema author wrote it, not the user
// opening the database, and a missing pick-list value isn't worth refusing
// the open over.
func SeedRows(api *crud.API, sch *ddl.Schema) []string {
	if len(sch.Seeds) == 0 {
		return nil
	}
	rows := append([]ddl.SeedRow(nil), sch.Seeds...)
	// groups before their values, in case a value's group ever gets checked
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Table == "lookups" && rows[j].Table != "lookups"
	})

	var warns []string
	held := map[string]map[int]bool{}
	cols := map[string]map[string]bool{}
	broken := map[string]bool{}
	for _, row := range rows {
		if broken[row.Table] {
			continue
		}
		if held[row.Table] == nil {
			var err error
			if cols[row.Table], held[row.Table], err = seedState(api, row.Table); err != nil {
				broken[row.Table] = true
				warns = append(warns, fmt.Sprintf("seed: %s rows skipped: %v", row.Table, err))
				continue
			}
		}
		if held[row.Table][row.Idx] {
			continue
		}
		if bad := unknownField(row, cols[row.Table]); bad != "" {
			warns = append(warns, fmt.Sprintf("seed: line %d: %s idx %d not added; %q isn't a field it can set", row.Line, row.Table, row.Idx, bad))
			continue
		}
		if _, err := api.Create(row.Table, row.Fields); err != nil {
			warns = append(warns, fmt.Sprintf("seed: line %d: %s idx %d not added: %v", row.Line, row.Table, row.Idx, err))
			continue
		}
		held[row.Table][row.Idx] = true
	}
	return warns
}

// seedState reads a table's columns and every idx it holds, deleted rows too.
func seedState(api *crud.API, table string) (map[string]bool, map[int]bool, error) {
	colRows, err := api.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, nil, err
	}
	cols := make(map[string]bool, len(colRows))
	for _, r := range colRows {
		cols[r["name"]] = true
	}
	if !cols["idx"] {
		return nil, nil, fmt.Errorf("the table has no idx field")
	}
	idxRows, err := api.Query(`SELECT "idx" FROM "` + strings.ReplaceAll(table, `"`, `""`) + `" WHERE "idx" IS NOT NULL`)
	if err != nil {
		return nil, nil, err
	}
	held := make(map[int]bool, len(idxRows))
	for _, r := range idxRows {
		if n, err := strconv.Atoi(r["idx"]); err == nil {
			held[n] = true
		}
	}
	return cols, held, nil
}

// unknownField names the first field a seed row can't set, or "". crud would
// log an unknown one before the view refused it, and id is never the DDL's
// to pick.
func unknownField(row ddl.SeedRow, cols map[string]bool) string {
	for _, field := range sortedFields(row.Fields) {
		if field == "id" || !cols[field] {
			return field
		}
	}
	return ""
}

func sortedFields(fields map[string]string) []string {
	out := make([]string, 0, len(fields))
	for k := range fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
