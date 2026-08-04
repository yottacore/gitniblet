// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"fmt"
	"strings"

	shcl "github.com/jim-collier/shcl/source/go"
)

// cursor is a position in an SHCL document: the document plus an absolute path
// prefix. The mappers walk the DDL by descending cursors, so each one reads
// short relative names instead of assembling long paths by hand.
//
// SHCL's Go API is path-based - there is no exported tree to walk - so a
// repeated entity (table, field, view, block) is reached by index: count the
// instances, then descend into each with a [#i] selector.
type cursor struct {
	doc  *shcl.Document
	path string // "" at the document root
}

// at joins a relative name onto the cursor's path.
func (c cursor) at(rel string) string {
	if c.path == "" {
		return rel
	}
	if rel == "" {
		return c.path
	}
	return c.path + "." + rel
}

// child descends into a named wrapper (`fields:`, `validation:`, `ui:`).
func (c cursor) child(rel string) cursor {
	return cursor{doc: c.doc, path: c.at(rel)}
}

// count reports how many instances of a repeated name sit under the cursor.
func (c cursor) count(rel string) int { return c.doc.Count(c.at(rel)) }

// instances lists a repeated name's discriminator values, in file order. For
// the DDL's entities that value IS the name (`table: issue`).
func (c cursor) instances(rel string) []string { return c.doc.Instances(c.at(rel)) }

// instance descends into one indexed occurrence of a repeated name. The index
// form is deliberate: a value selector would misread an entity whose name is
// numeric, and would also collapse two same-named entities into one.
func (c cursor) instance(rel string, i int) cursor {
	return cursor{doc: c.doc, path: fmt.Sprintf("%s[#%d]", c.at(rel), i)}
}

// exists reports whether a relative path resolves at all, which is how an
// optional section (`access:`, `features:`) is told from an absent one.
func (c cursor) exists(rel string) bool { return c.doc.Exists(c.at(rel)) }

// line is the source line a path was written on, 0 when it resolves to nothing
// or to more than one node. It lets a warning about the relationship between
// lines still point at one of them.
func (c cursor) line(rel ...string) int {
	if len(rel) == 0 {
		return c.doc.Line(c.path)
	}
	return c.doc.Line(c.at(rel[0]))
}

// children lists a section's child names in file order, for the sections whose
// keys are open rather than a closed vocabulary. Names come back as written, so
// splice one into a path with shcl.QuoteSegment.
func (c cursor) children(rel string) []string { return c.doc.Children(c.at(rel)) }

// single resolves a scalar path that may have collected more than one instance,
// which happens when a DDL restates the same key at the same level. The DDL's
// rule is that the first value wins; SHCL reports the ambiguity as Multiple and
// keeps both, so reading [#0] applies the rule. shcl also emits its repeated-leaf
// hint for the same line, so the restatement is never silent.
func (c cursor) single(rel string) string {
	path := c.at(rel)
	if c.doc.Count(path) > 1 {
		return path + "[#0]"
	}
	return path
}

// str reads a plain string leaf, empty when absent.
func (c cursor) str(rel string) string { return c.doc.GetStringOr(c.single(rel), "") }

// firstStr returns the first of several spellings that resolves, for keys
// accepted under more than one name.
func (c cursor) firstStr(rels ...string) string {
	for _, rel := range rels {
		if value := c.str(rel); value != "" {
			return value
		}
	}
	return ""
}

// lower reads a string leaf folded to lowercase - the DDL's keyword-ish values
// (types, encryption modes, relationship kinds) are case-insensitive.
func (c cursor) lower(rel string) string { return strings.ToLower(c.str(rel)) }

// raw reads a leaf's original text, bypassing SHCL's value model. Needed where
// a value legitimately contains reserved characters and must survive verbatim:
// backtick SQL and regexes both hold commas, which a normal string read would
// treat as array separators and re-quote. A fenced raw block reads through the
// same call, so either spelling works.
func (c cursor) raw(rel string) string {
	path := c.single(rel)
	if block, st := c.doc.GetRaw(path); st == shcl.Good {
		return block
	}
	if r := c.doc.ReadString(path); r.Raw != nil {
		return *r.Raw
	}
	return ""
}

// list reads a comma list (or a stacked `*` list - SHCL treats them alike).
func (c cursor) list(rel string) []string {
	return c.doc.GetStringArrayOr(c.single(rel), nil)
}

// firstList returns the first of several spellings that resolves, for keys that
// have been misspelled historically and are still accepted.
func (c cursor) firstList(rels ...string) []string {
	for _, rel := range rels {
		if c.exists(rel) {
			return c.list(rel)
		}
	}
	return nil
}

// The pointer readers keep the DDL's unset/default distinction: nil means "the
// key was absent or unusable, use the default", which is not the same as an
// explicit false/0. An empty value reads as absent for this purpose - `width:`
// with nothing after it states no width.

func (c cursor) boolPtr(rel string) *bool {
	if r := c.doc.ReadBool(c.single(rel)); r.Status == shcl.Good {
		value := r.Value
		return &value
	}
	return nil
}

func (c cursor) intPtr(rel string) *int {
	if r := c.doc.ReadInt(c.single(rel)); r.Status == shcl.Good {
		value := int(r.Value)
		return &value
	}
	return nil
}

func (c cursor) floatPtr(rel string) *float64 {
	if r := c.doc.ReadFloat(c.single(rel)); r.Status == shcl.Good {
		value := r.Value
		return &value
	}
	return nil
}

// boolOr is the flag form, for keys that simply default off.
func (c cursor) boolOr(rel string, def bool) bool {
	if val := c.boolPtr(rel); val != nil {
		return *val
	}
	return def
}

// codeMap collects a `code:` section's hooks by reading the section itself, so
// the hook a DDL actually wrote is the hook that gets recorded - schema
// validation is what rejects a name that is not a real hook, and it does that
// with a line number. Empty entries are placeholders the example DDL is full
// of, so they are skipped rather than recorded as an empty hook.
func (c cursor) codeMap(rel string) map[string]string {
	out := map[string]string{}
	if !c.exists(rel) {
		rel = "methods" // pre-2026-07 name for code:
		if !c.exists(rel) {
			return out
		}
	}
	section := c.child(rel)
	for _, name := range section.children("") {
		if _, dup := out[name]; dup {
			continue // first wins, like every other merge
		}
		if fn := section.str(shcl.QuoteSegment(name)); fn != "" {
			out[name] = fn
		}
	}
	return out
}
