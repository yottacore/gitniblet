// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"fmt"

	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// Field-value encryption orchestration: which fields get sealed (policy) and
// the seal/open passes over log entries. It stays in the
// open-source core on purpose - it names core types (txlog.Entry, ddl.Schema)
// the separate enterprise module cannot import, and the core must understand the
// encrypted-entry format anyway to degrade gracefully without a key. The actual
// cryptography and key handling live behind enc.Cipher, which only the
// enterprise build supplies (a.cipher is nil in the open-source build).

// encDecision is the resolved per-field outcome combining the DDL directive,
// the local preference, and (at write time) key presence.
type encDecision int

const (
	encClear  encDecision = iota // never encrypt
	encIfKey                     // auto: encrypt if a key is present, else clear silently
	encPrefer                    // local "on": encrypt if a key is present, else clear
	encAlways                    // DDL "always": encrypt, or refuse the write with no key
)

// EnableEncryption arms field-value encryption on the API. cipher is the loaded
// key (nil = none available); pref is the local on|off|auto preference; schema
// supplies the DDL always|never|auto directives. Passing a non-nil schema arms
// the write-time pass even with no cipher, so a DDL `encryption: always` is
// still enforced (writes to those fields are refused). In the open-source build
// cipher is always nil, so nothing encrypts and always-fields cannot be written.
func (a *API) EnableEncryption(cipher enc.Cipher, pref string, schema *ddl.Schema) {
	a.cipher = cipher
	a.encPref = pref
	a.encSchema = schema
}

// decide resolves how one field's value should be handled on write.
func (a *API) decide(table, field string) encDecision {
	policy := "auto"
	if a.encSchema != nil {
		policy = a.encSchema.EncryptionPolicy(table, field)
	}
	switch policy {
	case "always":
		return encAlways
	case "never":
		return encClear
	default: // auto -> defer to the local preference
		switch a.encPref {
		case "off":
			return encClear
		case "on":
			return encPrefer
		default: // auto
			return encIfKey
		}
	}
}

// isSystemField is never encrypted: id is the clear row key (and the AAD/replay
// depend on it), and the other managed columns are not user field writes.
func isSystemField(f string) bool {
	switch f {
	case "", "id", "is_active", "is_deleted", "date_created":
		return true
	}
	return false
}

// encryptEntries returns copies of entries with field values sealed per the
// resolved policy - the copies go to the log, while the originals (cleartext)
// go to the view. A field that must always be encrypted but has no cipher aborts
// the whole write, so nothing is half-committed.
func (a *API) encryptEntries(entries []txlog.Entry) ([]txlog.Entry, error) {
	out := make([]txlog.Entry, len(entries))
	copy(out, entries)
	for i := range out {
		entry := &out[i]
		if isSystemField(entry.Field) || entry.IsNull {
			continue // no value to seal
		}
		switch a.decide(entry.Table, entry.Field) {
		case encClear:
			continue
		case encIfKey:
			if a.cipher == nil {
				continue // auto with no key: silently clear
			}
		case encPrefer:
			if a.cipher == nil {
				continue // "on" with no key: clear (a single warning is raised at open)
			}
		case encAlways:
			if a.cipher == nil {
				return nil, fmt.Errorf(
					"%s.%s requires encryption (encryption: always) but no key is available", entry.Table, entry.Field)
			}
		}
		token, err := a.cipher.Seal(
			enc.Ctx{TxID: entry.TxID, Table: entry.Table, Field: entry.Field, RowID: entry.RowID}, entry.NewValue)
		if err != nil {
			return nil, fmt.Errorf("encrypt %s.%s: %w", entry.Table, entry.Field, err)
		}
		entry.NewValue = token
		entry.Enc = true
	}
	return out, nil
}

// DecryptEntries opens encrypted field values in place before replay, using
// cipher (nil = no key). It returns how many entries could not be decrypted (no
// key, wrong key, or tampering); those stay Enc-marked, so Apply binds them
// NULL and the view shows unreadable encrypted fields as empty rather than
// leaking ciphertext. Run it after ApplyAliases and before txlog.Apply.
func DecryptEntries(entries []txlog.Entry, cipher enc.Cipher) int {
	var unreadable int
	for i := range entries {
		entry := &entries[i]
		if !entry.Enc {
			continue
		}
		if cipher == nil {
			unreadable++
			continue
		}
		plaintext, err := cipher.Open(
			enc.Ctx{TxID: entry.TxID, Table: entry.Table, Field: entry.Field, RowID: entry.RowID}, entry.NewValue)
		if err != nil {
			unreadable++
			continue
		}
		entry.NewValue = plaintext
		entry.Enc = false
	}
	return unreadable
}
