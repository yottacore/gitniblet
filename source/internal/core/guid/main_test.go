// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package guid

import (
	"os"
	"testing"

	"github.com/yottacore/gitniblet/internal/leakcheck"
)

func TestMain(m *testing.M) { os.Exit(leakcheck.Run(m)) }
