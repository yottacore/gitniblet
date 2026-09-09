// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"fmt"

	"github.com/jim-collier/nano-git-db/donate"
)

// Donate prints the support blurb and the link to open. No database is involved.
func Donate() error {
	fmt.Println()
	fmt.Println(donate.Intro)
	fmt.Println()
	fmt.Println("    " + donate.URL)
	fmt.Println()
	return nil
}
