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
func Load(text string, s shcl.Strictness) (*Doc, error) {
	return shcl.ParseWith(Current(text), s)
}

// Current is text as shcl 3 should read it. A file with no Format line that
// doesn't load clean on 3, such as one with a bare `a, b` list, is from
// before 3 and is upgraded. One the upgrade refuses comes back as it is, so
// the load names its bad lines.
//
// The upgrade reads the file the 2.x way, which splits a backtick value at
// its commas. Those lines skip it and keep the reading gitniblet always gave
// them, until shclwrap can keep them itself (2026102618065645). A file that
// reads two ways is taken as 2.x, since every file without a Format line was
// written for the 2.x rules.
func Current(text string) string {
	if shcl.FormatOf(text) >= shcl.FormatMajor {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := map[string]string{}
	for i, line := range lines {
		if !isBacktick(line) {
			continue
		}
		// A bare word reads the same both ways, so the upgrade leaves it be.
		token := fmt.Sprintf("gnkept%dx", i)
		key, _, _ := strings.Cut(line, ":")
		kept[token], lines[i] = line, key+": "+token
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
	// The upgrade keeps blank lines, so each line keeps its number.
	out := strings.Split(u.Text, "\n")
	for i, line := range out {
		if _, value, ok := strings.Cut(line, ":"); ok {
			if orig, ok := kept[strings.TrimSpace(value)]; ok {
				out[i] = orig
			}
		}
	}
	return strings.Join(out, "\n")
}

// isBacktick is true for a line whose value is in backticks. 3 reads it as
// its text, as gitniblet did.
func isBacktick(line string) bool {
	_, value, ok := strings.Cut(line, ":")
	return ok && strings.HasPrefix(strings.TrimSpace(value), "`")
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
