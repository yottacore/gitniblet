// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// parseHash reads an attacker-controllable stored credential string; it must
// return a zero iteration count on any malformed input and never panic.
func FuzzParseHash(f *testing.F) {
	for _, s := range []string{
		"",
		"pbkdf2-sha256$600000$c2FsdA$aGFzaA",
		"pbkdf2-sha256$$$",
		"pbkdf2-sha256$-1$x$y",
		"scrypt$1$a$b",
		"$$$$$$",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, stored string) {
		iters, _, _ := parseHash(stored)
		if iters < 0 {
			t.Fatalf("negative iteration count from %q", stored)
		}
	})
}

// Load runs the fuzzed bytes through the SHCL parser at Strict: a garbled
// registry record must surface as an error so the picker can gray it out, never
// panic. When it does load, the local-file defaults have to be filled - that is
// what lets a minimal hand-written config.shcl (a name and two paths) open.
// Seeds cover a real record, the syntax a hand-edited one picks up (comments,
// quoting, nesting, a raw block), and one input per error class the parser
// reports.
func FuzzLoadRecord(f *testing.F) {
	for _, s := range []string{
		"",
		"name: mydb\nddl_path: /p/mydb.shcl\nlog_dir: /p/ngdb/mydb\n",
		"name: mydb\n\nddl_path: /p/mydb.shcl\n\nlog_dir: /p/ngdb/mydb\n\n" +
			"sqlite_path: /c/mydb.sqlite\n\nkey_file: /c/mydb.key\n\n" +
			"encryption: auto\n\nlast_opened: \"2026-08-04T21:00:00Z\"\n",
		"## registry record\nname: \"my db\"\n\nencryption: auto\t## local pref\n",
		"name: x\nextra.nested: 1\n\tchild: 2\n",
		"name: x\nnotes:\n\t~~~\n\tline one\n\tline two\n\t~~~\n",
		"name: a\nname: b\n",                     // two instances read as none, so the dir name wins
		"name: x\n\tddl_path: a\n  log_dir: b\n", // E012 indentation matches no open level
		"[[[\nnot a field line\n",                // E014 malformed line
		"name: \"unterminated\n",                 // E017 unterminated quote
		"name: x\nnotes:\n\t~~~\n\tdangling\n",   // E005 unterminated raw block
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, recordFile), data, 0o600); err != nil {
			t.Skip()
		}
		cfg, err := Load(dir)
		if err != nil {
			return
		}
		if cfg.Name == "" || cfg.SQLitePath == "" || cfg.KeyFile == "" || cfg.Encryption == "" {
			t.Fatalf("loaded record left a default unfilled: %+v", *cfg)
		}
	})
}
