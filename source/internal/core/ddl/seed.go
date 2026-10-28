// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"strconv"
	"strings"

	"github.com/yottacore/gitniblet/internal/core/shclio"
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

// legacyRow is a seed row from before shcl 3, `row: idx=1, title="a, b"`, in
// 3's bracket form, `row: [idx=1, "title=a, b"]`. The 1.2 rules kept a
// quoted value's commas and applied its escapes, and the upgrade's 2.x
// reading splits it at the comma instead. shclwrap mends that only for a key
// set once, and seed rows repeat (2026102621002700).
func legacyRow(line string) (string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || strings.TrimSpace(key) != "row" {
		return "", false
	}
	pairs, comment := splitRow(value)
	if len(pairs) == 0 || strings.HasPrefix(pairs[0], "[") {
		return "", false
	}
	elems := make([]string, len(pairs))
	for i, pair := range pairs {
		if name, val, ok := strings.Cut(pair, "="); ok {
			pair = strings.TrimSpace(name) + "=" + unescape(strings.TrimSpace(val))
		}
		elems[i] = rowElement(pair)
	}
	out := key + ": [" + strings.Join(elems, ", ") + "]"
	if comment != "" {
		out += "  " + comment
	}
	return out, true
}

// splitRow splits a row's value on the commas outside quotes, and cuts off
// a trailing comment, which is a # after a blank outside quotes.
func splitRow(value string) (pairs []string, comment string) {
	var quote byte
	start := 0
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch {
		case quote != 0 && ch == '\\' && quote == '"':
			i++
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
		case ch == ',':
			pairs = append(pairs, strings.TrimSpace(value[start:i]))
			start = i + 1
		case ch == '#' && i > 0 && (value[i-1] == ' ' || value[i-1] == '\t'):
			comment = strings.TrimSpace(value[i:])
			value = value[:i]
		}
	}
	if last := strings.TrimSpace(value[start:]); last != "" || len(pairs) > 0 {
		pairs = append(pairs, last)
	}
	return pairs, comment
}

// unescape is a quoted 1.2 value's text: its outer quotes off and, in double
// quotes, its backslash escapes applied.
func unescape(val string) string {
	inner := unquote(val)
	if inner == val || val[0] != '"' {
		return inner
	}
	return strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\'`, `'`, `\t`, "\t", `\n`, "\n").Replace(inner)
}

// rowElement is one bracket element, quoted whole when 3 wouldn't read it bare.
func rowElement(pair string) string {
	bare := pair != "" && strings.IndexFunc(pair, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.=+-/@", r))
	}) < 0
	switch {
	case bare:
		return pair
	case !strings.Contains(pair, `"`):
		return `"` + pair + `"`
	case !strings.Contains(pair, "'"):
		return "'" + pair + "'"
	}
	return shclio.Quote(pair)
}
