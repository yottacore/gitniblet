// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"fmt"
	"io"

	"github.com/jim-collier/nano-git-db/gate"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
)

// applyGate renders the startup notice for a non-interactive command. A Nag
// prints a one-line notice and runs normally; a Blocking posture prints the
// notice and drops the session to read-only - there is no start screen to wait
// out on the command line, so writes return ErrReadOnly until the user opens the
// app to restore write access. Full is silent. The open-source build registers
// no provider, so this is always Full and prints nothing.
func applyGate(api *crud.API, stderr io.Writer) {
	st := gate.Evaluate()
	switch st.Posture {
	case gate.Nag:
		fmt.Fprintln(stderr, "notice:", st.Message)
	case gate.Blocking:
		fmt.Fprintln(stderr, "notice:", st.Message)
		fmt.Fprintln(stderr, "notice: running read-only; open the app to restore write access")
		api.SetReadOnly(true)
	}
}
