// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Schema-op rewriting: rename a table or field IN the DDL file while recording
// the old name as an alias, so tx-log entries written before the rename still
// resolve (schema.ApplyAliases does the replay-side mapping).
//
// The file keeps its lines as typed, comments and column padding included,
// apart from the lines the rename edits. A file from before shcl 3 comes back
// in 3's form, with its format named.
package ddl

import (
	"github.com/yottacore/gitniblet/internal/core/shclio"
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
	doc, _ := shclio.Load(string(src), shclio.Standard, legacyRow)
	for _, base := range tablePaths() {
		at, ok := instanceOf(doc, base, oldName)
		if !ok {
			continue
		}
		return renamed(src, doc, at, oldName, newName)
	}
	return src, false
}

// RenameField renames a field inside the named table, same contract.
func RenameField(src []byte, table, oldName, newName string) ([]byte, bool) {
	doc, _ := shclio.Load(string(src), shclio.Standard, legacyRow)
	for _, base := range tablePaths() {
		tableAt, ok := instanceOf(doc, base, table)
		if !ok {
			continue
		}
		at, ok := instanceOf(doc, tableAt+".fields.field", oldName)
		if !ok {
			continue
		}
		return renamed(src, doc, at, oldName, newName)
	}
	return src, false
}

func renamed(src []byte, doc *shclio.Doc, at, oldName, newName string) ([]byte, bool) {
	if doc.Put(at, newName) != nil || addAlias(doc, at, oldName) != nil {
		return src, false
	}
	return []byte(shclio.Text(doc)), true
}

// instanceOf finds the instance under path whose discriminator is name. Matching
// is exact: a discriminator is a value, and shcl compares values case-sensitively.
func instanceOf(doc *shclio.Doc, path, name string) (string, bool) {
	for _, n := range doc.Each(path) {
		if n.Value == name {
			return n.Path, true
		}
	}
	return "", false
}

// addAlias appends the old name to an entity's aliases, creating the key when
// absent. An already-listed name is not repeated, so renaming A->B->A leaves one
// entry rather than two.
func addAlias(doc *shclio.Doc, entity, oldName string) error {
	path := entity + ".aliases"
	existing := doc.Strs(path, nil)
	for _, alias := range existing {
		if alias == oldName {
			return nil
		}
	}
	return doc.Put(path, append(existing, oldName))
}
