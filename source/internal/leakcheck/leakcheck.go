// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package leakcheck fails a package's tests when goroutines outlive them.
// Only tests import it, so it never reaches a binary.
package leakcheck

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

// EnvOff turns the check off when set to 0, as cicd.bash --no-leakcheck does.
const EnvOff = "GITNIBLET_LEAKCHECK"

// Run runs the tests under goleak.VerifyTestMain and returns the exit code,
// so a TestMain can clean up before it exits.
func Run(m *testing.M, opts ...goleak.Option) int {
	if os.Getenv(EnvOff) == "0" {
		return m.Run()
	}
	code := 0
	goleak.VerifyTestMain(m, append(opts, goleak.Cleanup(func(c int) { code = c }))...)
	return code
}
