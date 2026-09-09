// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import "testing"

func TestNormEncrypt(t *testing.T) {
	on := []string{"on", "yes", "true", "y", "t", "ENABLE", "Enabled"}
	off := []string{"off", "no", "false", "n", "f", "disable", "DISABLED"}
	auto := []string{"", "auto", "Automatic"}
	for _, s := range on {
		if v, ok := NormEncrypt(s); !ok || v != "on" {
			t.Errorf("%q -> %q,%v want on", s, v, ok)
		}
	}
	for _, s := range off {
		if v, ok := NormEncrypt(s); !ok || v != "off" {
			t.Errorf("%q -> %q,%v want off", s, v, ok)
		}
	}
	for _, s := range auto {
		if v, ok := NormEncrypt(s); !ok || v != "auto" {
			t.Errorf("%q -> %q,%v want auto", s, v, ok)
		}
	}
	if _, ok := NormEncrypt("maybe"); ok {
		t.Error("unknown value should not validate")
	}
}

// The --encrypt override wins the preference; otherwise the record's value (or
// auto for an ad-hoc open) is used.
func TestResolveEncryptionPrecedence(t *testing.T) {
	SetEncryptPref("")
	t.Cleanup(func() { SetEncryptPref("") })

	kf, pref := ResolveEncryption("/x/db.ddl", nil)
	if kf != "/x/db.key" || pref != "auto" {
		t.Fatalf("ad-hoc default: key=%q pref=%q", kf, pref)
	}

	cfg := &DBConfig{KeyFile: "/reg/db.key", Encryption: "off"}
	_, pref = ResolveEncryption("/x/db.ddl", cfg)
	if pref != "off" {
		t.Fatalf("record pref should apply: %q", pref)
	}

	SetEncryptPref("on")
	kf, pref = ResolveEncryption("/x/db.ddl", cfg)
	if pref != "on" || kf != "/reg/db.key" {
		t.Fatalf("override should win pref, keep record key: key=%q pref=%q", kf, pref)
	}
}
