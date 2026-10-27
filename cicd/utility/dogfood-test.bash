#!/usr/bin/env bash
#  shellcheck disable=2329  ## 'This function is never invoked.' Each is called through fCheck.
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.
#  shellcheck disable=2034  ## 'appears unused.' dirPath_Base and the exe path are read by cicd.bash's step, run through eval.

##	Purpose:
##		Checks include/dogfood.bash against folders under a fake HOME: which
##		build goes where, the in-use skip, and the one-line skips.
##	Syntax:
##		dogfood-test.bash     ## about 2 seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
# shellcheck source=include/dogfood.bash
source "${repoRoot}/cicd/utility/include/dogfood.bash"
runDir="$(fTempDir_Make dogfood-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

fEcho_Clean(){ echo "$*"; }
declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not. A subshell, so set -u stops only that check.
	local id="$1" name="$2"; shift 2
	if ( "$@" ); then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## <name>: a fake HOME with the 3 app folders, a release build, and a dist/
## with every amd64 and arm64 build, each with its own bytes.
fSetup(){
	local d="${runDir}/$1" os arch
	HOME="${d}/home"; app="${HOME}/synced/0-0/common/exec/app"; bin="${d}/bin/gitniblet"; dist="${d}/dist"
	mkdir -p "${app}/linux" "${app}/mswin" "${app}/macos" "${d}/bin"  ||  return 1
	echo "linux release $1" > "${bin}"  &&  chmod +x "${bin}"  ||  return 1
	for os in linux windows darwin; do for arch in amd64_v1 arm64_v8.0; do
		mkdir -p "${dist}/gitniblet_${os}_${arch}"
		if [[ "${os}" == windows ]]; then echo "${os} ${arch}" > "${dist}/gitniblet_${os}_${arch}/gitniblet.exe"
		else echo "${os} ${arch}" > "${dist}/gitniblet_${os}_${arch}/gitniblet"; fi
	done; done
}

fFirstDir(){
	local out d="${runDir}/first"
	fSetup first  &&  mkdir -p "${d}/second"  ||  return 1
	out="$(fDogfood_Copy "${bin}" "${d}/missing" "${d}/second/" "${app}/linux")"  ||  return 1
	cmp -s "${bin}" "${d}/second/gitniblet"  &&  [[ ! -e "${app}/linux/gitniblet" && "${out}" == "Dogfood: ${bin} -> ${d}/second/gitniblet" ]]  ||  { echo "  ${out}"; return 1; }
	[[ -x "${d}/second/gitniblet" && -z "$(find "${d}/second" -name '*.new.*')" ]]  ||  { echo "  not executable, or a temp copy left"; return 1; }
}
fCheck defr4 "the build goes to the first folder that exists, whole and executable" fFirstDir

## <how> is open (a process holds it open) or run (a process runs it).
fInUse(){
	local how="$1" out dst before pathNoFuser="${runDir}/nofuser-$1" tool
	fSetup "inuse-${how}"  ||  return 1
	dst="${app}/linux/gitniblet"
	cp /bin/sleep "${dst}"  &&  before="$(cksum < "${dst}")"  ||  return 1
	if [[ "${how}" == open ]]; then sleep 30 3< "${dst}" & else "${dst}" 30 & fi
	standIn="$!"  ## not local, since the trap runs after the function returns
	trap 'kill "${standIn}" 2> /dev/null' EXIT  ## the check's own subshell
	sleep 0.3
	out="$(fDogfood_Copy "${bin}" "${app}/linux")"  ||  return 1
	[[ "${out}" == "Dogfood: ${dst} is in use, skipped." && "$(cksum < "${dst}")" == "${before}" ]]  ||  { echo "  ${out}"; return 1; }
	## The /proc pass, for a box with no fuser.
	mkdir -p "${pathNoFuser}"  &&  for tool in find realpath; do ln -s "$(command -v "${tool}")" "${pathNoFuser}/${tool}"; done
	PATH="${pathNoFuser}" fDogfood_InUse "${dst}"  ||  { echo "  /proc pass missed it"; return 1; }
	kill "${standIn}"; wait "${standIn}" 2> /dev/null
	! fDogfood_InUse "${dst}"  &&  ! PATH="${pathNoFuser}" fDogfood_InUse "${dst}"  ||  { echo "  still in use after the stand-in quit"; return 1; }
	out="$(fDogfood_Copy "${bin}" "${app}/linux")"  &&  cmp -s "${bin}" "${dst}"  ||  { echo "  not copied once free: ${out}"; return 1; }
}
fCheck defr5 "a copy held open is skipped with one line, then copied once free" fInUse open
fCheck defr6 "a copy that's running is skipped with one line" fInUse run

fCross(){
	local out
	fSetup cross  &&  out="$(fDogfood "${bin}" "${dist}" 1)"  ||  { echo "  ${out:-}"; return 1; }
	cmp -s "${bin}" "${app}/linux/gitniblet"  &&  [[ "$(< "${app}/mswin/gitniblet.exe")" == "windows amd64_v1" && "$(< "${app}/macos/gitniblet")" == "darwin amd64_v1" ]] \
		||  { echo "  ${out}"; ls -R "${app}"; return 1; }
	[[ ! -e "${app}/mswin/gitniblet" && ! -e "${app}/macos/gitniblet.exe" ]]  ||  { echo "  wrong name in a folder"; return 1; }
}
fCheck defr7 "a cross-built run copies the amd64 Windows and macOS builds beside the Linux one" fCross

fNoCross(){
	local out
	fSetup nocross  &&  out="$(fDogfood "${bin}" "${dist}" 0)"  ||  return 1
	cmp -s "${bin}" "${app}/linux/gitniblet"  &&  [[ -z "$(find "${app}/mswin" "${app}/macos" -mindepth 1)" ]]  ||  { echo "  ${out}"; return 1; }
	[[ "${out}" == *$'\n'"Dogfood: no Windows or macOS build this run, skipped." ]]  ||  { echo "  ${out}"; return 1; }
}
fCheck defr8 "with no cross-build this run, an older dist/ is left alone" fNoCross

fSkips(){
	local out before
	fSetup skips  ||  return 1
	out="$(fDogfood_Copy "${bin}" "${runDir}/skips/nope" "${runDir}/skips/nada")"  &&  [[ "${out}" == "Dogfood: no ${runDir}/skips/nope ${runDir}/skips/nada, skipped." ]]  ||  { echo "  ${out}"; return 1; }
	fDogfood_Copy "${bin}" "${app}/linux" > /dev/null  &&  touch -d '2001-02-03 04:05' "${app}/linux/gitniblet"  &&  before="$(stat -c %Y "${app}/linux/gitniblet")"  ||  return 1
	out="$(fDogfood_Copy "${bin}" "${app}/linux")"  &&  [[ "${out}" == "Dogfood: ${app}/linux/gitniblet is already this build." && "$(stat -c %Y "${app}/linux/gitniblet")" == "${before}" ]]  ||  { echo "  ${out}"; return 1; }
}
fCheck defr9 "no folder, or the same bytes there, is one line and not a failure" fSkips

## cicd.bash's own Dogfood step, lifted out and run with the step helpers
## stubbed: cross-built or not, per the plan.
fStepWiring(){
	local block out
	block="$(sed -n '/^\tif fStep "Dogfood"; then$/,/^\tfi$/p' "${repoRoot}/cicd/cicd.bash")"
	[[ "${block}" == *fDogfood* ]]  ||  { echo "  no Dogfood step calling fDogfood in cicd.bash"; return 1; }
	eval "fDogfoodStep(){ ${block}
	}"
	fStep(){ [[ "$1" == Dogfood ]]; }; fStep_End(){ :; }; fThrowError(){ echo "$*"; return 1; }
	fStep_IsOn(){ local s; for s in "${stepsOn[@]}"; do [[ "${s}" != "$1" ]]  ||  return 0; done; return 1; }
	fSetup wiring-quick  &&  dirPath_Base="${runDir}/wiring-quick"  &&  filePath_ExecToTestAndInstall_FinalHome="${bin}"  ||  return 1
	stepsOn=("Build (native release)" Dogfood)
	out="$(fDogfoodStep)"  &&  cmp -s "${bin}" "${app}/linux/gitniblet"  &&  [[ -z "$(find "${app}/mswin" "${app}/macos" -mindepth 1)" ]]  ||  { echo "  quick: ${out}"; return 1; }
	fSetup wiring-full  &&  dirPath_Base="${runDir}/wiring-full"  &&  filePath_ExecToTestAndInstall_FinalHome="${bin}"  ||  return 1
	stepsOn=("Build (native release)" "Cross-compile + package" Dogfood)
	out="$(fDogfoodStep)"  &&  [[ "$(< "${app}/mswin/gitniblet.exe")" == "windows amd64_v1" && "$(< "${app}/macos/gitniblet")" == "darwin amd64_v1" ]]  ||  { echo "  full: ${out}"; return 1; }
}
fCheck defra "cicd.bash's Dogfood step copies Windows and macOS only when the run cross-built" fStepWiring

((fails == 0))  ||  { echo "${fails} dogfood check(s) failed"; exit 1; }
