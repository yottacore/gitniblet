// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"fmt"
	"strconv"
)

// autoIdx are the tables whose int idx is the reference key (design.md,
// Lookups): assigned max+1 on create, fixed once set.
var autoIdx = map[string]bool{"lookups": true, "lookup_values": true}

// withIdx returns fields with an idx in it, minting max+1 when none was
// given. Callers hold idxMu through the commit, or two creates mint the same
// one. Deleted rows count toward the max, so an idx is never handed out twice
// and a stale reference can't wake up pointing at a new value.
func (a *API) withIdx(table string, fields map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(fields)+1)
	for k, v := range fields {
		out[k] = v
	}
	if given := out["idx"]; given != "" {
		n, err := strconv.Atoi(given)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("crud: %s idx must be a whole number above 0, got %q", table, given)
		}
		var taken int
		if err := a.st.DB().QueryRow(
			`SELECT COUNT(*) FROM `+quoteIdent(table)+` WHERE "idx"=? AND "is_deleted"=0`, n).Scan(&taken); err != nil {
			return nil, err
		}
		if taken > 0 {
			return nil, fmt.Errorf("crud: %s idx %d is taken", table, n)
		}
		out["idx"] = strconv.Itoa(n)
		return out, nil
	}
	var top int64
	// typeof skips a stray non-numeric value a replay let in, which would
	// otherwise sort above every integer
	if err := a.st.DB().QueryRow(
		`SELECT COALESCE(MAX("idx"),0) FROM ` + quoteIdent(table) + ` WHERE typeof("idx")='integer'`).Scan(&top); err != nil {
		return nil, err
	}
	out["idx"] = strconv.FormatInt(top+1, 10)
	return out, nil
}

// keepIdx refuses a change to a set idx: lookup-backed fields store it, so
// changing it would re-point or orphan them. Writing the same value back, as a
// form save does, is dropped rather than refused.
func (a *API) keepIdx(table, id string, fields map[string]string) (map[string]string, error) {
	val, ok := fields["idx"]
	if !autoIdx[table] || !ok {
		return fields, nil
	}
	row, found, err := a.Get(table, id)
	if err != nil {
		return nil, err
	}
	if found && row["idx"] != "" {
		if val != row["idx"] {
			return nil, fmt.Errorf("crud: %s idx is fixed once set", table)
		}
		out := make(map[string]string, len(fields))
		for k, v := range fields {
			if k != "idx" {
				out[k] = v
			}
		}
		return out, nil
	}
	return fields, nil
}
