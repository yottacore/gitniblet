// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package web

import (
	"net/http"

	"github.com/jim-collier/nano-git-db/donate"
)

// donate renders the support partial into #main: the blurb and a link to the
// project's DONATE.md.
func (s *server) donate(w http.ResponseWriter, r *http.Request) {
	s.render(w, "donate.html", map[string]any{
		"Intro": donate.Intro, "URL": donate.URL,
	})
}
