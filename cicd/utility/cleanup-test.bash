#!/usr/bin/env bash
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The stand-in is written literally.
#  shellcheck disable=2329  ## 'This function is never invoked.' Called from the EXIT trap.

##	Purpose:
##		Checks the cleanup after a run: include/temp-run.bash's run folders, stale
##		sweep and guard, and cicd.bash removing its run folder and its docker
##		leftovers on success, failure and Ctrl+C, with a stand-in for docker. The
##		stand-in's run passes, fails or hangs at once, so no build happens.
##	Syntax:
##		cleanup-test.bash     ## about 15 seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make cleanup-test)"  ||  exit 1
export GITNIBLET_LOG_DIR="${runDir}/logs"
fCleanup(){ jobs -p | xargs -r kill 2> /dev/null; fSafeRm "${runDir}" > /dev/null; }
trap fCleanup EXIT

declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not
	local id="$1" name="$2"; shift 2
	if "$@"; then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## The include on its own, against a temp root of this test's.
fInclude(){ env -u GITNIBLET_RUN_DIR HOST_TEMP="${runDir}/inc" bash -c 'set -Eeuo pipefail; source "$1"; shift; "$@"' _ "${repoRoot}/cicd/utility/include/temp-run.bash" "$@"; }
mkdir -p "${runDir}/inc"
incRoot="${runDir}/inc/${tempProject}"

made="$(fInclude fTempRun_Make)"
fCheck dedp4 "run folder is <stamp>_<NNN> with run.pid" \
	bash -c '[[ "$1" =~ /[0-9]{8}-[0-9]{6}_[0-9]{3}$ && -d "$1" ]] && read -r p < "$1/run.pid" && [[ "${p}" =~ ^[0-9]+$ ]]' _ "${made}"

inner="$(GITNIBLET_RUN_DIR="${made}" HOST_TEMP="${runDir}/inc" bash -c 'source "$1"; fTempDir_Make smoke' _ "${repoRoot}/cicd/utility/include/temp-run.bash")"
fCheck dedp5 "helper folder goes inside the cicd run's" bash -c '[[ "$1" == "$2"/smoke.* && -d "$1" ]]' _ "${inner}" "${made}"

## Old and dead, old and alive, new and dead, old with an empty run.pid, and a
## name that isn't a run's.
mkdir "${incRoot}"/20200101-000000_00{1,2,3,4} "${incRoot}/keepme"
echo 999999999 > "${incRoot}/20200101-000000_001/run.pid"
echo "$$"      > "${incRoot}/20200101-000000_002/run.pid"
: > "${incRoot}/20200101-000000_004/run.pid"
touch -d '3 days ago' "${incRoot}"/20200101-000000_00{1,2,4} "${incRoot}/keepme"
fInclude fTempRun_Sweep > /dev/null
fCheck dedp6 "sweep removes an old run folder with no live process" test ! -e "${incRoot}/20200101-000000_001" -a ! -e "${incRoot}/20200101-000000_004"
fCheck dedp7 "sweep keeps a live one, a new one, and other names" \
	test -d "${incRoot}/20200101-000000_002" -a -d "${incRoot}/20200101-000000_003" -a -d "${incRoot}/keepme" -a -d "${made}"

## The guard refuses each of these, and every one is still there after.
mkdir -p "${runDir}/inc/real/tmp/x"
ln -s "${runDir}/inc/real" "${incRoot}/link"
fRefused(){
	local p
	fInclude fSafeRm "" 2> /dev/null && return 1
	fInclude fSafeRm "${incRoot}/a" "${incRoot}/b" 2> /dev/null && return 1
	for p in "${incRoot}/../inc" "${incRoot}//keepme" "${incRoot}/link" "${incRoot}/link/tmp/x" "${incRoot}/gone" /etc /tmp; do
		fInclude fSafeRm "${p}" 2> /dev/null && { echo "  removed ${p}"; return 1; }
	done
	[[ -d "${runDir}/inc/real/tmp/x" && -d "${incRoot}/keepme" ]]
}
fCheck dedp8 "guard refuses bad, linked, missing and outside paths" fRefused

## cicd.bash with a docker stand-in. DMODE: pass, fail or hang at the run.
mkdir -p "${runDir}/bin" "${runDir}/cicd"
cur="gitniblet-cicd:$(git -C "${repoRoot}" hash-object "${repoRoot}/cicd/container/Dockerfile" | cut -c1-12)"
cat > "${runDir}/bin/docker" <<- 'EOF'
	#!/bin/bash
	echo "$*" >> "${DLOG}"
	case "$1" in
		version) echo 29 ;;
		run)     case "${DMODE}" in fail) exit 1 ;; hang) trap '' INT; sleep 30 & echo "$!" > "${DLOG}.hang"; wait ;; esac ;;
		ps)      printf 'c1\nc2\n' ;;
		rm)      [[ -f "${DLOG}.hang" ]] && kill "$(cat "${DLOG}.hang")" 2> /dev/null ;;
		image)   [[ "$2" == ls ]] || exit 0
		         if [[ "$*" == *dangling=true* ]]; then echo d1
		         else printf '%s\n' "${DCUR}" gitniblet-cicd:old1 gitniblet-cicd:old2 gitniblet-cicd:old3; fi ;;
		system)  if [[ -e "${DLOG}.df" ]]; then printf '1.5GB\n0B\n'; else touch "${DLOG}.df"; printf '2GB\n0B\n'; fi ;;
	esac
	exit 0
	EOF
chmod +x "${runDir}/bin/docker"

fCicd(){  ## mode, log, then extra args
	local mode="$1" log="$2"; shift 2
	env -u GITNIBLET_RUN_DIR DMODE="${mode}" DLOG="${log}" DCUR="${cur}" GITNIBLET_DOCKER="${runDir}/bin/docker" HOST_TEMP="${runDir}/cicd" \
		nice -n 19 bash "${repoRoot}/cicd/cicd.bash" -q --quick --no-sync --no-remote --no-install --no-hooks --no-publish --no-demo --no-format "$@"
}
fLeft(){ find "${runDir}/cicd/${tempProject}" -mindepth 1 -maxdepth 1 2> /dev/null | wc -l; }
fRanWith(){ grep -E '^run ' "$1" | grep -qF -- "$2"; }

fCicd pass "${runDir}/pass.log" > "${runDir}/pass.out" 2>&1
fPassed(){
	local log="${runDir}/pass.log" stamp
	stamp="$(grep -oE 'dev\.run=[0-9_-]+' "${log}" | head -1)"; stamp="${stamp#dev.run=}"
	[[ "${stamp}" =~ ^[0-9]{8}-[0-9]{6}_[0-9]{3}$ ]]                                                || { echo "  no run stamp"; return 1; }
	fRanWith "${log}" "--rm" && fRanWith "${log}" "--name gitniblet-${stamp}-pipeline"             || { echo "  run args"; return 1; }
	fRanWith "${log}" "-v ${runDir}/cicd/${tempProject}/${stamp}:${runDir}/cicd/${tempProject}/${stamp}" || { echo "  no run folder mount"; return 1; }
	grep -qxF -- "ps -aq --filter label=dev.project=gitniblet --filter label=dev.run=${stamp}" "${log}" || { echo "  ps filter"; return 1; }
	grep -qxF "rm -f c1 c2" "${log}" && grep -qxF "rmi d1" "${log}"                                   || { echo "  containers or dangling"; return 1; }
	grep -qxF "rmi gitniblet-cicd:old2" "${log}" && grep -qxF "rmi gitniblet-cicd:old3" "${log}"  || { echo "  old tags kept"; return 1; }
	! grep -qE "^rmi (${cur}|gitniblet-cicd:old1)$" "${log}"                                        || { echo "  removed a kept tag"; return 1; }
	grep -qxF "builder prune -f --filter until=168h" "${log}"                                         || { echo "  no builder prune"; return 1; }
	grep -qE '^Docker cleanup: .*freed about 500MB\.$' "${runDir}/pass.out"                           || { echo "  no cleanup line"; return 1; }
	(($(fLeft) == 0))                                                                                  || { echo "  run folder left"; return 1; }
}
fCheck dedp9 "passing run removes its folder, containers, dangling and old tags" fPassed

fCicd fail "${runDir}/fail.log" > "${runDir}/fail.out" 2>&1
fFailed(){
	local log="${runDir}/fail.log"
	grep -qxF "rm -f c1 c2" "${log}" && grep -qxF "rmi d1" "${log}"   || { echo "  containers or dangling"; return 1; }
	! grep -qE '^rmi gitniblet-cicd:' "${log}"                       || { echo "  pruned tags after a failure"; return 1; }
	(($(fLeft) == 0))                                                  || { echo "  run folder left"; return 1; }
}
fCheck dedpa "failed run still cleans up, but keeps old tags" fFailed

## Job control, so the run gets its own process group and Ctrl+C isn't ignored.
## The hanging stand-in ignores Ctrl+C and waits out its container, as docker's
## client does, so the run has to stop it rather than wait.
set -m
fCicd hang "${runDir}/hang.log" > "${runDir}/hang.out" 2>&1 &
hangPid=$!
for _ in {1..100}; do grep -q '^run ' "${runDir}/hang.log" 2> /dev/null && break; sleep 0.2; done
kill -INT -- "-${hangPid}" 2> /dev/null
SECONDS=0; wait "${hangPid}"; hangSecs=${SECONDS}
set +m
fInterrupted(){
	grep -q '^run ' "${runDir}/hang.log"                                 || { echo "  never reached the run"; return 1; }
	((hangSecs < 10))                                                    || { echo "  took ${hangSecs}s to stop"; return 1; }
	grep -qxF "rm -f c1 c2" "${runDir}/hang.log"                         || { echo "  containers left"; return 1; }
	(($(fLeft) == 0))                                                    || { echo "  run folder left"; return 1; }
}
fCheck dedpb "Ctrl+C removes the run folder and containers" fInterrupted

fCicd pass "${runDir}/keep.log" --keep-temp > "${runDir}/keep.out" 2>&1
fKept(){
	local log="${runDir}/keep.log" kept
	! fRanWith "${log}" "--rm"                                             || { echo "  ran with --rm"; return 1; }
	! grep -qE '^rm ' "${log}" && grep -qxF "stop c1 c2" "${log}"         || { echo "  containers removed"; return 1; }
	kept="$(sed -n 's/^Kept the run folder: //p' "${runDir}/keep.out")"
	[[ -n "${kept}" && -d "${kept}" ]]                                     || { echo "  no kept folder"; return 1; }
	fSafeRm "${kept}" > /dev/null
}
fCheck dedpc "--keep-temp keeps the folder and containers, and says where" fKept

fNoBroadRemoval(){
	local line
	while IFS= read -r line; do
		case "${line}" in
			"rmi -f"*|*prune*)  [[ "${line}" == "builder prune -f --filter until=168h" ]]  ||  { echo "  ${line}"; return 1; } ;;
			"ps "*|"image ls"*) [[ "${line}" == *"label=dev.project=gitniblet"* ]]     ||  { echo "  ${line}"; return 1; } ;;
		esac
	done < <(cat "${runDir}"/{pass,fail,hang,keep}.log)
}
fCheck dedpd "never rmi -f, another prune, or an unlabeled lookup" fNoBroadRemoval

if ((fails)); then
	for f in pass fail hang keep; do echo "--- ${f}.out"; tail -15 "${runDir}/${f}.out"; done
fi
exit $((fails > 0))


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: Run logs go in the test's folder.
