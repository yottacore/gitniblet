// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package enc is the field-value encryption seam between the open-source core
// and the enterprise build. The core calls a registered Cipher to seal values
// into the tx-log and open them on replay; the open-source build registers none,
// so it cannot encrypt - and any already-encrypted entry it meets is left
// unreadable (the core binds it to NULL rather than leaking ciphertext into the
// view). The cryptography and key management live in the enterprise build,
// behind this interface, so the open-source binary carries none of it.
//
// The seam is deliberately narrow and string-typed: it names no core types, so
// the separate enterprise module can implement it (Go forbids importing another
// module's internal/ packages). Which fields get encrypted, the on-disk format,
// and the always/never/auto policy all stay in the open-source core - they are
// data-flow, not cryptography, and the core must understand the format anyway to
// degrade gracefully when it has no key.
package enc

import "fmt"

// Ctx is the cleartext context a value is encrypted under: TxID makes each
// value's subkey unique; Table/Field/RowID are authenticated so a sealed value
// cannot be relocated to another column or row.
type Ctx struct{ TxID, Table, Field, RowID string }

// Cipher seals and opens one field value. The enterprise build implements it;
// the returned token is line-safe text the core stores behind the tx-log's ENC
// marker.
type Cipher interface {
	Seal(ctx Ctx, plaintext string) (token string, err error)
	Open(ctx Ctx, token string) (plaintext string, err error)
}

// Provider is the enterprise encryption backend: it loads key files into Ciphers
// and mints new keys.
type Provider interface {
	// LoadCipher loads a key file into a Cipher. A missing file is the no-key
	// state, (nil, false, nil), not an error.
	LoadCipher(keyFile string) (Cipher, bool, error)
	// MintKey creates a fresh key file when absent (idempotent); it refuses to
	// overwrite an existing key, which would strand data sealed under the old one.
	MintKey(path string) error
}

var provider Provider

// Register installs the enterprise provider. The enterprise main calls it once
// at startup; the open-source build never does.
func Register(p Provider) { provider = p }

// Available reports whether an encryption provider is registered - i.e. whether
// this is the enterprise build.
func Available() bool { return provider != nil }

// LoadCipher loads a key via the registered provider, or returns the no-key
// state when none is registered (the open-source build).
func LoadCipher(keyFile string) (Cipher, bool, error) {
	if provider == nil {
		return nil, false, nil
	}
	return provider.LoadCipher(keyFile)
}

// MintKey creates a key file via the registered provider. Without one it is an
// error: key material is an enterprise capability.
func MintKey(path string) error {
	if provider == nil {
		return fmt.Errorf("encryption is an enterprise feature; this build cannot create keys")
	}
	return provider.MintKey(path)
}
