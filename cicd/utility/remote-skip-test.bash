#!/usr/bin/env bash
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The stand-ins are written literally.
#  shellcheck disable=2329  ## 'This function is never invoked.' Called from the EXIT trap.

##	Purpose:
##		Checks remote-test.bash's skip and fail paths with stand-ins for the
##		host lock's wrap and for ssh, so no test box is touched.
##		- A box that doesn't answer, a box that can't be held, and no wrap
##		  helper at all are each a SKIP line, and the run exits 0.
##		- A box that answers and then fails is a FAIL, and the run exits 1.
##		- Each run removes its own run folder.
##	Syntax:
##		remote-skip-test.bash     ## about a minute, most of it the cross-builds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make remote-skip)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

## wrap: WRAP_MODE=run runs what follows --, hold gives up before it, as a lock wait would.
## ssh: SSH_MODE=dead never answers, half answers the probe and fails the rest.
mkdir "${runDir}/bin"
printf '#!/bin/bash\n[[ "${WRAP_MODE}" == run ]] || exit 3\nwhile (($#)) && [[ "$1" != -- ]]; do shift; done; shift\nexec "$@"\n' > "${runDir}/wrap"
printf '#!/bin/bash\n[[ "${SSH_MODE}" == half && "${*: -1}" == 0 ]] && exit 0\nexit 255\n' > "${runDir}/bin/ssh"
chmod +x "${runDir}/wrap" "${runDir}/bin/ssh"
## Its own temp root, so another run's folders don't count against dednb.
mkdir -p "${runDir}/temp/${tempProject}"

declare -i fails=0
fCase(){  ## id, name, expected exit, expected line, env for the run
	local id="$1" name="$2" wantRc="$3" want="$4" out rc; shift 4
	out="$(env "$@" nice -n 19 bash "${repoRoot}/cicd/utility/remote-test.bash" freebsd macos 2>&1)"; rc=$?
	if ((rc == wantRc)) && grep -qF -- "${want}" <<< "${out}"
	then echo "PASS ${id} ${name}"
	else echo "FAIL ${id} ${name} (exit ${rc})"; tail -20 <<< "${out}"; fails+=1; fi
}
stand=(NGDB_RUN_DIR= NGDB_VM_WRAP="${runDir}/wrap" PATH="${runDir}/bin:${PATH}" HOST_TEMP="${runDir}/temp")
fCase dedn7 "box that doesn't answer is a SKIP" 0 "SKIP macos: no answer from b26, vmKt6m"             "${stand[@]}" WRAP_MODE=run  SSH_MODE=dead
fCase dedn8 "box that can't be held is a SKIP"  0 "SKIP freebsd: no answer from vmFreeBSD"             "${stand[@]}" WRAP_MODE=hold SSH_MODE=dead
fCase dedn9 "no wrap helper is a SKIP"          0 "SKIP freebsd: no VM and host lock wrap helper here" NGDB_VM_WRAP= HOME=/nonexistent
fCase dedna "box that fails is a FAIL"          1 "FAIL freebsd on vmFreeBSD"                          "${stand[@]}" WRAP_MODE=run  SSH_MODE=half

left="$(find "${runDir}/temp/${tempProject}" -mindepth 1 -maxdepth 1 | wc -l)"
if ((left == 0)); then echo "PASS dednb runs remove their own folders"
else echo "FAIL dednb runs remove their own folders (${left} left)"; fails+=1; fi

exit $((fails > 0))


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: The shared run folder and guard from include/temp-run.bash.
