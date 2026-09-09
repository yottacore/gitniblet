// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package txlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The symbol codec is the file-format boundary: encodeSym must always yield a
// single physical line (that is the whole reason the format exists), and
// decodeSym must reverse it - up to the illegal control characters encodeSym
// scrubs on purpose. Anything that violates either is a real format bug.
func FuzzSymCodec(f *testing.F) {
	for _, s := range []string{
		"", "plain", "a,b,c", "line\nbreak\rmore", "tab\there",
		nullMarker, encMarker, "quote\"'`\\/", "\U0001F5E6NOPE\U0001F5E7",
		"\x00\x01\x1fctrl\x7f", "unicode ☃ 😀",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		encoded := encodeSym(s)
		if strings.ContainsAny(encoded, "\n\r") {
			t.Fatalf("encoded value keeps a raw newline: %q -> %q", s, encoded)
		}
		// Round-trip fidelity is defined over valid UTF-8 (the codec only ever
		// sees SQLite TEXT values); invalid bytes are still exercised above for
		// the no-panic + single-line guarantees.
		if !utf8.ValidString(s) {
			return
		}
		var want strings.Builder
		for _, r := range s {
			if !isIllegal(r) {
				want.WriteRune(r)
			}
		}
		if got := decodeSym(encoded); got != want.String() {
			t.Fatalf("round-trip: %q -> %q -> %q, want %q", s, encoded, got, want.String())
		}
	})
}

// readLogFile parses whatever bytes a synced git repo hands us (another client's
// append, a merge artifact, a truncated write); it must return warnings/errors,
// never panic.
func FuzzReadLogFile(f *testing.F) {
	hdr := strings.Join(header, ",")
	for _, s := range []string{
		"",
		hdr + "\n",
		hdr + "\n01,2026-01-01,t,00,name,create,val,u,0,h\n",
		"garbage,,,,\nnot a record",
		hdr + "\n" + hdr + "\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "txlog.csv")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Skip()
		}
		_, _, _ = readLogFile(path)
	})
}
