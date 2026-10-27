#!/usr/bin/env bash
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' They expand in the inner bash.
#  shellcheck disable=2329  ## 'This function is never invoked.' fCheck calls it.

##	Purpose:
##		Checks include/temp-run.bash's fake home: where it points HOME and the
##		config folders, that Go keeps its real caches, and that its check
##		catches a run of gitniblet that reached the default registry.
##	Syntax:
##		config-home-test.bash     ## a few seconds; the binary case needs bin/gitniblet
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make config-home-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not
	local id="$1" name="$2"; shift 2
	if "$@"; then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}
## In a child bash, so this script keeps its own HOME. Takes the folder, then
## a snippet that sees the rest as $1 and up.
fFaked(){ bash -c 'set -Eeuo pipefail; source "$1"; dir="$2"; snippet="$3"; shift 3; fConfigHome_Use "${dir}"; eval "${snippet}"' _ "${repoRoot}/cicd/utility/include/temp-run.bash" "$@"; }

realCache="$(go env GOCACHE 2> /dev/null)"
fCheck defpe "HOME and the config folders point into the folder, Go's cache doesn't" \
	fFaked "${runDir}/a" '[[ "${HOME}" == "$1/a/home" && "${XDG_CONFIG_HOME}" == "${HOME}/.config" && "${AppData}" == "${XDG_CONFIG_HOME}" && "$(go env GOCACHE)" == "$2" ]]' "${runDir}" "${realCache}"

fCheck defpf "an untouched fake home passes" \
	fFaked "${runDir}/b" 'fConfigHome_Check "$1/b"' "${runDir}"

fCheck defpg "a registry made under either OS's config folder fails" \
	fFaked "${runDir}/c" 'mkdir "${HOME}/Library/Application Support/gitniblet" && ! fConfigHome_Check "$1/c" 2> /dev/null && rmdir "${HOME}/Library/Application Support/gitniblet" && fConfigHome_Check "$1/c" && mkdir "${XDG_CONFIG_HOME}/gitniblet" && ! fConfigHome_Check "$1/c" 2> /dev/null' "${runDir}"

exe="${repoRoot}/bin/gitniblet"
if [[ -x "${exe}" ]]; then
	## No --config, so the lookup goes to the default registry and moves the old
	## folder. Only once every config path is inside the test's folder.
	fCheck defph "gitniblet reaching the default registry is caught" \
		fFaked "${runDir}/d" 'for v in "${HOME}" "${XDG_CONFIG_HOME}" "${AppData}"; do [[ "${v}" == "$1"/d/* ]] || exit 1; done; "$2" get no-such-db x y > /dev/null 2>&1 || true; ! fConfigHome_Check "$1/d" 2> /dev/null' "${runDir}" "${exe}"
else
	echo "SKIP defph gitniblet reaching the default registry is caught, needs bin/gitniblet"
fi

exit $((fails > 0))


##	History:
##		- 20261026 JC: Created.
