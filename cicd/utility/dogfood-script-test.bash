#!/usr/bin/env bash
#  shellcheck disable=2329  ## 'This function is never invoked.' Each is called through fCheck.
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.

##	Purpose:
##		Checks utility/dogfood_gitniblet.ps1 and its bash launcher under a fake
##		HOME: taking a new build, passing arguments and the exit code through,
##		the GFS pool, the fixed name, and the Dogfood step's copies of both.
##		A box with no pwsh gets a SKIP line for each check that needs it.
##	Syntax:
##		dogfood-script-test.bash     ## about 15 seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail
export TZ=UTC  ## the fake builds' times, and the stamps the script reads from them

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
ps1="${repoRoot}/utility/dogfood_gitniblet.ps1"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
# shellcheck source=include/dogfood.bash
source "${repoRoot}/cicd/utility/include/dogfood.bash"
runDir="$(fTempDir_Make dogfood-script-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

fEcho_Clean(){ echo "$*"; }
declare -i fails=0 havePwsh=0
command -v pwsh > /dev/null  &&  havePwsh=1
fCheck(){  ## id, name, then a command that passes, fails, or returns 2 to skip. A subshell, so set -u stops only that check.
	local id="$1" name="$2" rc; shift 2
	if ((! havePwsh)) && [[ "${1}" != fScriptCopy ]]; then echo "SKIP ${id} ${name}: no pwsh"; return; fi
	( "$@" ); rc=$?
	if ((rc == 0)); then echo "PASS ${id} ${name}"; elif ((rc == 2)); then echo "SKIP ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## <name>: a fake HOME with the Linux dogfood folder. The fake build echoes
## its arguments, one per line, and exits 3.
fSetup(){
	d="${runDir}/$1"; HOME="${d}/home"; app="${HOME}/synced/0-0/common/exec/app/linux"; pool="${HOME}/.local/bin/gitniblet_versions"
	mkdir -p "${app}"  ||  return 1
	fBuild "${2:-one}" '2026-09-20 10:00:00'
}
## <tag> <time>: a new build in the dogfood folder.
fBuild(){
	printf '#!/bin/sh\nprintf "%s:%%s\\n" "$@"\nexit 3\n' "$1" > "${app}/gitniblet"  &&  chmod +x "${app}/gitniblet"  &&  touch -d "$2" "${app}/gitniblet"
}
fDogfood_Run(){ env -u DISPLAY -u WAYLAND_DISPLAY HOME="${HOME}" pwsh -NoProfile -File "${ps1}" "$@" 2> "${d}/err" ;}

fFirstRun(){
	local out rc
	fSetup first  ||  return 1
	out="$(fDogfood_Run -v 'a b' '--x=url=http://y' '')"; rc=$?
	[[ "${out}" == $'one:-v\none:a b\none:--x=url=http://y\none:' && "${rc}" == 3 ]]  ||  { echo "  out ${out@Q} rc ${rc}"; return 1; }
	[[ "$(readlink "${HOME}/.local/bin/gitniblet")" == "${pool}/gitniblet_20260920-100000_newest" ]]  ||  { echo "  fixed name: $(ls -l "${HOME}/.local/bin")"; return 1; }
	grep -q 'new build 20260920-100000' "${d}/err"  ||  { echo "  stderr: $(< "${d}/err")"; return 1; }
	## Nothing new: quiet, and a restamped twin isn't a new version.
	touch -d '2026-09-21 10:00:00' "${app}/gitniblet"
	out="$(fDogfood_Run x)"  &&  :
	[[ "${out}" == "one:x" && ! -s "${d}/err" && "$(find "${pool}" -type f | wc -l)" == 1 ]]  ||  { echo "  second run: ${out@Q} $(< "${d}/err")"; return 1; }
}
fCheck dehyr "takes the new build, passes arguments and the exit code through, then stays quiet" fFirstRun

fPool(){
	local out n stamp
	fSetup pool  ||  return 1
	mkdir -p "${pool}"  &&  echo junk > "${pool}/gitniblet_junk"  ||  return 1
	for stamp in 20260801-120000 20260805-120000 20260810-120000 20260815-120000 20260820-120000 20260825-120000 \
		20260901-120000 20260902-120000 20260903-120000 20260904-120000 20260905-120000 20260906-120000; do
		printf '#!/bin/sh\nexit 0\n' > "${pool}/gitniblet_${stamp}"; chmod +x "${pool}/gitniblet_${stamp}"
	done
	## One that is running, and that GFS alone would drop.
	cp /bin/sleep "${pool}/gitniblet_20260805-120000"  ||  return 1
	"${pool}/gitniblet_20260805-120000" 30 &
	standIn="$!"  ## not local, since the trap runs after the function returns
	trap 'kill "${standIn}" 2> /dev/null' EXIT  ## the check's own subshell
	sleep 0.3
	fBuild two '2026-09-20 10:00:00'
	out="$(fDogfood_Run y)"  &&  :
	n="$(find "${pool}" -type f -name 'gitniblet_2026*' | wc -l)"
	[[ "${out}" == "two:y" ]]  ||  { echo "  ran ${out@Q}"; return 1; }
	((n >= 5 && n <= 11))  ||  { echo "  kept ${n}"; return 1; }
	[[ -e "${pool}/gitniblet_junk" && -e "${pool}/gitniblet_20260805-120000" ]]  ||  { echo "  removed junk or the running one: $(ls "${pool}")"; return 1; }
	[[ -e "${pool}/gitniblet_20260920-100000_newest" && -e "${pool}/gitniblet_20260801-120000_oldest" ]]  ||  { echo "  lost newest or oldest: $(ls "${pool}")"; return 1; }
}
fCheck dehys "the pool keeps 5 to 10 by GFS, the oldest and newest, and never one that's running" fPool

fFixedName(){
	local out
	fSetup fixed  ||  return 1
	mkdir -p "${HOME}/.local/bin"  &&  echo mine > "${HOME}/.local/bin/gitniblet"  ||  return 1
	out="$(fDogfood_Run w)"  &&  :
	[[ "$(< "${HOME}/.local/bin/gitniblet")" == mine && "${out}" == "one:w" ]]  ||  { echo "  ${out@Q}"; return 1; }
	grep -q 'regular file' "${d}/err"  ||  { echo "  said nothing: $(< "${d}/err")"; return 1; }
	## --no-update copies nothing, and runs what's held.
	fBuild three '2026-09-22 10:00:00'
	out="$(fDogfood_Run --no-update v)"  &&  :
	[[ "${out}" == "one:v" && "$(find "${pool}" -type f | wc -l)" == 1 ]]  ||  { echo "  --no-update: ${out@Q}"; return 1; }
}
fCheck dehyt "a regular file at the fixed name is left alone, and --no-update copies nothing" fFixedName

## The launcher from the synced layout, through a link on PATH, as a shell finds it.
fLauncher(){
	local out util rc
	fSetup launcher  ||  return 1
	util="${HOME}/synced/0-0/common/exec/util"
	mkdir -p "${util}/0_crossplatform" "${util}/linux/bash" "${d}/path"  ||  return 1
	cp "${ps1}" "${util}/0_crossplatform/"  &&  cp "${repoRoot}/utility/dogfood_gitniblet" "${util}/linux/bash/"  ||  return 1
	ln -s "${util}/linux/bash/dogfood_gitniblet" "${d}/path/dogfood_gitniblet"
	out="$(cd / && env -u DISPLAY -u WAYLAND_DISPLAY HOME="${HOME}" PATH="${d}/path:${PATH}" dogfood_gitniblet 'p q' r 2> "${d}/err")"; rc=$?
	[[ "${out}" == $'one:p q\none:r' && "${rc}" == 3 ]]  ||  { echo "  ${out@Q} rc ${rc} $(< "${d}/err")"; return 1; }
}
fCheck dehyv "the bash launcher finds the script in the synced layout and passes everything through" fLauncher

## No pwsh needed: the Dogfood step's copies, and the skips.
fScriptCopy(){
	local out util
	HOME="${runDir}/copy/home"; util="${HOME}/synced/0-0/common/exec/util"
	mkdir -p "${util}/0_crossplatform" "${util}/linux/bash" "${util}/mswin/cli/by-self/cmd"  ||  return 1
	out="$(fDogfood_Script "${repoRoot}/utility")"  ||  { echo "  ${out}"; return 1; }
	cmp -s "${ps1}" "${util}/0_crossplatform/dogfood_gitniblet.ps1"  &&  cmp -s "${repoRoot}/utility/dogfood_gitniblet.cmd" "${util}/mswin/cli/by-self/cmd/dogfood_gitniblet.cmd"  ||  { echo "  ${out}"; return 1; }
	[[ -x "${util}/linux/bash/dogfood_gitniblet" && ! -e "${util}/macos" ]]  ||  { echo "  ${out}"; return 1; }
	[[ "${out}" == *"Dogfood: no ${util}/macos/bash, skipped."* ]]  ||  { echo "  ${out}"; return 1; }
	## cicd.bash's Dogfood step calls it.
	sed -n '/^\tif fStep "Dogfood"; then$/,/^\tfi$/p' "${repoRoot}/cicd/cicd.bash" | grep -qF "fDogfood_Script \"\${dirPath_Base}/utility\""  ||  { echo "  not in the Dogfood step"; return 1; }
	## The .cmd keeps CRLF, or cmd.exe misreads its labels.
	[[ "$(tr -cd '\r' < "${repoRoot}/utility/dogfood_gitniblet.cmd" | wc -c)" == "$(wc -l < "${repoRoot}/utility/dogfood_gitniblet.cmd")" ]]  ||  { echo "  .cmd is not CRLF"; return 1; }
}
fCheck dehyw "the Dogfood step copies the script and launchers to the util folders that exist" fScriptCopy

fAnalyze(){
	local out
	if ! pwsh -NoProfile -Command 'exit [int](-not (Get-Module -ListAvailable -Name PSScriptAnalyzer))' > /dev/null 2>&1; then echo "  no PSScriptAnalyzer"; return 2; fi
	out="$(pwsh -NoProfile -Command "\$s = @{ Severity = @('Error', 'Warning'); Rules = @{ PSUseCompatibleSyntax = @{ Enable = \$true; TargetVersions = @('5.1', '7.0') }; PSUseConsistentIndentation = @{ Enable = \$true; Kind = 'tab' } } }; Invoke-ScriptAnalyzer -Path '${ps1}' -Settings \$s | ForEach-Object { '{0}:{1} {2}' -f \$_.RuleName, \$_.Line, \$_.Message }" 2>&1)"
	[[ -z "${out}" ]]  ||  { echo "  ${out//$'\n'/$'\n'  }"; return 1; }
}
fCheck dehyx "PSScriptAnalyzer finds nothing, for pwsh 7 and Windows PowerShell 5.1" fAnalyze

((fails == 0))  ||  { echo "${fails} dogfood script check(s) failed"; exit 1; }
