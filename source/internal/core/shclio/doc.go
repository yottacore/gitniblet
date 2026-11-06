// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package shclio

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	shcl "github.com/yottacore/shcl/source/go/v3"
)

// Doc is a parsed file. It keeps the file's lines, so a save changes only
// what was edited.
type Doc struct {
	d     *shcl.Document
	stamp bool
}

// Got is one typed read. Text is the value as written.
type Got[T any] struct {
	Value  T
	Text   string
	Status shcl.Status
	Line   int
}

// Ok is true for a path that was addressed, even with nothing after it.
func (g Got[T]) Ok() bool { return g.Status == shcl.Good || g.Status == shcl.Empty }

// Node is one instance of a repeated name.
type Node struct {
	Path  string
	Value string
	Line  int
}

func got[T any](r shcl.Read[T]) Got[T] {
	g := Got[T]{Value: r.Value, Status: r.Status, Line: r.Line}
	if r.Raw != nil {
		g.Text = *r.Raw
	}
	return g
}

// Diags is what the load found.
func (d *Doc) Diags() []Diag { return d.d.Diagnostics() }

// Errors counts the error diagnostics.
func (d *Doc) Errors() int { return d.d.ErrorCount() }

// Check validates the document against a schema document.
func (d *Doc) Check(schema *Doc) []Diag { return d.d.Validate(schema.d) }

func (d *Doc) Str(path, fallback string) string { return d.d.GetStringOr(path, fallback) }

func (d *Doc) Strs(path string, fallback []string) []string {
	return d.d.GetStringArrayOr(path, fallback)
}

func (d *Doc) GetInt(path string) Got[int64]  { return got(d.d.ReadInt(path)) }
func (d *Doc) GetBool(path string) Got[bool]  { return got(d.d.ReadBool(path)) }
func (d *Doc) GetStr(path string) Got[string] { return got(d.d.ReadString(path)) }
func (d *Doc) GetRaw(path string) Got[string] { return got(d.d.ReadRaw(path)) }

// GetFloat reads a number, and takes infinity and NaN for the wrong type.
func (d *Doc) GetFloat(path string) Got[float64] {
	g := got(d.d.ReadFloat(path))
	if g.Status == shcl.Good && (math.IsInf(g.Value, 0) || math.IsNaN(g.Value)) {
		g.Status, g.Value = shcl.BadType, 0
	}
	return g
}

func (d *Doc) Has(path string) bool  { return d.d.Exists(path) }
func (d *Doc) Count(path string) int { return d.d.Count(path) }
func (d *Doc) Line(path string) int  { return d.d.Line(path) }

// Names lists the child names under path once each, in file order. Case
// folds for ASCII only.
func (d *Doc) Names(path string) []string {
	names := d.d.Children(path)
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		k := asciiLower(n)
		if !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	return out
}

// asciiLower folds A to Z only, since shcl keeps names like E and e-acute apart.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// Each is every instance path names, each with a path that reads and edits
// it alone. A wildcard slot that reaches no single node is left out.
func (d *Doc) Each(path string) []Node {
	fields := d.d.ReadFields(path).Value
	out := make([]Node, 0, len(fields))
	for _, f := range fields {
		if f.Path != "" {
			out = append(out, Node{Path: f.Path, Value: f.Value, Line: f.Line})
		}
	}
	return out
}

// Child is base with one more name on the end, quoted if it needs it.
func (d *Doc) Child(base, name string) string {
	if base == "" {
		return shcl.QuoteSegment(name)
	}
	return base + "." + shcl.QuoteSegment(name)
}

// First is path narrowed to its first instance in the file, so a read or an
// edit takes that one. A path with no instances comes back as it is.
func (d *Doc) First(path string) string {
	nodes := d.Each(path)
	if len(nodes) == 0 {
		return path
	}
	key := func(n Node) int {
		if n.Line == 0 {
			return math.MaxInt
		}
		return n.Line
	}
	slices.SortStableFunc(nodes, func(a, b Node) int { return cmp.Compare(key(a), key(b)) })
	return nodes[0].Path
}

// Put sets path to v, which is an int, int64, float64, bool, string, or a
// slice of strings. A value that doesn't read back the same is an error,
// with the edit left in place.
func (d *Doc) Put(path string, v any) error {
	var st shcl.SetStatus
	switch x := v.(type) {
	case int:
		st = d.d.SetInt(path, int64(x))
	case int64:
		st = d.d.SetInt(path, x)
	case float64:
		st = d.d.SetFloat(path, x)
	case bool:
		st = d.d.SetBool(path, x)
	case string:
		st = d.d.SetString(path, x)
	case []string:
		st = d.d.SetStringArray(path, x)
	default:
		return fmt.Errorf("shclio: %s: can't write a %T", path, v)
	}
	if st != shcl.SetOk {
		return fmt.Errorf("shclio: %s: %s", path, st)
	}
	switch x := v.(type) {
	case string:
		if r := d.d.ReadString(path); r.Value != x {
			return fmt.Errorf("shclio: %s: value doesn't read back", path)
		}
	case []string:
		if r := d.d.ReadStringArray(path); !slices.Equal(r.Value, x) {
			return fmt.Errorf("shclio: %s: value doesn't read back", path)
		}
	}
	return nil
}

// StampFormat adds the Format line at the end of what Text returns, unless
// the file already names one.
func (d *Doc) StampFormat() error {
	d.stamp = true
	return nil
}

// Text is the document as a save writes it: the file's own lines with the
// edits in, or the canonical form where the lines can't be kept.
func (d *Doc) Text() string {
	text, _ := d.d.ToTextKeepLines()
	if !d.stamp || FormatOf(text) >= shcl.FormatMajor {
		return text
	}
	stamped := strings.TrimRight(text, "\n")
	if stamped != "" {
		stamped += "\n"
	}
	stamped += shcl.FormatLine + "\n"
	// A stamp that changes what the file reads as is dropped.
	if FormatOf(stamped) != shcl.FormatMajor || shcl.Parse(stamped).ErrorCount() != d.d.ErrorCount() {
		return text
	}
	return stamped
}

// FormatOf is the format a file's Format line names, or 0 with none.
func FormatOf(text string) int {
	n, ok := shcl.FormatVersion(text)
	if !ok {
		return 0
	}
	return n
}

// DropDeclared takes out the repeat and reopen hints for keys this schema
// says repeat.
func (d *Doc) DropDeclared(ds []Diag) []Diag {
	in := shcl.SuppressDeclaredRepeats(d.d, ds)
	return shcl.SuppressDeclaredReopens(d.d, in)
}
