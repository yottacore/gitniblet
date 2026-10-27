// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
// SPDX-License-Identifier: MIT

// Package shclw has the types and helpers every shcl version adapter shares.
//
// A program imports the adapter package that matches the shcl it builds
// against, shclw3 for shcl 3.0. A later one will have the same calls, so
// moving to another shcl version is a change to the import line.
package shclw

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Status says how a read went.
type Status int

const (
	Good Status = iota
	Empty
	NotFound
	BadType
	Multiple
	BadPath
)

func (s Status) String() string {
	switch s {
	case Good:
		return "Good"
	case Empty:
		return "Empty"
	case NotFound:
		return "NotFound"
	case BadType:
		return "BadType"
	case Multiple:
		return "Multiple"
	case BadPath:
		return "BadPath"
	}
	return "Status(" + strconv.Itoa(int(s)) + ")"
}

// Strictness is how forgiving a load is. Only Strict can fail a load.
type Strictness int

const (
	Loose Strictness = iota
	Standard
	Strict
)

// Severity of a diagnostic.
type Severity int

const (
	SevError Severity = iota
	SevHint
)

func (s Severity) String() string {
	if s == SevError {
		return "error"
	}
	return "hint"
}

// Diag is one diagnostic from a load or a schema check. Line is 1-based.
// Layer is the 1-based place in OpenLayers' list of the layer the line is
// in, or 0 for a document that isn't layered.
type Diag struct {
	Line     int
	Severity Severity
	Code     string
	Message  string
	Layer    int
}

func (d Diag) String() string {
	if d.Layer > 0 {
		return fmt.Sprintf("layer %d line %d: %s %s", d.Layer, d.Line, d.Code, d.Message)
	}
	return fmt.Sprintf("line %d: %s %s", d.Line, d.Code, d.Message)
}

// Spot is a line in a loaded document. Layer is as in Diag.
type Spot struct {
	Line  int
	Layer int
}

// Merge is a name shcl kept one copy of, since a later copy had the same
// name and value and was folded into it. Lines is the kept copy's line, then
// each later copy's. Path reads the kept copy, as a Node.Path does, and Layer
// is as in Diag.
type Merge struct {
	Path  string
	Lines []int
	Layer int
}

// Layer is one layer for OpenLayers: a file, or with File empty, Text the
// program has, such as its defaults or this run's command line.
type Layer struct {
	File string
	Text string
}

// Got is a read with its status. Check Status before trusting Value: an
// array that reads as BadType still has its items, with 0 for the bad ones.
// Text is the value as typed in the file, when there was one.
type Got[T any] struct {
	Value  T
	Status Status
	Line   int
	Text   string
}

// Ok is true when the read found a usable value.
func (g Got[T]) Ok() bool { return g.Status == Good }

// Node is one instance found by Each. Path reads this instance and no
// other; add child names to it with Child.
type Node struct {
	Path  string
	Value string
	Line  int
}

// ErrKind sorts errors into what a caller can act on.
type ErrKind int

const (
	ErrNotFound ErrKind = iota + 1
	ErrIO
	ErrStrict
	ErrRefused
	ErrBadPath
	ErrBadValue
	ErrTooNew
	ErrUnsupported
	ErrAmbiguous
	ErrBackupTaken
	ErrNoReadBack
	ErrChanged
	ErrExists
)

func (k ErrKind) String() string {
	switch k {
	case ErrNotFound:
		return "notfound"
	case ErrIO:
		return "io"
	case ErrStrict:
		return "strict"
	case ErrRefused:
		return "refused"
	case ErrBadPath:
		return "badpath"
	case ErrBadValue:
		return "badvalue"
	case ErrTooNew:
		return "toonew"
	case ErrUnsupported:
		return "unsupported"
	case ErrAmbiguous:
		return "ambiguous"
	case ErrBackupTaken:
		return "backuptaken"
	case ErrNoReadBack:
		return "noreadback"
	case ErrChanged:
		return "changed"
	case ErrExists:
		return "exists"
	}
	return "unknown"
}

// Error is the one error type the adapters return. Diags is set for a
// failed strict load.
type Error struct {
	Kind  ErrKind
	Path  string
	Msg   string
	Diags []Diag
	Err   error
}

func (e *Error) Error() string {
	s := "shclw: " + e.Kind.String()
	if e.Path != "" {
		s += " " + strconv.Quote(e.Path)
	}
	if e.Msg != "" {
		s += ": " + e.Msg
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf is the ErrKind of err, or 0 when err isn't an *Error.
func KindOf(err error) ErrKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

// Upgraded is what an upgrade made of a file. From is the format it was
// written for. Unsure counts lines the upgrade couldn't convert for sure.
// Lost lists the lines of the original, from 1, with a value the old shcl
// read that the upgraded file doesn't have. Rewritten lists the rest of the
// original's lines that the upgraded text doesn't have as typed. A line that
// only moved isn't one.
type Upgraded struct {
	Text      string
	From      int
	Changed   bool
	Unsure    int
	Backup    string
	Lost      []int
	Rewritten []int
}

// RewrittenLines lists the lines of was, from 1, that now doesn't have as
// typed, leaving out the ones in lost. Lines are matched as a multiset, so a
// line that only moved isn't listed (design.md, Decisions).
func RewrittenLines(was, now string, lost []int) []int {
	have := map[string]int{}
	for _, l := range textLines(now) {
		have[l]++
	}
	var out []int
	for i, l := range textLines(was) {
		switch {
		case slices.Contains(lost, i+1):
		case have[l] > 0:
			have[l]--
		default:
			out = append(out, i+1)
		}
	}
	return out
}

// textLines splits on LF and keeps any CR, so a line whose ending changed
// counts as written differently.
func textLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// WriteOpts change what a Write does. The zero value is a plain Write.
//
// Force writes over the file Open read even when it changed on disk since.
// It doesn't write over a file in a newer format.
//
// Create writes only where nothing is there yet, and is ErrExists otherwise,
// even with 2 programs creating the file at once. It never replaces, so
// there's no changed check for Force to skip.
//
// Mode is the permission bits a file this Write creates gets, exactly, with
// no umask. 0 is a plain create's 0666 less the umask. A file that's already
// there keeps its own mode. Windows has no such bits and ignores it.
//
// Backup writes over the file Open read when the save would lose lines the
// load couldn't use, which is ErrRefused otherwise. It copies the file aside
// first, as the upgrade's backup does, and BackupName names the copy. Later
// Writes then don't refuse, since the lines are in the copy. Anywhere else
// there's no copy to make, and it's still ErrRefused.
type WriteOpts struct {
	Force  bool
	Create bool
	Backup bool
	Mode   fs.FileMode
}

// Doc is what every adapter's document does. Programs use the adapter's
// own *Doc; this interface is how shared code, like Run, drives any of them.
type Doc interface {
	Text() string
	Canonical() string
	Write(path string) error
	WriteWith(path string, o WriteOpts) error
	ReadOnly() bool
	Rewrites() bool
	Diags() []Diag
	Errors() int
	Unused() []Spot
	Merged() []Merge
	Commented() []Commented
	Check(schema Doc) []Diag
	DropDeclared(diags []Diag) []Diag

	Int(path string, fallback int64) int64
	Float(path string, fallback float64) float64
	Bool(path string, fallback bool) bool
	Str(path string, fallback string) string
	Strs(path string, fallback []string) []string
	Ints(path string, fallback []int64) []int64
	Floats(path string, fallback []float64) []float64
	Raw(path string, fallback string) string

	GetInt(path string) Got[int64]
	GetFloat(path string) Got[float64]
	GetBool(path string) Got[bool]
	GetStr(path string) Got[string]
	GetStrs(path string) Got[[]string]
	GetInts(path string) Got[[]int64]
	GetFloats(path string) Got[[]float64]
	GetRaw(path string) Got[string]

	Has(path string) bool
	Count(path string) int
	Names(path string) []string
	Each(path string) []Node
	Line(path string) int
	Lines(path string) []int
	First(path string) string
	Last(path string) string
	Child(base, name string) string

	Put(path string, v any) error
	PutSyntax(path, text string) error
	Comment(path, text string) error
	Remove(path string) int

	InfoBlock(on bool) error
	StampFormat() error

	UpgradedFrom() int
	BackupName() string
}

// Lib is one adapter's package-level calls, for code that picks a shcl
// version at run time.
type Lib struct {
	Name        string
	FormatMajor int
	New         func() Doc
	Parse       func(text string) Doc
	ParseWith   func(text string, s Strictness) (Doc, error)
	OpenWith    func(path string, s Strictness) (Doc, error)
	OpenLayers  func(layers ...Layer) (Doc, error)
	FormatOf    func(text string) int
	Upgrade     func(text string, from int) (Upgraded, error)
}

// ValueKind is the type of value Put was handed, after Normalize.
type ValueKind int

const (
	KindInt ValueKind = iota + 1
	KindFloat
	KindBool
	KindStr
	KindStrs
	KindInts
	KindFloats
)

// Value is a Put argument with its Go type folded down to the few shcl takes.
type Value struct {
	Kind   ValueKind
	Int    int64
	Float  float64
	Bool   bool
	Str    string
	Strs   []string
	Ints   []int64
	Floats []float64
}

// Normalize folds v into a Value, or fails with ErrBadValue for a type
// shcl has no setter for.
func Normalize(path string, v any) (Value, error) {
	switch x := v.(type) {
	case int:
		return Value{Kind: KindInt, Int: int64(x)}, nil
	case int32:
		return Value{Kind: KindInt, Int: int64(x)}, nil
	case int64:
		return Value{Kind: KindInt, Int: x}, nil
	case uint32:
		return Value{Kind: KindInt, Int: int64(x)}, nil
	case float32:
		// float64(x) would write the float32's binary noise: 0.9 as 0.8999999761581421.
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return finite(path, Value{Kind: KindFloat, Float: f})
	case float64:
		return finite(path, Value{Kind: KindFloat, Float: x})
	case bool:
		return Value{Kind: KindBool, Bool: x}, nil
	case string:
		return Value{Kind: KindStr, Str: x}, nil
	case []string:
		return Value{Kind: KindStrs, Strs: x}, nil
	case []int64:
		return Value{Kind: KindInts, Ints: x}, nil
	case []int:
		ints := make([]int64, len(x))
		for i, n := range x {
			ints[i] = int64(n)
		}
		return Value{Kind: KindInts, Ints: ints}, nil
	case []float64:
		return finite(path, Value{Kind: KindFloats, Floats: x})
	}
	return Value{}, &Error{Kind: ErrBadValue, Path: path, Msg: fmt.Sprintf("no setter for %T", v)}
}

// finite refuses infinity and NaN, since no read takes them back. See
// "What the wrapper evens out" in design.md.
func finite(path string, v Value) (Value, error) {
	if notFinite(v.Float) || slices.ContainsFunc(v.Floats, notFinite) {
		return Value{}, &Error{Kind: ErrBadValue, Path: path, Msg: "not a finite number"}
	}
	return v, nil
}

// FiniteFloat makes an infinity or NaN read BadType, as 3.0 reads 1e400.
func FiniteFloat(g Got[float64]) Got[float64] {
	if g.Status == Good && notFinite(g.Value) {
		g.Status, g.Value = BadType, 0
	}
	return g
}

// FiniteFloats is FiniteFloat for an array. Like 3.0, the array keeps its
// length, with 0 for each item that wasn't a number.
func FiniteFloats(g Got[[]float64]) Got[[]float64] {
	if g.Status != Good || !slices.ContainsFunc(g.Value, notFinite) {
		return g
	}
	g.Value = slices.Clone(g.Value)
	for i, f := range g.Value {
		if notFinite(f) {
			g.Value[i] = 0
		}
	}
	g.Status = BadType
	return g
}

func notFinite(f float64) bool { return math.IsInf(f, 0) || math.IsNaN(f) }

// VerifyPut reads back what a setter wrote, since shcl can take a value and
// read back something else. See "What the wrapper evens out" in design.md.
func VerifyPut(d Doc, path string, v Value) error {
	var st Status
	same := false
	switch v.Kind {
	case KindInt:
		g := d.GetInt(path)
		st, same = g.Status, g.Value == v.Int
	case KindFloat:
		g := d.GetFloat(path)
		st, same = g.Status, g.Value == v.Float
	case KindBool:
		g := d.GetBool(path)
		st, same = g.Status, g.Value == v.Bool
	case KindStr:
		g := d.GetStr(path)
		st, same = g.Status, g.Value == v.Str
	case KindStrs:
		g := d.GetStrs(path)
		st, same = g.Status, slices.Equal(g.Value, v.Strs)
		if st == Empty && len(v.Strs) == 0 {
			return nil
		}
	case KindInts:
		g := d.GetInts(path)
		st, same = g.Status, slices.Equal(g.Value, v.Ints)
		if st == Empty && len(v.Ints) == 0 {
			return nil
		}
	case KindFloats:
		g := d.GetFloats(path)
		st, same = g.Status, slices.Equal(g.Value, v.Floats)
		if st == Empty && len(v.Floats) == 0 {
			return nil
		}
	}
	if st == Good && same {
		return nil
	}
	msg := "reads back as " + st.String()
	if st == Good {
		msg = "reads back as a different value"
	}
	return &Error{Kind: ErrNoReadBack, Path: path, Msg: msg}
}

// VerifySyntax reads back what PutSyntax set at path. alone is a new
// document with the same text set at "k", and line is "k: text" parsed.
// shcl drops a trailing comment from the text, so the two have to match. See
// "What the wrapper evens out" in design.md.
func VerifySyntax(d Doc, path string, alone, line Doc) error {
	if alone.Canonical() != line.Canonical() {
		return &Error{Kind: ErrNoReadBack, Path: path, Msg: "the text doesn't read back as typed"}
	}
	got, want := d.GetStr(path), line.GetStr("k")
	if got.Status != want.Status || got.Value != want.Value {
		return &Error{Kind: ErrNoReadBack, Path: path, Msg: "reads back as a different value"}
	}
	return nil
}

// DedupeNames keeps the first of each name, compared without ASCII case,
// since shcl matches names that way and lists one per line it was given on.
func DedupeNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		k := asciiLower(n)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, n)
	}
	return out
}

// asciiLower folds only A to Z. strings.ToLower would also fold É into é,
// which shcl holds as 2 names, and turn bad UTF-8 into U+FFFD.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// InFileOrder is the instances of path sorted by line. Each goes in tree
// order, which for a reopened block isn't the order of the file. An
// instance a put made has no line, and goes after the rest in Each order.
func InFileOrder(d Doc, path string) []Node {
	nodes := d.Each(path)
	key := func(n Node) int {
		if n.Line == 0 {
			return math.MaxInt
		}
		return n.Line
	}
	slices.SortStableFunc(nodes, func(a, b Node) int { return cmp.Compare(key(a), key(b)) })
	return nodes
}

// FirstOf is path narrowed to its first instance in the file, or path as it
// is when it names none. One instance still gets its Each path, which an
// edit takes where an inner step of path repeats.
func FirstOf(d Doc, path string) string {
	if nodes := InFileOrder(d, path); len(nodes) > 0 {
		return nodes[0].Path
	}
	return path
}

// LastOf is FirstOf for the last instance.
func LastOf(d Doc, path string) string {
	if nodes := InFileOrder(d, path); len(nodes) > 0 {
		return nodes[len(nodes)-1].Path
	}
	return path
}

// LinesOf is the line of each instance of path, in file order.
func LinesOf(d Doc, path string) []int {
	nodes := InFileOrder(d, path)
	out := make([]int, len(nodes))
	for i, n := range nodes {
		out[i] = n.Line
	}
	return out
}

// ReadText reads a whole file, with a missing file as ErrNotFound. A file
// that isn't valid UTF-8 is ErrIO, since no shcl version reads one right.
func ReadText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if !utf8.Valid(b) {
			return "", &Error{Kind: ErrIO, Path: path, Msg: "not valid UTF-8"}
		}
		return string(b), nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "", &Error{Kind: ErrNotFound, Path: path, Err: err}
	}
	return "", &Error{Kind: ErrIO, Path: path, Err: err}
}

// Changed is the error for a file that changed on disk since it was read.
func Changed(path string) error {
	return &Error{Kind: ErrChanged, Path: path, Msg: "changed since it was read; nothing written"}
}

// Unchanged is whether path still holds was, byte for byte.
func Unchanged(path, was string) bool {
	b, err := os.ReadFile(path)
	return err == nil && string(b) == was
}

// ReplaceRead writes data over path, where was is the text last read from
// it. A file that has changed since is left alone, checked before the
// backup and again before the write. backup, when not nil, keeps was first
// and gives the copy's name, which ReplaceRead returns. When the write
// fails, the original stays or is put back from the backup, and then the
// backup goes. Only when neither works is the backup kept, and the error
// names it.
func ReplaceRead(path, was, data string, backup func() (string, error), write func(path, data string) error) (string, error) {
	changed := func() error {
		return Changed(path)
	}
	if !Unchanged(path, was) {
		return "", changed()
	}
	name := ""
	if backup != nil {
		var err error
		if name, err = backup(); err != nil {
			return "", err
		}
	}
	drop := func() {
		if name != "" {
			os.Remove(name) // a copy of what's still at path
		}
	}
	if !Unchanged(path, was) {
		drop()
		return "", changed()
	}
	err := write(path, data)
	if err == nil {
		return name, nil
	}
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Kind: ErrIO, Path: path, Err: err}
	}
	if Unchanged(path, was) {
		drop()
		return "", e
	}
	if name == "" {
		return "", e
	}
	// A replace that fails part way on Windows can leave nothing at path.
	note := "the original is " + name
	if _, serr := os.Lstat(path); errors.Is(serr, fs.ErrNotExist) && os.Rename(name, path) == nil {
		note = "the original was put back"
	}
	if e.Msg != "" {
		note = e.Msg + "; " + note
	}
	e.Msg = note
	return "", e
}

// FormatFloat writes a float the same way in every wrapper language: the
// shortest digits that read back exactly, positional between 1e-5 and
// 1e17, else with an exponent.
func FormatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expText, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expText)
	neg := strings.HasPrefix(mant, "-")
	digits := strings.Replace(strings.TrimPrefix(mant, "-"), ".", "", 1)
	return assembleFloat(neg, digits, exp)
}

func assembleFloat(neg bool, digits string, exp int) string {
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	switch {
	case exp >= 17 || exp < -5:
		b.WriteString(digits[:1])
		if len(digits) > 1 {
			b.WriteByte('.')
			b.WriteString(digits[1:])
		}
		b.WriteString("e")
		b.WriteString(strconv.Itoa(exp))
	case exp < 0:
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", -exp-1))
		b.WriteString(digits)
	case len(digits) <= exp+1:
		b.WriteString(digits)
		b.WriteString(strings.Repeat("0", exp+1-len(digits)))
		b.WriteString(".0")
	default:
		b.WriteString(digits[:exp+1])
		b.WriteByte('.')
		b.WriteString(digits[exp+1:])
	}
	return b.String()
}

// UnusedOf is the lines with an error in ds, once each, by layer then line.
// An error on no line, such as one about the whole file, has no line to give.
func UnusedOf(ds []Diag) []Spot {
	out := []Spot{}
	for _, g := range ds {
		if g.Severity == SevError && g.Line > 0 {
			out = append(out, Spot{Line: g.Line, Layer: g.Layer})
		}
	}
	slices.SortFunc(out, func(a, b Spot) int {
		if a.Layer != b.Layer {
			return cmp.Compare(a.Layer, b.Layer)
		}
		return cmp.Compare(a.Line, b.Line)
	})
	return slices.Compact(out)
}

// MergesOf finds the copies shcl folded into an earlier one in text, apart
// or not (shcl's H002 only says so when they're apart). A folded copy's line
// is on no node, so each such line gets its name made unique and the text
// is parsed again, where the copy then stands alone (design.md, Decisions).
func MergesOf(text string, parse func(string) Doc, layer int) []Merge {
	d := parse(text)
	at := map[int]string{}
	var walk func(Doc, string, func(string, string, Node) bool)
	walk = func(x Doc, path string, see func(string, string, Node) bool) {
		for _, name := range x.Names(path) {
			for _, n := range x.Each(x.Child(path, name)) {
				if see(path, name, n) {
					walk(x, n.Path, see)
				}
			}
		}
	}
	walk(d, "", func(_, _ string, n Node) bool {
		if n.Line > 0 {
			at[n.Line] = n.Path
		}
		return true
	})
	bad := map[int]bool{}
	for _, g := range d.Diags() {
		if g.Severity == SevError {
			bad[g.Line] = true
		}
	}
	lines := strings.Split(text, "\n")
	probe := slices.Clone(lines)
	folded := map[int]bool{}
	for i, l := range lines {
		n := i + 1
		if _, ok := at[n]; ok || bad[n] {
			continue
		}
		body := strings.TrimLeft(l, " \t")
		if body == "" {
			continue
		}
		indent, c := l[:len(l)-len(body)], body[0]
		switch {
		case c == '"' || c == '\'':
			probe[i] = indent + body[:1] + probeName(n) + body[1:]
		case c >= 0x80 || 'a' <= c|0x20 && c|0x20 <= 'z':
			probe[i] = indent + probeName(n) + body
		default:
			continue
		}
		folded[n] = true
	}
	if len(folded) == 0 {
		return []Merge{}
	}
	p := parse(strings.Join(probe, "\n"))
	out := []Merge{}
	slot := map[string]int{}
	// orig is the path in d of the node a probe node stands for.
	orig := map[string]string{"": ""}
	walk(p, "", func(parent, name string, n Node) bool {
		if !folded[n.Line] {
			o, ok := at[n.Line]
			orig[n.Path] = o
			return ok
		}
		up, ok := orig[parent]
		if !ok {
			return false
		}
		if pre := probeName(n.Line); strings.HasPrefix(asciiLower(name), pre) {
			name = name[len(pre):]
		}
		var kept Node
		for _, k := range d.Each(d.Child(up, name)) {
			if k.Value == n.Value && k.Line < n.Line && k.Line > kept.Line {
				kept = k
			}
		}
		if kept.Line == 0 {
			return false
		}
		orig[n.Path] = kept.Path
		// A dotted line folds each step; only its last one is the copy.
		for _, c := range p.Names(n.Path) {
			for _, k := range p.Each(p.Child(n.Path, c)) {
				if k.Line == n.Line {
					return true
				}
			}
		}
		i, ok := slot[kept.Path]
		if !ok {
			i = len(out)
			slot[kept.Path] = i
			out = append(out, Merge{Path: kept.Path, Lines: []int{kept.Line}, Layer: layer})
		}
		out[i].Lines = append(out[i].Lines, n.Line)
		return true
	})
	for i := range out {
		slices.Sort(out[i].Lines)
	}
	slices.SortFunc(out, func(a, b Merge) int {
		if a.Lines[0] != b.Lines[0] {
			return cmp.Compare(a.Lines[0], b.Lines[0])
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

func probeName(line int) string { return "shclwprobe" + strconv.Itoa(line) + "x" }
