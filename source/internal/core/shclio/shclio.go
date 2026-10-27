// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package shclio is where gitniblet picks its shcl version. Every schema,
// queries, registry, settings and login file is read and written through it,
// so a file from before shcl 3 comes forward as it is read.
package shclio

import (
	"fmt"
	"strings"

	"github.com/jim-collier/shclwrap/source/go/shclw"
	shcl "github.com/jim-collier/shclwrap/source/go/shclw3"
)

// Doc is a parsed file.
type Doc = shcl.Doc

// Diag is one load or validation diagnostic.
type Diag = shcl.Diag

// The strictness levels a load takes.
const (
	Loose    = shcl.Loose
	Standard = shcl.Standard
	Strict   = shcl.Strict
)

// IsError is true for a diagnostic that cost the load a line.
func IsError(d Diag) bool { return d.Severity == shclw.SevError }

// New is an empty document.
func New() *Doc { return shcl.New() }

// Load parses text at strictness s, after Current. Only Strict can fail.
func Load(text string, s shcl.Strictness, fixes ...LineFix) (*Doc, error) {
	return shcl.ParseWith(Current(text, fixes...), s)
}

// LineFix is a line of a file from before shcl 3 as 3 should read it, for a
// line the upgrade's 2.x reading gets wrong for this program. ok is false
// for a line it leaves to the upgrade.
type LineFix func(line string) (fixed string, ok bool)

// Current is text as shcl 3 should read it. A file with no Format line that
// doesn't load clean on 3, such as one with a bare `a, b` list, is from
// before 3 and is upgraded. One the upgrade refuses comes back as it is, so
// the load names its bad lines.
//
// The upgrade reads the file the 2.x way. A line fixes knows, or one with a
// backtick value, skips it and reads the way gitniblet always read it
// (2026102509204445). A file that reads two ways is taken as 2.x, since
// every file without a Format line was written for the 2.x rules.
func Current(text string, fixes ...LineFix) string {
	if shcl.FormatOf(text) >= shcl.FormatMajor {
		return text
	}
	fixes = append(fixes, keepBacktick)
	lines := strings.Split(text, "\n")
	kept := map[string]string{}
	for i, line := range lines {
		for _, fix := range fixes {
			if fixed, ok := fix(line); ok {
				// A bare word reads the same both ways, so the upgrade leaves it be.
				token := fmt.Sprintf("gnkept%dx", i)
				key, _, _ := strings.Cut(line, ":")
				kept[token], lines[i] = fixed, key+": "+token
				break
			}
		}
	}
	held := strings.Join(lines, "\n")
	u, err := shcl.Upgrade(held, 0)
	if shcl.KindOf(err) == shclw.ErrAmbiguous {
		u, err = shcl.Upgrade(held, 2)
	}
	switch {
	case err != nil:
		return text
	case !u.Changed:
		u.Text = held
	}
	out := strings.Split(align(held, u.Text), "\n")
	for i, line := range out {
		if _, value, ok := strings.Cut(line, ":"); ok {
			if fixed, ok := kept[strings.TrimSpace(value)]; ok {
				out[i] = fixed
			}
		}
	}
	return strings.Join(out, "\n")
}

// align puts now's lines on the lines was had them on. The upgrade drops
// blank lines at the top and folds runs of them, which would move every line
// a diagnostic names. Lines are matched in order, skipping blanks, and now
// comes back as it is when a pair doesn't set the same key.
func align(was, now string) string {
	old := strings.Split(was, "\n")
	lines := strings.Split(now, "\n")
	var full []int
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			full = append(full, i)
		}
	}
	out := make([]string, len(old))
	k, last := 0, -1
	for i, line := range old {
		if strings.TrimSpace(line) == "" {
			out[i] = line
			continue
		}
		if k == len(full) || keyOf(lines[full[k]]) != keyOf(line) {
			return now
		}
		out[i], last = lines[full[k]], full[k]
		k++
	}
	text := strings.Join(out, "\n")
	if rest := strings.Join(lines[last+1:], "\n"); strings.TrimSpace(rest) != "" {
		text = strings.TrimRight(text, "\n") + "\n" + rest
	}
	return text
}

func keyOf(line string) string {
	key, _, _ := strings.Cut(line, ":")
	return strings.TrimSpace(key)
}

// keepBacktick keeps a backtick value as typed. 3 reads it as its text, as
// gitniblet did, where the 2.x reading splits it at its commas.
func keepBacktick(line string) (string, bool) {
	_, value, ok := strings.Cut(line, ":")
	return line, ok && strings.HasPrefix(strings.TrimSpace(value), "`")
}

// Quote is name as one path segment, quoted if it needs it.
func Quote(name string) string { return shcl.New().Child("", name) }

// Select is the path to the instance of name whose value is disc. The body
// is always quoted, since a bare number would select by index.
func Select(name, disc string) string {
	body := Quote(disc)
	if !strings.HasPrefix(body, `"`) && !strings.HasPrefix(body, "'") {
		body = `"` + body + `"`
	}
	return name + "(" + body + ")"
}

// Text is doc as a save writes it. It names its format, so a later load
// never takes it for 2.x.
func Text(doc *Doc) string {
	// A stamp fails only after an edit that stops the Format line going in,
	// and the text then reads as current anyway.
	_ = doc.StampFormat()
	return doc.Text()
}
