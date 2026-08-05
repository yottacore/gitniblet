// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"os"
	"path/filepath"
	"testing"
)

// The schema parser reads whatever a user hands it, so it must degrade to
// warnings or an error on any input, never panic or hang. The seed corpus pulls
// in the real example.shcl when present, plus a few shapes that stress
// indentation, restated sections and the path shorthand.

func seedDDL(f *testing.F) {
	f.Helper()
	for _, s := range []string{
		"",
		"database:\n\ttable: t\n\t\tfields:\n\t\t\tname: string\n",
		"a/b/c: 1\n\t\t\tbad indent\n",
		"database:\ndatabase:\n\ttable: t\n",
		"ui:\n\tview: v\n\t\tblock: t\n",
		":\n\t:\n",
	} {
		f.Add([]byte(s))
	}
	if b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "project", "example.shcl")); err == nil {
		f.Add(b)
	}
}

func FuzzParse(f *testing.F) {
	seedDDL(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		_, _ = Parse(src)
	})
}

func FuzzParseQueries(f *testing.F) {
	for _, s := range []string{
		"",
		"query_name: q\n\tview: v\n\tsql: select 1\n",
		"query_name:\nquery_name: q\n",
		"\t\t\tsql: \"unterminated\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		_, _, _ = ParseQueries(src)
	})
}
