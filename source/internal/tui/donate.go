// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package tui

import (
	"github.com/jim-collier/nano-git-db/donate"
)

// donateModal shows the support appeal and the link to visit in a dismissable
// dialog. A terminal cannot reliably open a browser, so it displays the URL.
func (p *picker) donateModal() {
	p.modal("support", donate.Intro+"\n\n"+donate.URL, []string{"Close"}, func(string) {})
}
