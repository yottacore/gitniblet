// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"strconv"
	"strings"
)

// The DDL's special values (design.md). These are type-directed: the caller
// asks for the type it expects, which resolves ambiguities like `1,000` (one
// number) vs a comma list.

// Unquote strips a matching pair of double or single quotes. The alternate
// quote may appear literally inside; there is no backslash escaping.
func Unquote(s string) (string, bool) {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1], true
		}
	}
	return s, false
}

// The default-value sentinels, a closed set. They are spelled with a leading @
// rather than brackets because SHCL reserves [ and ] inside a value.
const (
	// SentinelNull is an explicit "no value" default.
	SentinelNull = "@null"
	// SentinelPrevious repeats the value from the last row entered in this
	// session. Interactive front-ends only - a programmatic write has no
	// "previous row" and never inherits it.
	SentinelPrevious = "@previous"
)

// IsSentinel reports whether a default is one of the reserved sentinels. Both
// mean "the app computes this", so neither is ever emitted as a SQL default.
//
// Quoting escapes them: a default arrives here as its source text, so `@null`
// is the sentinel and `"@null"` is the four-character string, which falls
// through to the ordinary literal path and has its quotes stripped there. Do
// not unquote before this test, or the escape closes.
func IsSentinel(s string) bool {
	switch strings.TrimSpace(s) {
	case SentinelNull, SentinelPrevious:
		return true
	}
	return false
}

// IsNull reports a default that resolves to NULL: the @null sentinel, or the
// bare word NULL that predated it.
func IsNull(s string) bool {
	trimmed := strings.TrimSpace(s)
	return trimmed == SentinelNull || trimmed == "NULL"
}

// AsBool parses the DDL boolean vocabulary. Second return is false if not a bool.
func AsBool(s string) (bool, bool) {
	trimmed := strings.TrimSpace(s)
	trimmed, _ = Unquote(trimmed)
	switch strings.ToLower(trimmed) {
	case "true", "1", "t", "yes", "y", "enable", "enabled":
		return true, true
	case "false", "0", "f", "no", "n", "disable", "disabled":
		return false, true
	}
	return false, false
}

// AsFloat parses a number, tolerating thousands commas and a bare leading or
// trailing dot (`.1`, `1.`).
func AsFloat(s string) (float64, bool) {
	normalized, ok := normNum(s)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// AsInt parses an integer-valued number (rejects fractional values).
func AsInt(s string) (int, bool) {
	f, ok := AsFloat(s)
	if !ok || f != float64(int64(f)) {
		return 0, false
	}
	return int(f), true
}

// AsFunc matches a bare `name(args)` script-function reference.
func AsFunc(s string) (name, args string, ok bool) {
	s = strings.TrimSpace(s)
	if u, q := Unquote(s); q {
		s = u
	}
	open := strings.IndexByte(s, '(')
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	if name = s[:open]; !isIdent(name) {
		return "", "", false
	}
	return name, s[open+1 : len(s)-1], true
}

// AsSQL matches a backtick-wrapped SQL literal and returns its inner text. A
// fenced raw block is the canonical spelling now that the DDL is SHCL - the
// reader hands blocks back already unwrapped, so this only has to recognize the
// older inline backtick form.
func AsSQL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1], true
	}
	return "", false
}

// isIdent reports whether s is a bareword identifier - a script function name,
// or a field name in a place that has to be one.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// SplitList splits a top-level comma list, honoring quotes/backticks/brackets,
// then trims and unquotes each element. Empty elements are dropped.
func SplitList(s string) []string {
	parts := splitTop(s)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if u, q := Unquote(part); q {
			part = u
		}
		out = append(out, part)
	}
	return out
}

// splitTop splits on unquoted, unbracketed top-level commas.
func splitTop(s string) []string {
	var quote byte
	depth := 0
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// normNum canonicalizes a number string for strconv (strips commas, pads a bare
// leading/trailing dot, keeps sign).
func normNum(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s, _ = Unquote(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return "", false
	}
	sign := ""
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		sign, s = "-", s[1:]
	}
	if s == "" || s == "." {
		return "", false
	}
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	if strings.HasSuffix(s, ".") {
		s += "0"
	}
	return sign + s, true
}
