#!/usr/bin/env bash
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The workflow text is matched literally.

##	Purpose:
##		Checks the hosted workflows take their pins from where cicd.bash does:
##		Go from source/go.mod's toolchain line, goreleaser from
##		include/tool-versions.bash. Runs release.yml's own pins step, with a
##		stand-in GITHUB_OUTPUT.
##	Syntax:
##		pins-test.bash     ## about a second
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
flows="${repoRoot}/.github/workflows"
release="${flows}/release.yml"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make pins-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT
declare -i fails=0
fCase(){ if [[ "${3}" == "${4}" ]]; then echo "PASS ${1} ${2}"; else echo "FAIL ${1} ${2}: got '${3}', want '${4}'"; fails+=1; fi; }

setups="$(grep -c 'uses: actions/setup-go@' "${flows}"/*.yml | awk -F: '{n+=$2} END {print n+0}')"
fromMod="$(grep -c '^ *go-version-file: source/go.mod$' "${flows}"/*.yml | awk -F: '{n+=$2} END {print n+0}')"
fCase defqw "every setup-go reads source/go.mod" "${fromMod}" "${setups}"
fCase defqx "no workflow pins Go itself" "$(grep -lE '^ *go-version:' "${flows}"/*.yml | wc -l)" 0
((setups > 0))  ||  { echo "FAIL defqx no setup-go step found"; fails+=1; }

## The run block of the step with "id: pins", as the runner gets it.
awk '/^ *id: pins$/ {f=1; next} f && /^ *run: \|$/ {r=1; next} r && NF && !/^          / {exit} r {sub(/^          /, ""); print}' \
	"${release}" > "${runDir}/pins.bash"
out="${runDir}/out"; : > "${out}"
( cd "${repoRoot}"  &&  GITHUB_OUTPUT="${out}" bash -e "${runDir}/pins.bash" > /dev/null 2>&1 )
# shellcheck source=include/tool-versions.bash
source "${repoRoot}/cicd/utility/include/tool-versions.bash"
fCase defqy "the pins step gives tool-versions.bash's goreleaser" "$(sed -n 's/^goreleaser=//p' "${out}")" "${GITNIBLET_GORELEASER##*@}"
fCase defqz "the goreleaser action takes the pins step's version" \
	"$(awk '/uses: goreleaser\/goreleaser-action@/ {f=1} f && /^ *version:/ {sub(/^ *version: */, ""); print; exit}' "${release}")" \
	'${{ steps.pins.outputs.goreleaser }}'

exit $((fails > 0))


##	History:
##		- 20261026 JC: Created.
