// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// ngdb: one binary, four front-ends over a shared core.
// The first arg picks the front-end; default is the CLI.
package main

import (
	"fmt"
	"os"

	"github.com/jim-collier/nano-git-db/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
