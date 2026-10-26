#!/bin/bash

##	Purpose:
##		- CI/CD-friendly test harness that passes or fails.
##		- Static checks (go vet, gofmt) plus the full Go test suite, run from source/ (the module root).
##		- Every Go test needs a unique ID, and prints one PASS, FAIL or SKIP line with it (utility/testid).
##		- Does not launch the built binary - that stays a separate manual step before the merge.
##	Syntax:
##		test.bash [package...]   ## e.g. ./internal/cli/...; only those packages' tests, and not testid's own
##	History: At bottom of this file. (Note: History for this is maintained outside of [or in addition to] git project.)

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -euo pipefail
source "$(dirname "$0")/utility/include/cpu-limit.bash"  ## NGDB_JOBS + GOMAXPROCS (<=half cores)
repoRoot="$(realpath -e "$(dirname "$0")/..")"
idDir="${repoRoot}/cicd/utility/testid"   ## its own module, outside vendor mode, hence GOFLAGS=
cd "${repoRoot}/source"
declare -a pkgs=("$@")

echo "go vet ..."
go vet -mod=vendor -p "${NGDB_JOBS}" ./...
GOFLAGS=''  go -C "${idDir}" vet .

echo "gofmt check ..."
unformatted="$(gofmt -l cmd internal "${idDir}")"
[[ -n "${unformatted}" ]]  &&  { echo "gofmt needed: ${unformatted}"; exit 1; }

echo "test IDs ..."
GOFLAGS=''  go -C "${idDir}" run . check "${repoRoot}"

echo "go test ..."
fGoTests(){  ## Both go test exits count, since a go command that dies early says nothing in JSON.
	local rc=0
	go test -json -mod=vendor -p "${NGDB_JOBS}" -parallel "${NGDB_JOBS}" "${pkgs[@]:-./...}"  ||  rc=1
	((${#pkgs[@]}))  ||  GOFLAGS=''  go -C "${idDir}" test -json -p "${NGDB_JOBS}" .        ||  rc=1
	return "${rc}"
}
fGoTests | GOFLAGS='' go -C "${idDir}" run . report "${repoRoot}"

## No binary launch here - the app is left for a separate manual run before
## the merge, so it can be canceled if it misbehaves.

echo "test.bash: PASS"


##	Script history:
##		- 20260420 JC: Created (convert-base-v2 template).
##		- 20260701 JC: Replaced the template's round-trip suite with this project's Go checks.
##		- 20260701 JC: Binary moved to bin/ (build.bash now lives in cicd/ and outputs there).
##		- 20260703 JC: Dropped the binary smoke test - launching stays a separate manual step.
##		- 20261025 JC: -parallel passed explicitly.
##		- 20261025 JC: Test IDs checked, and one PASS/FAIL/SKIP line per test with its ID.
##		- 20261025 JC: Takes packages, for cicd.bash --backlog-run-quick's areas.
