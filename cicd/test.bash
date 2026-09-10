#!/bin/bash

##	Purpose:
##		- CI/CD-friendly test harness that passes or fails.
##		- Static checks (go vet, gofmt) plus the full Go test suite, run from source/ (the module root).
##		- Does not launch the built binary - that stays a separate manual step before the merge.
##	History: At bottom of this file. (Note: History for this is maintained outside of [or in addition to] git project.)

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -euo pipefail
source "$(dirname "$0")/utility/include/cpu-limit.bash"  ## NGDB_JOBS + GOMAXPROCS (<=half cores)
cd "$(dirname "$0")/../source"

echo "go vet ..."
go vet -mod=vendor -p "${NGDB_JOBS}" ./...

echo "gofmt check ..."
unformatted="$(gofmt -l cmd internal)"
[[ -n "${unformatted}" ]]  &&  { echo "gofmt needed: ${unformatted}"; exit 1; }

echo "go test ..."
go test -mod=vendor -p "${NGDB_JOBS}" ./...

## No binary launch here - the app is left for a separate manual run before
## the merge, so it can be canceled if it misbehaves.

echo "test.bash: PASS"


##	Script history:
##		- 20260420 JC: Created (convert-base-v2 template).
##		- 20260701 JC: Replaced the template's round-trip suite with this project's Go checks.
##		- 20260701 JC: Binary moved to bin/ (build.bash now lives in cicd/ and outputs there).
##		- 20260703 JC: Dropped the binary smoke test - launching stays a separate manual step.
