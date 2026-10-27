// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var fenceRe = regexp.MustCompile("^([ \t]*)(```|~~~)\\S*[ \t]*$")

// docSchemas returns each fenced block in a markdown file that is a whole
// schema, by its first line, as the start line and the block with the fence's
// indent taken off. Fragments and shell blocks don't open with database:.
func docSchemas(md string) map[int]string {
	out := map[int]string{}
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		m := fenceRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		var body []string
		j := i + 1
		for ; j < len(lines) && strings.TrimRight(lines[j], " \t") != m[1]+m[2]; j++ {
			body = append(body, strings.TrimPrefix(lines[j], m[1]))
		}
		for _, l := range body {
			if s := strings.TrimSpace(l); s != "" && !strings.HasPrefix(s, "##") {
				if s == "database:" {
					out[i+1] = strings.Join(body, "\n") + "\n"
				}
				break
			}
		}
		i = j
	}
	return out
}

// The schema examples in README.md and syntax.md still parse, clean. Some are
// one section, such as relationships or seed rows, so they need no tables.
// ID: defpx
func TestDocSchemasParse(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8 && !fileExists(filepath.Join(dir, "syntax.md")); i++ {
		dir = filepath.Dir(dir)
	}
	for _, name := range []string{"README.md", "syntax.md"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		blocks := docSchemas(string(b))
		if len(blocks) == 0 {
			t.Fatalf("%s: no schema blocks found", name)
		}
		for line, src := range blocks {
			s, err := Parse([]byte(src))
			if err != nil {
				t.Errorf("%s:%d: %v", name, line, err)
				continue
			}
			if len(s.Warnings) > 0 {
				t.Errorf("%s:%d: %v", name, line, s.Warnings)
			}
		}
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
