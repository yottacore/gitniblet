#!/usr/bin/env bash

##	Purpose:
##		Checks what the CodeQL step lets through, by feeding codeql-check small
##		SARIF files: our own code always counts, vendor/ only for the security
##		queries, an accepted entry passes, and one that matches nothing fails.
##		Also that the bundle pin is there. CodeQL itself runs in the CodeQL step.
##	Syntax:
##		codeql-test.bash     ## a few seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
checkDir="${repoRoot}/cicd/utility/codeql-check"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make codeql-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT
declare -i fails=0
fCase(){ if [[ "${3}" == "${4}" ]]; then echo "PASS ${1} ${2}"; else echo "FAIL ${1} ${2}: got '${3}', want '${4}'"; fails+=1; fi; }

GOFLAGS='' go -C "${checkDir}" build -o "${runDir}/codeql-check" .  ||  { echo "FAIL dehy4 codeql-check doesn't build"; exit 1; }

## A SARIF file with a security rule and a quality rule, and one result per
## "<rule> <path>" argument.
fSarif(){
	local out="${1}" sep="" r
	shift
	{
		printf '{"runs":[{"tool":{"driver":{"name":"CodeQL"},"extensions":[{"rules":['
		printf '{"id":"go/sql-injection","properties":{"tags":["security","external/cwe/cwe-089"]}},'
		printf '{"id":"go/redundant-operation","properties":{"tags":["correctness"]}}]}]},"results":['
		for r in "$@"; do
			printf '%s{"ruleId":"%s","message":{"text":"m"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"%s"},"region":{"startLine":3}}}]}' \
				"${sep}" "${r% *}" "${r#* }"
			sep=","
		done
		printf ']}]}\n'
	} > "${out}"
}
## Exit code of codeql-check over the results given, with the accepted list in ${accepted}.
fRun(){ fSarif "${runDir}/r.sarif" "$@"; "${runDir}/codeql-check" "${runDir}/r.sarif" "${accepted}" > "${runDir}/out" 2>&1; echo "$?"; }

accepted="${runDir}/none.txt"; : > "${accepted}"
fCase dehy4 "a quality result in our own code fails" "$(fRun "go/redundant-operation internal/a.go")" 1
fCase dehy5 "a quality result in vendor/ passes" "$(fRun "go/redundant-operation vendor/x/y.go")" 0
fCase dehy6 "a security result in vendor/ fails" "$(fRun "go/sql-injection vendor/x/y.go")" 1

accepted="${runDir}/some.txt"
printf '# why\n\ngo/sql-injection vendor/x/y.go  ## reason\ngo/redundant-operation internal/a.go\n' > "${accepted}"
fCase dehy7 "accepted results pass" "$(fRun "go/sql-injection vendor/x/y.go" "go/redundant-operation internal/a.go" "go/redundant-operation internal/a.go")" 0
fCase dehy8 "an accepted entry that matches nothing fails" "$(fRun "go/sql-injection vendor/x/y.go")" 1
grep -q '^STALE go/redundant-operation internal/a.go' "${runDir}/out"  ||  { echo "FAIL dehy8 no STALE line"; fails+=1; }

accepted="${runDir}/bad.txt"; printf 'go/sql-injection a.go no hashes before the reason\n' > "${accepted}"
fCase dehy9 "a malformed accepted line can't pass" "$(fRun)" 2
accepted="${repoRoot}/cicd/utility/codeql-accepted.txt"
fCase dehy9 "codeql-accepted.txt reads" "$(fRun)" 1

# shellcheck source=include/tool-versions.bash
source "${repoRoot}/cicd/utility/include/tool-versions.bash"
[[ "${GITNIBLET_CODEQL_VER:-}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]];  fCase dehya "the CodeQL version is pinned" "$?" 0
[[ "${GITNIBLET_CODEQL_SHA256:-}" =~ ^[0-9a-f]{64}$ ]];  fCase dehya "the CodeQL bundle's sha256 is pinned" "$?" 0

exit $((fails > 0))


##	History:
##		- 20261028 JC: Created.
