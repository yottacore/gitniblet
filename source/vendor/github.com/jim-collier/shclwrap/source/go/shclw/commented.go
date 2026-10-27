package shclw

import (
	"strings"
)

// Commented is a comment line that reads as one `key: value` once its `#`
// comes off, such as a setting left in the file at its default. Path is
// what the line would set. Default says the line ends in a `## Default`
// note, which is the line a Put of Path fills in.
type Commented struct {
	Path    string
	Value   string
	Line    int
	Default bool
}

type seen struct {
	name string
	line int
}

// CommentedOf lists the commented settings in text. Each candidate line is
// uncommented alone and the text parsed again, so the path is shcl's own
// reading of where the line sits. A line is left out when that adds an
// error or changes any other field, such as one that would open a raw
// block or take the lines under it as children.
func CommentedOf(text string, parse func(string) Doc) []Commented {
	return commentedOf(text, parse, func(string, string) bool { return true })
}

// DefaultLine is the line of path's commented `## Default` setting in text,
// or 0. Only the lines whose key path ends with are parsed.
func DefaultLine(text, path string, parse func(string) Doc) int {
	keep := func(l, body string) bool { return hasDefaultNote(l) && keyEnds(path, body) }
	for _, c := range commentedOf(text, parse, keep) {
		if asciiLower(c.Path) == asciiLower(path) {
			return c.Line
		}
	}
	return 0
}

// keyEnds is a quick test, before the parse that says for sure, of whether
// path ends with the key body starts with.
func keyEnds(path, body string) bool {
	end := strings.IndexByte(body, ':')
	if q := body[0]; q == '"' || q == '\'' {
		if c := strings.IndexByte(body[1:], q); c >= 0 {
			if k := strings.IndexByte(body[c+2:], ':'); k >= 0 {
				end = c + 2 + k
			}
		}
	}
	key := asciiLower(strings.TrimRight(body[:end], " \t"))
	p := asciiLower(path)
	return p == key || strings.HasSuffix(p, "."+key)
}

func commentedOf(text string, parse func(string) Doc, keep func(line, body string) bool) []Commented {
	out := []Commented{}
	lines := strings.Split(text, "\n")
	var cands []int
	for i, l := range lines {
		if u, ok := uncomment(l, false); ok && keep(l, strings.TrimLeft(u, " \t")) {
			cands = append(cands, i)
		}
	}
	if len(cands) == 0 {
		return out
	}
	d := parse(text)
	base := fieldsOf(d, -1)
	errs := d.Errors()
	probe := make([]string, len(lines))
	for _, i := range cands {
		copy(probe, lines)
		probe[i], _ = uncomment(lines[i], false)
		p := parse(strings.Join(probe, "\n"))
		if p.Errors() != errs {
			continue
		}
		n := i + 1
		if !sameFields(base, fieldsOf(p, n)) {
			continue
		}
		if c, ok := leafAt(p, n); ok {
			c.Default = hasDefaultNote(lines[i])
			out = append(out, c)
		}
	}
	return out
}

// Uncommented is text with line n's `#` taken off, and its `## Default`
// note too, as a fill-in writes it.
func Uncommented(text string, n int) string {
	lines := strings.Split(text, "\n")
	lines[n-1], _ = uncomment(lines[n-1], true)
	return strings.Join(lines, "\n")
}

// uncomment is l without its `#`, for a line with one `#` and then text
// with a colon in it. A `##` line is a note, not a setting.
func uncomment(l string, dropNote bool) (string, bool) {
	body := strings.TrimLeft(l, " \t")
	indent := l[:len(l)-len(body)]
	if !strings.HasPrefix(body, "#") || strings.HasPrefix(body, "##") {
		return l, false
	}
	body = strings.TrimLeft(body[1:], " \t")
	if body == "" || body[0] == '#' || !strings.Contains(body, ":") {
		return l, false
	}
	if dropNote {
		if at, ok := defaultNote(body); ok {
			eol := ""
			if strings.HasSuffix(body, "\r") {
				eol = "\r"
			}
			body = strings.TrimRight(body[:at], " \t") + eol
		}
	}
	return indent + body, true
}

func hasDefaultNote(l string) bool {
	_, ok := defaultNote(l)
	return ok
}

// defaultNote is where a trailing `## Default` note starts in l.
func defaultNote(l string) (int, bool) {
	t := strings.TrimRight(l, " \t\r")
	const note = "default"
	if len(t) < len(note) || asciiLower(t[len(t)-len(note):]) != note {
		return 0, false
	}
	t = strings.TrimRight(t[:len(t)-len(note)], " \t")
	if !strings.HasSuffix(t, "##") {
		return 0, false
	}
	at := len(t) - 2
	if at == 0 || (t[at-1] != ' ' && t[at-1] != '\t') {
		return 0, false
	}
	return at, true
}

// fieldsOf is every field's name and line in shcl's order, less the ones
// on line skip.
func fieldsOf(d Doc, skip int) []seen {
	var out []seen
	var walk func(path string)
	walk = func(path string) {
		for _, name := range d.Names(path) {
			for _, x := range d.Each(d.Child(path, name)) {
				if x.Line != skip {
					out = append(out, seen{asciiLower(name), x.Line})
				}
				walk(x.Path)
			}
		}
	}
	walk("")
	return out
}

func sameFields(a, b []seen) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// leafAt is the one field on line n with nothing under it, with its path as
// a program would write it: no index on the last step, which a key already
// set elsewhere in the block gets once the line comes back.
func leafAt(d Doc, n int) (Commented, bool) {
	var found []Commented
	var walk func(path string)
	walk = func(path string) {
		for _, name := range d.Names(path) {
			for _, x := range d.Each(d.Child(path, name)) {
				if x.Line == n && len(d.Names(x.Path)) == 0 {
					found = append(found, Commented{Path: d.Child(path, name), Value: x.Value, Line: n})
				}
				walk(x.Path)
			}
		}
	}
	walk("")
	if len(found) != 1 {
		return Commented{}, false
	}
	return found[0], true
}
