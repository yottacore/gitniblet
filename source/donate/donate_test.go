// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package donate

import (
	"strings"
	"testing"
)

func TestFields(t *testing.T) {
	if Intro == "" {
		t.Error("Intro must not be empty")
	}
	if !strings.HasPrefix(URL, "https://") {
		t.Errorf("URL should be an https link, got %q", URL)
	}
}
