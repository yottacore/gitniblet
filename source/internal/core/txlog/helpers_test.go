// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package txlog

import "github.com/jim-collier/nano-git-db/internal/core/guid"

// testID renders a short fixture label as a valid id, so tests keep using
// readable stand-ins like "01" against the format's fixed width.
func testID(label string) string {
	var raw [guid.RawLen]byte
	copy(raw[:], label)
	return guid.Encode(raw[:])
}

func decodeID(id string) ([]byte, error) { return guid.Decode(id) }
