#!/bin/bash
#  shellcheck disable=2034  ## 'appears unused.' stepsOn and stepsOff are read by the script that sources this.

##	Purpose:
##		Step lines for cicd.bash, per the CICD style guide. A step opens under the
##		rule with "[ <time>: Step N of M; Starting <name> ]" and closes with the
##		same line saying PASS or FAIL. Time is local, YYYYmmDD-HHMMSS.NN. Steps are
##		planned before the first one runs, so one that's off isn't counted.
##		- fStep_Plan NAME [WHY]: adds a step, or with WHY, lists it as off.
##		- fSteps_Header TITLE [LINE...]: the first section, with the count, what's
##		  off and why, then the extra lines.
##		- fStep_Start NAME: opens NAME and returns 0, or 1 when it isn't planned.
##		- fStep_End: closes the open step as PASS. fStep_Fail closes it as FAIL,
##		  and does nothing when none is open, so an exit trap can call it.
##		- fSteps_Footer TITLE [LINE...]: the last section.
##		- A nested run, such as the one in the container, fills stepsOn from the
##		  outer run's list (fSteps_Export) so its numbers carry on.
##		Uses the caller's fEcho, fEcho_Clean and fEcho_Section.
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

declare -a stepsOn=() stepsOff=()
declare stepOpen="" stepOpenNum=""

fStep_Plan(){ if [[ -n "${2:-}" ]]; then stepsOff+=("${1} (${2})"); else stepsOn+=("${1}"); fi; }

## For a nested run's environment: NGDB_STEPS, the names joined by |.
fSteps_Export(){ local IFS='|'; NGDB_STEPS="${stepsOn[*]}"; export NGDB_STEPS; }
fSteps_Import(){ IFS='|' read -r -a stepsOn <<< "${NGDB_STEPS:-}"; }

fStep_Stamp(){
	local -n stamp_t8vq="${1}"
	local now="${EPOCHREALTIME:-}"
	now="${now/,/.}"  ## some locales
	if [[ "${now}" =~ ^([0-9]+)\.([0-9]{2}) ]]; then
		printf -v stamp_t8vq '%(%Y%m%d-%H%M%S)T.%s' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
	else
		printf -v stamp_t8vq '%(%Y%m%d-%H%M%S)T.00' -1
	fi
}

fSteps_Header(){
	local stamp line off=""
	fStep_Stamp stamp
	if ((${#stepsOff[@]})); then off=", ${#stepsOff[@]} off"; fi
	fEcho_Section "${stamp}: ${1}; ${#stepsOn[@]} steps${off}"
	for line in "${stepsOff[@]}"; do fEcho_Clean "Off: ${line}"; done
	shift
	for line in "${@}"; do fEcho_Clean "${line}"; done
}

fStep_Start(){
	local i stamp
	for i in "${!stepsOn[@]}"; do
		[[ "${stepsOn[i]}" == "${1}" ]]  ||  continue
		stepOpen="${1}"; stepOpenNum="$((i + 1))"
		fStep_Stamp stamp
		fEcho_Section "${stamp}: Step ${stepOpenNum} of ${#stepsOn[@]}; Starting ${stepOpen}"
		return 0
	done
	return 1
}

fStep_Close(){
	[[ -n "${stepOpen}" ]]  ||  return 0
	local stamp; fStep_Stamp stamp
	fEcho "${stamp}: Step ${stepOpenNum} of ${#stepsOn[@]}; ${1} ${stepOpen}"
	fEcho_Clean
	stepOpen=""
}
fStep_End(){  fStep_Close PASS; }
fStep_Fail(){ fStep_Close FAIL; }

fSteps_Footer(){
	local stamp line
	fStep_Stamp stamp
	fEcho_Section "${stamp}: ${1}"
	shift
	for line in "${@}"; do fEcho_Clean "${line}"; done
}


##	History:
##		- 20261025 JC: Created.
