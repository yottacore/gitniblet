// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
// SPDX-License-Identifier: MIT

package shclw

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RunOptions says what Run may touch outside the document.
type RunOptions struct {
	// Dir is where `check` reads schema files and `write` writes. Empty
	// turns both ops off.
	Dir string
}

// Run parses input with lib, applies a script of ops to it, and returns
// what the ops printed. The script and its output are the same in every
// wrapper language, which is what the cross-language tests compare. The
// ops are listed in project/design.md under "Ops scripts".
func Run(lib Lib, input, script string, opt RunOptions) string {
	r := runner{lib: lib, input: input, opt: opt, doc: lib.Parse(input)}
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r.out("> " + line)
		r.op(splitFields(line))
	}
	return r.b.String()
}

type runner struct {
	lib   Lib
	input string
	opt   RunOptions
	doc   Doc
	b     strings.Builder
}

func (r *runner) out(s string) {
	r.b.WriteString(s)
	r.b.WriteByte('\n')
}

func (r *runner) op(f []string) {
	arg := func(i int) string {
		if i < len(f) {
			return f[i]
		}
		return ""
	}
	rest := func(i int) []string {
		if i < len(f) {
			return f[i:]
		}
		return []string{}
	}
	d := r.doc
	switch f[0] {
	case "parse":
		r.parse(arg(1))
	case "text":
		r.block(d.Text())
	case "canonical":
		r.block(d.Canonical())
	case "rewrites":
		r.out(strconv.FormatBool(d.Rewrites()))
	case "diags":
		r.diags(d.Diags())
	case "errors":
		r.out(strconv.Itoa(d.Errors()))
	case "unused":
		spots := d.Unused()
		out := make([]string, len(spots))
		for i, sp := range spots {
			out[i] = spotText(sp.Line, sp.Layer)
		}
		r.out(counted(len(out), strings.Join(out, "\t")))
	case "merged":
		for _, m := range d.Merged() {
			out := []string{esc(m.Path)}
			for _, n := range m.Lines {
				out = append(out, spotText(n, m.Layer))
			}
			r.out(strings.Join(out, "\t"))
		}
	case "commented":
		for _, c := range d.Commented() {
			def := ""
			if c.Default {
				def = "\tdefault"
			}
			r.out(strconv.Itoa(c.Line) + "\t" + esc(c.Path) + "\t" + esc(c.Value) + def)
		}
	case "get":
		r.out(r.get(arg(1), arg(2)))
	case "or":
		r.or(arg(1), arg(2), rest(3))
	case "has":
		r.out(strconv.FormatBool(d.Has(arg(1))))
	case "count":
		r.out(strconv.Itoa(d.Count(arg(1))))
	case "line":
		r.out(strconv.Itoa(d.Line(arg(1))))
	case "lines":
		lines := d.Lines(arg(1))
		r.out(counted(len(lines), joinLines(lines)))
	case "first":
		r.out(r.get(arg(1), d.First(arg(2))))
	case "last":
		r.out(r.get(arg(1), d.Last(arg(2))))
	case "names":
		r.out(joinEsc(d.Names(arg(1))))
	case "each":
		for i, n := range d.Each(arg(1)) {
			r.out(fmt.Sprintf("%d\t%d\t%s", i, n.Line, esc(n.Value)))
		}
	case "eachget":
		for i, n := range d.Each(arg(2)) {
			r.out(strconv.Itoa(i) + "\t" + r.get(arg(1), d.Child(n.Path, arg(3))))
		}
	case "put":
		v, ok := parseValue(arg(1), rest(3))
		if !ok {
			r.out("error badvalue")
			return
		}
		r.result(d.Put(arg(2), v))
	case "syntax":
		r.result(d.PutSyntax(arg(1), arg(2)))
	case "comment":
		r.result(d.Comment(arg(1), arg(2)))
	case "remove":
		r.out(strconv.Itoa(d.Remove(arg(1))))
	case "info":
		if arg(1) != "on" && arg(1) != "off" {
			r.out("error badvalue")
			return
		}
		r.result(d.InfoBlock(arg(1) == "on"))
	case "stamp":
		r.result(d.StampFormat())
	case "check":
		r.check(arg(1), arg(2))
	case "format":
		r.out(strconv.Itoa(r.lib.FormatOf(r.input)))
	case "upgrade":
		r.upgrade(arg(1))
	case "write":
		r.write(arg(1), rest(2))
	case "readonly":
		r.out(strconv.FormatBool(d.ReadOnly()))
	case "scribble":
		r.scribble(arg(1), arg(2))
	case "mode":
		r.mode(arg(1))
	case "open":
		r.open(arg(1), arg(2))
	case "layers":
		r.layers(rest(1))
	case "upgraded":
		r.upgraded()
	case "files":
		r.files()
	default:
		r.out("error unknownop")
	}
}

func strictnessOf(level string) Strictness {
	switch level {
	case "loose":
		return Loose
	case "strict":
		return Strict
	}
	return Standard
}

func (r *runner) parse(level string) {
	doc, err := r.lib.ParseWith(r.input, strictnessOf(level))
	r.doc = doc
	r.result(err)
}

// open prints an error's diagnostics after it, and goes on with an empty
// document, as C has no list on its error.
func (r *runner) open(name, level string) {
	path, ok := r.file(name)
	if !ok {
		return
	}
	doc, err := r.lib.OpenWith(path, strictnessOf(level))
	if err != nil {
		r.result(err)
		var e *Error
		if errors.As(err, &e) {
			r.diags(e.Diags)
		}
		r.doc = r.lib.New()
		return
	}
	r.doc = doc
	r.out("ok")
}

// layers takes file names, and text for a text layer after a "=".
func (r *runner) layers(specs []string) {
	layers := make([]Layer, len(specs))
	for i, spec := range specs {
		if text, ok := strings.CutPrefix(spec, "="); ok {
			layers[i].Text = text
			continue
		}
		path, ok := r.file(spec)
		if !ok {
			return
		}
		layers[i].File = path
	}
	doc, err := r.lib.OpenLayers(layers...)
	r.doc = doc
	r.result(err)
	var e *Error
	if errors.As(err, &e) {
		r.diags(e.Diags)
	}
}

func (r *runner) upgraded() {
	from, name := r.doc.UpgradedFrom(), r.doc.BackupName()
	if name == "" {
		r.out(fmt.Sprintf("from %d backup none", from))
		return
	}
	r.out(fmt.Sprintf("from %d backup %s", from, esc(maskStamp(filepath.Base(name)))))
	text, err := ReadText(name)
	if err != nil {
		r.result(err)
		return
	}
	r.block(text)
}

func (r *runner) files() {
	if r.opt.Dir == "" {
		r.out("error unsupported")
		return
	}
	ents, err := os.ReadDir(r.opt.Dir)
	if err != nil {
		r.result(&Error{Kind: ErrIO, Err: err})
		return
	}
	for _, e := range ents {
		r.out(esc(maskStamp(e.Name())))
	}
}

// maskStamp puts STAMP in place of the time in a backup's name.
func maskStamp(name string) string {
	at := strings.Index(name, "_backup_")
	if at < 0 {
		return name
	}
	start := at + len("_backup_")
	end := strings.Index(name[start:], "_format-v")
	if end < 0 {
		return name
	}
	return name[:start] + "STAMP" + name[start+end:]
}

func (r *runner) result(err error) {
	if err == nil {
		r.out("ok")
		return
	}
	r.out("error " + KindOf(err).String())
}

func (r *runner) diags(ds []Diag) {
	for _, d := range ds {
		r.out(fmt.Sprintf("%s\t%s\t%s\t%s", spotText(d.Line, d.Layer), d.Severity, d.Code, esc(d.Message)))
	}
}

// spotText is a line, with its layer in front on a layered document.
func spotText(line, layer int) string {
	if layer > 0 {
		return strconv.Itoa(layer) + ":" + strconv.Itoa(line)
	}
	return strconv.Itoa(line)
}

// block prints multi-line text one line per output line, so a golden file
// diff shows which line changed.
func (r *runner) block(text string) {
	lines := strings.Split(text, "\n")
	last := len(lines) - 1
	if lines[last] == "" {
		lines = lines[:last]
	}
	for _, l := range lines {
		r.out("| " + esc(l))
	}
	if !strings.HasSuffix(text, "\n") && text != "" {
		r.out("(no newline at end)")
	}
}

func (r *runner) get(typ, path string) string {
	d := r.doc
	head := func(s Status, line int) string { return s.String() + "\t" + strconv.Itoa(line) }
	switch typ {
	case "int":
		g := d.GetInt(path)
		return head(g.Status, g.Line) + "\t" + strconv.FormatInt(g.Value, 10)
	case "float":
		g := d.GetFloat(path)
		return head(g.Status, g.Line) + "\t" + FormatFloat(g.Value)
	case "bool":
		g := d.GetBool(path)
		return head(g.Status, g.Line) + "\t" + strconv.FormatBool(g.Value)
	case "str":
		g := d.GetStr(path)
		return head(g.Status, g.Line) + "\t" + esc(g.Value)
	case "raw":
		g := d.GetRaw(path)
		return head(g.Status, g.Line) + "\t" + esc(g.Value)
	case "strs":
		g := d.GetStrs(path)
		return head(g.Status, g.Line) + "\t" + counted(len(g.Value), joinEsc(g.Value))
	case "ints":
		g := d.GetInts(path)
		return head(g.Status, g.Line) + "\t" + counted(len(g.Value), joinInts(g.Value))
	case "floats":
		g := d.GetFloats(path)
		return head(g.Status, g.Line) + "\t" + counted(len(g.Value), joinFloats(g.Value))
	}
	return "error badtype"
}

func (r *runner) or(typ, path string, fallback []string) {
	d := r.doc
	first := ""
	if len(fallback) > 0 {
		first = fallback[0]
	}
	switch typ {
	case "int":
		n, err := strconv.ParseInt(first, 10, 64)
		if err != nil {
			r.out("error badvalue")
			return
		}
		r.out(strconv.FormatInt(d.Int(path, n), 10))
	case "float":
		x, ok := parseFloat(first)
		if !ok {
			r.out("error badvalue")
			return
		}
		r.out(FormatFloat(d.Float(path, x)))
	case "bool":
		r.out(strconv.FormatBool(d.Bool(path, first == "true")))
	case "str":
		r.out(esc(d.Str(path, first)))
	case "raw":
		r.out(esc(d.Raw(path, first)))
	case "strs":
		v := d.Strs(path, fallback)
		r.out(counted(len(v), joinEsc(v)))
	case "ints":
		v, ok := parseValue("ints", fallback)
		if !ok {
			r.out("error badvalue")
			return
		}
		n := d.Ints(path, v.([]int64))
		r.out(counted(len(n), joinInts(n)))
	case "floats":
		v, ok := parseValue("floats", fallback)
		if !ok {
			r.out("error badvalue")
			return
		}
		x := d.Floats(path, v.([]float64))
		r.out(counted(len(x), joinFloats(x)))
	default:
		r.out("error badtype")
	}
}

// check prints a schema check. Mode "all" adds the load's own diagnostics
// first, and "drop" does that and then drops what the schema declares.
func (r *runner) check(name, mode string) {
	path, ok := r.file(name)
	if !ok {
		return
	}
	text, err := ReadText(path)
	if err != nil {
		r.result(err)
		return
	}
	schema := r.lib.Parse(text)
	diags := r.doc.Check(schema)
	if mode == "all" || mode == "drop" {
		diags = append(r.doc.Diags(), diags...)
	}
	if mode == "drop" {
		diags = schema.DropDeclared(diags)
	}
	r.diags(diags)
}

func (r *runner) upgrade(from string) {
	n, err := strconv.Atoi(from)
	if from == "" {
		n, err = 0, nil
	}
	if err != nil {
		r.out("error badvalue")
		return
	}
	u, err := r.lib.Upgrade(r.input, n)
	if err != nil {
		r.result(err)
		return
	}
	r.out(fmt.Sprintf("from %d changed %t unsure %d", u.From, u.Changed, u.Unsure))
	r.out(fmt.Sprintf("lost %v rewritten %v", u.Lost, u.Rewritten))
	r.block(u.Text)
}

func (r *runner) write(name string, flags []string) {
	path, ok := r.file(name)
	if !ok {
		return
	}
	var o WriteOpts
	for _, flag := range flags {
		switch {
		case flag == "force":
			o.Force = true
		case flag == "create":
			o.Create = true
		case flag == "backup":
			o.Backup = true
		case strings.HasPrefix(flag, "mode="):
			m, err := strconv.ParseUint(strings.TrimPrefix(flag, "mode="), 8, 32)
			if err != nil {
				r.out("error badvalue")
				return
			}
			o.Mode = fs.FileMode(m)
		default:
			r.out("error badvalue")
			return
		}
	}
	if err := r.doc.WriteWith(path, o); err != nil {
		r.result(err)
		return
	}
	text, err := ReadText(path)
	if err != nil {
		r.result(err)
		return
	}
	r.out("ok")
	r.block(text)
}

// mode prints a file's permission bits.
func (r *runner) mode(name string) {
	path, ok := r.file(name)
	if !ok {
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		r.result(&Error{Kind: ErrIO, Path: path, Err: err})
		return
	}
	r.out(fmt.Sprintf("%04o", uint32(st.Mode().Perm())))
}

// scribble writes text to a file as another program would, behind the
// document's back.
func (r *runner) scribble(name, text string) {
	path, ok := r.file(name)
	if !ok {
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		r.result(&Error{Kind: ErrIO, Path: path, Err: err})
		return
	}
	r.out("ok")
}

// file keeps a script's file names inside RunOptions.Dir.
func (r *runner) file(name string) (string, bool) {
	if r.opt.Dir == "" || name == "" || name != filepath.Base(name) || name == ".." {
		r.out("error unsupported")
		return "", false
	}
	return filepath.Join(r.opt.Dir, name), true
}

func parseValue(typ string, f []string) (any, bool) {
	first := ""
	if len(f) > 0 {
		first = f[0]
	}
	switch typ {
	case "int":
		n, err := strconv.ParseInt(first, 10, 64)
		return n, err == nil
	case "float":
		return parseFloat(first)
	case "bool":
		return first == "true", first == "true" || first == "false"
	case "str":
		return first, true
	case "strs":
		return append([]string{}, f...), true
	case "ints":
		out := make([]int64, 0, len(f))
		for _, s := range f {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, false
			}
			out = append(out, n)
		}
		return out, true
	case "floats":
		out := make([]float64, 0, len(f))
		for _, s := range f {
			x, ok := parseFloat(s)
			if !ok {
				return nil, false
			}
			out = append(out, x)
		}
		return out, true
	}
	return nil, false
}

// parseFloat takes plain decimal and exponent forms, plus nan, inf, and
// -inf as FormatFloat writes them. Each language's own parser takes a
// different set of spellings for those. A number too big is infinity, as
// Rust reads it.
func parseFloat(s string) (float64, bool) {
	switch s {
	case "nan":
		return math.NaN(), true
	case "inf":
		return math.Inf(1), true
	case "-inf":
		return math.Inf(-1), true
	}
	if s == "" || strings.ContainsAny(s, "xXpP_") || strings.ContainsFunc(s, isLetterNotE) {
		return 0, false
	}
	x, err := strconv.ParseFloat(s, 64)
	return x, err == nil || errors.Is(err, strconv.ErrRange)
}

func isLetterNotE(r rune) bool {
	return (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') && r != 'e' && r != 'E'
}

// splitFields splits a script line on tabs and undoes \t, \n, \r, and \\.
func splitFields(line string) []string {
	f := strings.Split(line, "\t")
	for i, s := range f {
		f[i] = unesc(s)
	}
	return f
}

func unesc(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i+1])
		}
		i++
	}
	return b.String()
}

// esc makes a value safe for one tab-separated output field. Bytes that
// aren't valid UTF-8 print as \xHH.
func esc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case c == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// counted puts an array's length before its items, so [] and [""] print
// differently.
func counted(n int, items string) string {
	if n == 0 {
		return "0"
	}
	return strconv.Itoa(n) + "\t" + items
}

func joinEsc(v []string) string {
	out := make([]string, len(v))
	for i, s := range v {
		out[i] = esc(s)
	}
	return strings.Join(out, "\t")
}

func joinInts(v []int64) string {
	out := make([]string, len(v))
	for i, n := range v {
		out[i] = strconv.FormatInt(n, 10)
	}
	return strings.Join(out, "\t")
}

func joinLines(v []int) string {
	out := make([]string, len(v))
	for i, n := range v {
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, "\t")
}

func joinFloats(v []float64) string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = FormatFloat(x)
	}
	return strings.Join(out, "\t")
}
