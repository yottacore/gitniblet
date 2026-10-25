#!/usr/bin/env bash
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The stand-ins are written literally.
#  shellcheck disable=2329  ## 'This function is never invoked.' Called from the EXIT trap.

##	Purpose:
##		Checks cicd.bash's container fallbacks, with stand-ins for docker.
##		- No docker, a docker that won't answer, and an image that won't build
##		  each print one line and carry on with the build on this host. The
##		  first case runs the quick host run to the end; the rest stop once the
##		  host build starts.
##		- A GO_VER in cicd/container/Dockerfile that doesn't match source/go.mod's
##		  toolchain line fails the run. The recipe is copied aside, edited, and
##		  put back by copy, on exit too.
##	Syntax:
##		container-fallback-test.bash     ## about 2 minutes, most of it the quick run
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
recipe="${repoRoot}/cicd/container/Dockerfile"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make container-fallback)"  ||  exit 1

fCleanup(){
	if [[ -f "${runDir}/Dockerfile.orig" ]]; then cp -p "${runDir}/Dockerfile.orig" "${recipe}"; fi
	fSafeRm "${runDir}" > /dev/null
}
trap fCleanup EXIT

## Answers version, fails the rest, so the image never builds.
printf '#!/bin/bash\n[[ "$1" == version ]] && { echo 29; exit 0; }\nexit 1\n' > "${runDir}/nobuild"
printf '#!/bin/bash\nexit 1\n' > "${runDir}/deaf"
chmod +x "${runDir}/nobuild" "${runDir}/deaf"

flags=(-q --quick --no-install --no-hooks --no-publish --no-demo --no-format)
declare -i fails=0
fCase(){  ## id, name, expected line, timeout or 0 to run to the end, docker stand-in
	local id="$1" name="$2" want="$3" secs="$4" out rc
	if ((secs)); then out="$(NGDB_DOCKER="$5" timeout "${secs}" nice -n 19 bash "${repoRoot}/cicd/cicd.bash" "${flags[@]}" 2>&1)"; rc=$?
	else              out="$(NGDB_DOCKER="$5"                   nice -n 19 bash "${repoRoot}/cicd/cicd.bash" "${flags[@]}" 2>&1)"; rc=$?; fi
	if grep -qF -- "${want}" <<< "${out}" && grep -qF '[ Build (native debug) ]' <<< "${out}" && ! grep -qF '[ Container (' <<< "${out}" \
		&& { ((secs)) || { ((rc == 0)) && grep -qF 'CI/CD: done' <<< "${out}"; }; }
	then echo "PASS ${id} ${name}"
	else echo "FAIL ${id} ${name} (exit ${rc})"; tail -20 <<< "${out}"; fails+=1; fi
}
fCase dedmb "no docker, host run to the end" "Container: no ${runDir}/missing, so build and test run on this host." 0  "${runDir}/missing"
fCase dedmc "docker not answering"           "isn't answering, so build and test run on this host."                  15 "${runDir}/deaf"
fCase dedmd "image won't build"              "didn't build, so build and test run on this host."                     15 "${runDir}/nobuild"

cp -p "${recipe}" "${runDir}/Dockerfile.orig"
sed -i 's/^ARG GO_VER=.*/ARG GO_VER=1.0.0/' "${recipe}"
out="$(NGDB_DOCKER="${runDir}/nobuild" timeout 30 bash "${repoRoot}/cicd/cicd.bash" "${flags[@]}" 2>&1)"; rc=$?
cp -p "${runDir}/Dockerfile.orig" "${recipe}"
if ((rc != 0)) && grep -qF "GO_VER in cicd/container/Dockerfile is '1.0.0'" <<< "${out}" && ! grep -qF '[ Build (native debug) ]' <<< "${out}"
	then echo "PASS dedme stale GO_VER fails the run"
	else echo "FAIL dedme stale GO_VER fails the run (exit ${rc})"; tail -20 <<< "${out}"; fails+=1; fi

exit $((fails > 0))


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: The shared run folder and guard from include/temp-run.bash.
