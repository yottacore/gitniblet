// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package guid is the row/transaction id format, shared by the log and the
// view because both render the same 16 bytes: the view stores them raw in a
// BLOB, the log writes them as 22 characters of unpadded base64url.
//
// base64url rather than hex halves the width of a field that appears twice on
// every log line. That is worth little in git (both forms hold the same 128
// bits and zlib collapses the difference) but real in the working tree and on
// every replay, which reads the whole log.
//
// Decode also accepts 32-char hex, so an id pasted in either form works. The
// two widths are exact and disjoint, so nothing has to be sniffed.
//
// NEVER re-encode an existing id. Encrypted values derive their per-value
// subkey from the tx_id and row_id STRINGS (enc.Ctx), and authenticate the row
// id as AAD, so rewriting an id from one form to the other silently strands the
// ciphertext under it. Append-only makes this a non-issue in normal operation -
// GC rotation copies lines verbatim - but a "tidy up the old ids" pass would
// destroy data. Entries keep whatever form they were written with, forever.
package guid

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	RawLen  = 16         // a UUID
	HexLen  = RawLen * 2 // 32, the alternate input form
	TextLen = 22         // unpadded base64url of 16 bytes
)

var encoding = base64.RawURLEncoding

// Encode renders a raw id in the canonical text form.
func Encode(raw []byte) string { return encoding.EncodeToString(raw) }

// Decode reads either text form. Anything that is not one of the two exact
// widths is rejected rather than guessed at: replay treats a bad row id as a
// skippable entry, and silently decoding junk into 16 plausible bytes would
// write a ghost row instead of warning.
func Decode(id string) ([]byte, error) {
	var (
		raw []byte
		err error
	)
	switch len(id) {
	case TextLen:
		raw, err = encoding.DecodeString(id)
	case HexLen:
		raw, err = hex.DecodeString(id)
	default:
		return nil, fmt.Errorf("bad id %q: want %d or %d characters, got %d",
			id, TextLen, HexLen, len(id))
	}
	if err != nil {
		return nil, fmt.Errorf("bad id %q: %w", id, err)
	}
	return raw, nil
}
