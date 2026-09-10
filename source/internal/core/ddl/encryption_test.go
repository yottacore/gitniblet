// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import "testing"

const encDDL = "" +
	"encryption: auto\n" +
	"database:\n" +
	"\ttables:\n" +
	"\t\ttable: person\n" +
	"\t\t\tencryption: auto\n" +
	"\t\t\tfields:\n" +
	"\t\t\t\tfield: name\n" +
	"\t\t\t\t\ttype: string\n" +
	"\t\t\t\tfield: ssn\n" +
	"\t\t\t\t\ttype: string\n" +
	"\t\t\t\t\tencryption: always\n" +
	"\t\t\t\tfield: public_id\n" +
	"\t\t\t\t\ttype: string\n" +
	"\t\t\t\t\tencryption: never\n" +
	"\t\ttable: secrets\n" +
	"\t\t\tencryption: always\n" +
	"\t\t\tfields:\n" +
	"\t\t\t\tfield: value\n" +
	"\t\t\t\t\ttype: string\n" +
	"\t\t\t\tfield: label\n" +
	"\t\t\t\t\ttype: string\n" +
	"\t\t\t\t\tencryption: never\n"

func TestEncryptionParseAndResolve(t *testing.T) {
	s, err := Parse([]byte(encDDL))
	if err != nil {
		t.Fatal(err)
	}
	if s.Encryption != "auto" {
		t.Fatalf("db encryption = %q", s.Encryption)
	}
	cases := []struct {
		table, field, want string
	}{
		{"person", "name", "auto"},       // all auto -> defer
		{"person", "ssn", "always"},      // field lock
		{"person", "public_id", "never"}, // field lock
		{"secrets", "value", "always"},   // table lock inherited
		{"secrets", "label", "always"},   // table 'always' cannot be overridden lower
	}
	for _, c := range cases {
		if got := s.EncryptionPolicy(c.table, c.field); got != c.want {
			t.Errorf("%s.%s: got %q want %q", c.table, c.field, got, c.want)
		}
	}
}

func TestEncryptionUnknownValueWarns(t *testing.T) {
	s, err := Parse([]byte("database:\n\ttables:\n\t\ttable: t\n\t\t\tencryption: sometimes\n\t\t\tfields:\n\t\t\t\tfield: f\n\t\t\t\t\ttype: string\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.table("t").Encryption != "" {
		t.Fatalf("bad value should be dropped to unset, got %q", s.table("t").Encryption)
	}
	found := false
	for _, w := range s.Warnings {
		if len(w) > 0 && (contains(w, "sometimes")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning about the bad encryption value, got %v", s.Warnings)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
