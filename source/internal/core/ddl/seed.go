// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"strconv"
	"strings"
)

// SeedRow is one `row:` under seed:. Fields always holds a valid idx, since
// that is how open tells a row it seeded before from one it hasn't.
type SeedRow struct {
	Table  string
	Idx    int
	Fields map[string]string
	Line   int
}

// seedable are the tables seed: may name. They're the ones with a unique,
// fixed idx (design.md, Lookups), which is what keeps seeding idempotent.
var seedable = map[string]bool{"lookups": true, "lookup_values": true}

// mapSeeds reads seed: rows from either section prefix. A bad row is skipped
// with a warning rather than counted as an error: a missing seed row costs a
// pick-list value, not data, so it shouldn't grey out the database.
func (s *Schema) mapSeeds(root cursor) {
	seen := map[string]map[int]bool{}
	for _, prefix := range sectionPrefixes("database") {
		at := cursor{doc: root.doc, path: prefix}.child("seed")
		for i, table := range at.instances("table") {
			tc := at.instance("table", i)
			if !seedable[table] {
				s.warnAt(tc, "seed: table %q is skipped; only lookups and lookup_values take seed rows", table)
				continue
			}
			if seen[table] == nil {
				seen[table] = map[int]bool{}
			}
			for j := 0; j < tc.count("row"); j++ {
				rc := tc.instance("row", j)
				row, ok := s.parseSeedRow(rc, table)
				if !ok {
					continue
				}
				if seen[table][row.Idx] {
					s.warnAt(rc, "seed: %s idx %d is seeded more than once; the first row wins", table, row.Idx)
					continue
				}
				seen[table][row.Idx] = true
				s.Seeds = append(s.Seeds, row)
			}
		}
	}
}

// parseSeedRow reads `row: [idx=1, title=Open]`. Each element is one pair,
// and a value may still be in quotes inside it, as `title='Open'`.
func (s *Schema) parseSeedRow(rc cursor, table string) (SeedRow, bool) {
	row := SeedRow{Table: table, Fields: map[string]string{}, Line: rc.line()}
	for _, pair := range rc.list("") {
		key, val, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			s.warnAt(rc, "seed: %s row is skipped; %q isn't a field=value pair", table, pair)
			return row, false
		}
		if _, dup := row.Fields[key]; dup {
			s.warnAt(rc, "seed: %s row names %q twice; the first value wins", table, key)
			continue
		}
		row.Fields[key] = unquote(strings.TrimSpace(val))
	}
	idx, err := strconv.Atoi(row.Fields["idx"])
	if err != nil || idx < 1 {
		s.warnAt(rc, "seed: %s row is skipped; it needs an idx that's a whole number above 0", table)
		return row, false
	}
	row.Idx = idx
	row.Fields["idx"] = strconv.Itoa(idx)
	return row, true
}

// unquote strips one matching pair of outer quotes, the same pair shcl would
// strip from a whole element.
func unquote(val string) string {
	if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		return val[1 : len(val)-1]
	}
	return val
}
