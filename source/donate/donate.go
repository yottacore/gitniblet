// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package donate holds the project's support blurb and the one link the
// front-ends open. --donate, the TUI Support entry, and the web Support page all
// read from here; the actual list of ways to help lives in DONATE.md, which the
// URL points at.
//
// DONATE.md, .github/FUNDING.yml, .github/CODEOWNERS, and this file are owned by
// the maintainer in CODEOWNERS, so a pull request cannot quietly change where
// support goes.
package donate

// Enabled is the master switch for the whole feature. The open-source build leaves
// it on; the enterprise (commercial) build turns it off before dispatch - you do
// not ask a paying customer to donate. It gates --donate and the TUI/web entries,
// so a disabled build shows no trace of the feature.
var Enabled = true

// Intro is the short appeal shown above the link in every front-end.
const Intro = "If you find nano-git-db useful, please consider supporting its development."

// URL is the page the front-ends open (or print): the project's DONATE.md, which
// lists every way to help.
const URL = "https://github.com/jim-collier/nano-git-db/blob/HEAD/DONATE.md"
