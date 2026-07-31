# Pinned versions of the linters/scanners the pipeline runs via `go run`. One
# place so results don't drift across machines or over time. These stay out of
# go.mod on purpose (no vendored tool dep); dependabot bumps go.mod deps and the
# toolchain, these pins are bumped by hand when a scan wants a newer tool.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT
NGDB_STATICCHECK="honnef.co/go/tools/cmd/staticcheck@v0.7.0"
NGDB_GOVULNCHECK="golang.org/x/vuln/cmd/govulncheck@v1.6.0"
NGDB_GOSEC="github.com/securego/gosec/v2/cmd/gosec@v2.27.1"
NGDB_GORELEASER="github.com/goreleaser/goreleaser/v2@v2.17.0"
