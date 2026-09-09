// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Schema-op rewriting: rename a table or field IN the DDL file while recording
// the old name as an alias, so tx-log entries written before the rename still
// resolve (schema.ApplyAliases does the replay-side mapping).
//
// This goes through shcl's Writer, which carries comments and blank-line
// grouping across the edit and whose output is a formatter fixpoint. The file
// comes back in canonical form, so a first rename normalizes any hand-tuned
// column padding - comment text itself is never rewritten.
package ddl

import (
	"fmt"

	shcl "github.com/jim-collier/shcl/source/go"
)

// tablePaths is where tables live. The DDL also accepts an unwrapped `tables:`
// at the top level, so both prefixes are tried in order.
func tablePaths() []string {
	var out []string
	for _, prefix := range sectionPrefixes("database") {
		out = append(out, cursor{path: prefix}.at("tables.table"))
	}
	return out
}

// RenameTable renames a table and records the old name on its aliases. Returns
// the new text and whether the table was found.
func RenameTable(src []byte, oldName, newName string) ([]byte, bool) {
	doc := shcl.Parse(string(src))
	for _, base := range tablePaths() {
		i, ok := indexOf(doc, base, oldName)
		if !ok {
			continue
		}
		at := fmt.Sprintf("%s[#%d]", base, i)
		if !doc.SetString(at, newName) {
			return src, false
		}
		addAlias(doc, at, oldName)
		return []byte(doc.ToCanonical()), true
	}
	return src, false
}

// RenameField renames a field inside the named table, same contract.
func RenameField(src []byte, table, oldName, newName string) ([]byte, bool) {
	doc := shcl.Parse(string(src))
	for _, base := range tablePaths() {
		tableIdx, ok := indexOf(doc, base, table)
		if !ok {
			continue
		}
		fields := fmt.Sprintf("%s[#%d].fields.field", base, tableIdx)
		i, ok := indexOf(doc, fields, oldName)
		if !ok {
			continue
		}
		at := fmt.Sprintf("%s[#%d]", fields, i)
		if !doc.SetString(at, newName) {
			return src, false
		}
		addAlias(doc, at, oldName)
		return []byte(doc.ToCanonical()), true
	}
	return src, false
}

// indexOf finds the instance under path whose discriminator is name. Matching is
// exact: a discriminator is a value, and shcl compares values case-sensitively.
func indexOf(doc *shcl.Document, path, name string) (int, bool) {
	for i, value := range doc.Instances(path) {
		if value == name {
			return i, true
		}
	}
	return 0, false
}

// addAlias appends the old name to an entity's aliases, creating the key when
// absent. An already-listed name is not repeated, so renaming A->B->A leaves one
// entry rather than two.
func addAlias(doc *shcl.Document, entity, oldName string) {
	path := entity + ".aliases"
	existing := doc.GetStringArrayOr(path, nil)
	for _, alias := range existing {
		if alias == oldName {
			return
		}
	}
	doc.SetStringArray(path, append(existing, oldName))
}
