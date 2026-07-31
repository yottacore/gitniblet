// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package guid

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	raw := []byte("0123456789abcdef")
	text := Encode(raw)
	if len(text) != TextLen {
		t.Fatalf("encoded to %d characters, want %d", len(text), TextLen)
	}
	got, err := Decode(text)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("round trip = %x, want %x", got, raw)
	}
}

// An id pasted in the longer hex form still resolves.
func TestDecodeAcceptsHex(t *testing.T) {
	raw := []byte("0123456789abcdef")
	got, err := Decode(hex.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("hex decode = %x, want %x", got, raw)
	}
}

// A malformed id is rejected rather than guessed at: replay skips the entry
// with a warning, where a lucky decode would write a ghost row.
func TestDecodeRejectsMalformed(t *testing.T) {
	bad := []string{
		"", "01", "zz",
		strings.Repeat("a", TextLen-1),
		strings.Repeat("a", TextLen+1),
		strings.Repeat("+", TextLen), // not in the base64url alphabet
		strings.Repeat("z", HexLen),  // right width, not hex
	}
	for _, id := range bad {
		if _, err := Decode(id); err == nil {
			t.Fatalf("Decode(%q) accepted a malformed id", id)
		}
	}
}

// The log is CSV, so an id must never contain a character that needs quoting.
func TestEncodeNeedsNoCSVQuoting(t *testing.T) {
	for b := 0; b < 256; b++ {
		var raw [RawLen]byte
		for i := range raw {
			raw[i] = byte(b)
		}
		if id := Encode(raw[:]); strings.ContainsAny(id, ",\"\r\n") {
			t.Fatalf("id %q needs CSV quoting", id)
		}
	}
}
