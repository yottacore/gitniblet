// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import "strings"

// encryptOverride, when set (by the --encrypt flag via SetEncryptPref), is the
// run's local preference. Empty means "unset": use the registry record's value
// (or auto for an ad-hoc open).
var encryptOverride string

// SetEncryptPref records the --encrypt preference for this run (normalized to
// on|off|auto). Empty clears it.
func SetEncryptPref(pref string) { encryptOverride = pref }

// EncryptPref returns the run's --encrypt override, or "" if none was given.
func EncryptPref() string { return encryptOverride }

// ResolveEncryption picks the key file and preference for an open. Pass a
// registered database's record (KeyFile + stored preference), or nil and the
// DDL path for an ad-hoc open (key beside the DDL, auto). The --encrypt override
// from this run, if any, wins the preference.
func ResolveEncryption(ddlPath string, cfg *DBConfig) (keyFile, pref string) {
	if cfg != nil {
		keyFile, pref = cfg.KeyFile, cfg.Encryption
	} else {
		keyFile, pref = LocalKey(ddlPath), "auto"
	}
	if override := EncryptPref(); override != "" {
		pref = override
	}
	if pref == "" {
		pref = "auto"
	}
	return keyFile, pref
}

// ResolveEncryptionForDDL is the stateless-CLI resolver: it finds the registry
// record for ddlPath (so the config-dir key is used), falling back to an ad-hoc
// key beside the DDL. The --encrypt override still wins the preference.
func ResolveEncryptionForDDL(ddlPath string) (keyFile, pref string) {
	return ResolveEncryption(ddlPath, FindByDDL(ddlPath))
}

// NormEncrypt maps the many accepted spellings of the --encrypt value to one of
// on|off|auto. ok is false for an unrecognized value.
func NormEncrypt(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "yes", "true", "y", "t", "enable", "enabled":
		return "on", true
	case "off", "no", "false", "n", "f", "disable", "disabled":
		return "off", true
	case "", "auto", "automatic":
		return "auto", true
	default:
		return "", false
	}
}
