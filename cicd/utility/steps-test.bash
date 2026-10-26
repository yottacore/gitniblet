#!/usr/bin/env bash
#  shellcheck disable=1090  ## 'Can't follow non-constant source.' The echo helpers come from cicd.bash at run time.
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The bash -c bodies expand there.
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.
#  shellcheck disable=2329  ## 'This function is never invoked.' Each is called through fCheck.
#  shellcheck disable=2154  ## 'Referenced but not assigned.' __cpuJobs and __cpuBusy come from include/cpu-limit.bash.

##	Purpose:
##		Checks the step lines and the CPU rules: include/cpu-limit.bash's job
##		counts, load check and low priority, include/steps.bash's lines with
##		cicd.bash's own echo helpers, and a quick host run of cicd.bash, once
##		passing and once with a step that fails.
##	Syntax:
##		steps-test.bash     ## about 2 minutes, most of it the quick run
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
inc="${repoRoot}/cicd/utility/include"
# shellcheck source=include/temp-run.bash
source "${inc}/temp-run.bash"
runDir="$(fTempDir_Make steps-test)"  ||  exit 1
export GITNIBLET_LOG_DIR="${runDir}/logs"  ## so the real runs below never pass for the newest run log
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not. A subshell, so set -u stops only that check.
	local id="$1" name="$2"; shift 2
	if ( "$@" ); then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## cores | load | heavy | cap | jobs | busy
fPicks(){
	local row cores load heavy cap want wantBusy bad=0
	local -a f
	# shellcheck source=include/cpu-limit.bash
	source "${inc}/cpu-limit.bash"
	for row in \
		"32|10.00|||16|0"   "32|16.00|||16|0"  "32|16.01|||8|1"   "32|17|||8|1"  "32|3|||16|0" \
		"32|0.50|heavy||8|0" "32|40|heavy||8|1" "1|0.00|||1|0"    "2|5.00|||1|1" "32|1.00||3|3|0" \
		"32||||16|0"        "x|1|||1|0"        "32|${runDir}/x[\$(touch ${runDir}/pwned)]|||16|0"
	do
		IFS='|' read -r -a f <<< "${row}"
		cores="${f[0]}"; load="${f[1]}"; heavy="${f[2]:-}"; cap="${f[3]:-}"; want="${f[4]}"; wantBusy="${f[5]}"
		__cpuJobs=""; __cpuBusy=""
		fCpuLimit_Pick "${cores}" "${load}" "${heavy}" "${cap}"
		[[ "${__cpuJobs}" == "${want}" && "${__cpuBusy}" == "${wantBusy}" ]]  ||  { echo "  ${row} gave ${__cpuJobs} ${__cpuBusy}"; bad=1; }
	done
	[[ ! -e "${runDir}/pwned" ]]  ||  { echo "  a load string ran a command"; bad=1; }
	return "${bad}"
}
fCheck dedq2 "half the CPUs, a quarter when busy or heavy, capped, never under 1" fPicks

## On this box: the count matches the load right now, and the process drops to
## nice 19 and idle I/O.
fLive(){
	local out
	out="$(bash -c 'set -Eeuo pipefail; source "$1"; c="$(nproc)"
		echo "${GITNIBLET_JOBS} ${GOMAXPROCS} $(( GITNIBLET_BUSY ? c / 4 : c / 2 )) ${GITNIBLET_JOBS_MAX} $((c / 2))"
		fCpuLimit heavy; echo "${GITNIBLET_JOBS} $(( c / 4 > 0 ? c / 4 : 1 ))"
		ps -o ni= -p $$; ionice -p $$ 2> /dev/null || echo idle' _ "${inc}/cpu-limit.bash")"
	local -a l; mapfile -t l <<< "${out}"
	local -a a b; read -r -a a <<< "${l[0]}"; read -r -a b <<< "${l[1]}"
	[[ "${a[0]}" == "${a[1]}" && "${a[0]}" == "${a[2]}" && "${a[3]}" == "${a[4]}" && "${b[0]}" == "${b[1]}" && "${l[2]// /}" == "19" && "${l[3]}" == idle* ]]  ||  { echo "  ${out//$'\n'/ | }"; return 1; }
}
fCheck dedq3 "this box: GOMAXPROCS follows the count, heavy gets a quarter, nice 19 and idle I/O" fLive

## The step lines through cicd.bash's own echo helpers.
fWithEcho(){
	bash -c 'set -Eeuo pipefail; source <(sed -n "/^## Echo-related/,/^## Error-handling/p" "$1"); source "$2"; shift 2; eval "$*"' \
		_ "${repoRoot}/cicd/cicd.bash" "${inc}/steps.bash" "$@"
}
stampRe='[0-9]{8}-[0-9]{6}\.[0-9]{2}'
rule="$(printf '%.0s•' {1..79})"
fLines(){
	local out want
	out="$(fWithEcho 'fStep_Plan A; fStep_Plan B --b-off; fStep_Plan C; fSteps_Header "T" "extra"
		if fStep_Start A; then echo "out a"; fStep_End; fi
		if fStep_Start B; then echo "ran B"; fi
		if fStep_Start C; then echo "out c"; fStep_End; fi
		fStep_Fail; fSteps_Footer "T: done" "last"; fEcho_Clean')"
	want="$(printf '%s\n' "" "${rule}" "[ S: T; 2 steps, 1 off ]" "Off: B (--b-off)" "extra" "" "${rule}" \
		"[ S: Step 1 of 2; Starting A ]" "out a" "[ S: Step 1 of 2; PASS A ]" "" "${rule}" \
		"[ S: Step 2 of 2; Starting C ]" "out c" "[ S: Step 2 of 2; PASS C ]" "" "${rule}" "[ S: T: done ]" "last" "")"
	[[ "$(sed -E "s/${stampRe}/S/g" <<< "${out}")" == "${want}" ]]  ||  { diff <(sed -E "s/${stampRe}/S/g" <<< "${out}") <(echo "${want}") | sed 's/^/  /'; return 1; }
}
fCheck dedq4 "79-char rule, local time, Step N of M, off steps not counted, one blank between" fLines

fFailLine(){
	local out
	out="$(fWithEcho 'fStep_Plan A; fStep_Plan B; if fStep_Start B; then echo boom; fi; fStep_Fail; fStep_Fail')"
	[[ "$(grep -cE "^\[ ${stampRe}: Step 2 of 2; FAIL B \]$" <<< "${out}")" == 1 ]]  &&  ! grep -q PASS <<< "${out}"
}
fCheck dedq5 "an open step closes as FAIL, once" fFailLine

fNested(){
	local out
	out="$(fWithEcho 'fStep_Plan A; fStep_Plan B; fStep_Plan C; fSteps_Export; bash -c "$(declare -f fSteps_Import); fSteps_Import; echo \${#stepsOn[@]} \${stepsOn[2]}"')"
	[[ "${out}" == "3 C" ]]  ||  { echo "  got '${out}'"; return 1; }
}
fCheck dedq6 "a nested run takes the outer run's step list" fNested

## cicd.bash itself, on this host. Every step opens and closes in order, the
## count matches, and the run ends on a blank line.
flags=(-q --quick --no-sync --no-remote --no-container --no-install --no-hooks --no-publish --no-demo --no-format)
fRunShape(){  ## log, PASS|FAIL
	awk -v want="$2" '
		/^\[ [0-9]+-[0-9]+\.[0-9]+: .*; [0-9]+ steps/ { split($0, h, "; "); sub(/ steps.*/, "", h[2]); total = h[2] + 0; next }
		match($0, /Step [0-9]+ of [0-9]+; (Starting|PASS|FAIL) /) {
			split(substr($0, RSTART), w, /[ ;]+/); n = w[2] + 0; m = w[4] + 0; what = w[5]
			if (m != total) { print "  total " m " != " total; bad = 1 }
			if (what == "Starting") { if (n != last + 1 || open) { print "  out of order: " $0; bad = 1 } open = 1; last = n }
			else { if (!open || n != last) { print "  close without open: " $0; bad = 1 } open = 0; closes[what]++ }
		}
		{ final = $0 }
		END {
			if (!total) { print "  no first section"; bad = 1 }
			if (want == "PASS" && (last != total || closes["FAIL"])) { print "  ran " last " of " total; bad = 1 }
			if (want == "FAIL" && closes["FAIL"] != 1) { print "  no FAIL line"; bad = 1 }
			if (final != "") { print "  last line not blank"; bad = 1 }
			exit bad
		}' "$1"
}
fQuickRun(){
	nice -n 19 bash "${repoRoot}/cicd/cicd.bash" "${flags[@]}" > "${runDir}/pass.out" 2>&1  ||  { tail -20 "${runDir}/pass.out"; return 1; }
	fRunShape "${runDir}/pass.out" PASS  &&  grep -qF 'CI/CD: done; PASS' "${runDir}/pass.out" \
		&&  grep -qE '^Off: Cross-compile \+ package \(--quick\)$' "${runDir}/pass.out" \
		&&  [[ "$(grep -cE '^Jobs: [0-9]+ of [0-9]+ CPUs, load ' "${runDir}/pass.out")" == "$(grep -cE 'Step [0-9]+ of [0-9]+; Starting ' "${runDir}/pass.out")" ]]
}
lintDir="${repoRoot}/cicd/artifacts/lint"
lintBefore="$(find "${lintDir}" -maxdepth 1 -name 'run_*.log' -newer "${runDir}" 2> /dev/null | wc -l)"
fCheck dedq7 "a quick host run numbers every step and closes each with PASS" fQuickRun
fLogDir(){
	compgen -G "${GITNIBLET_LOG_DIR}/run_*.log" > /dev/null  ||  { echo "  no run log in ${GITNIBLET_LOG_DIR}"; return 1; }
	(( $(find "${lintDir}" -maxdepth 1 -name 'run_*.log' -newer "${runDir}" 2> /dev/null | wc -l) == lintBefore ))  ||  { echo "  a new log in ${lintDir}"; return 1; }
}
fCheck deef2 "a run's log goes to GITNIBLET_LOG_DIR, not cicd/artifacts/lint" fLogDir

## A go that always fails, so the first step does.
mkdir -p "${runDir}/bin"
printf '#!/bin/bash\nexit 3\n' > "${runDir}/bin/go"; chmod +x "${runDir}/bin/go"
fFailRun(){
	PATH="${runDir}/bin:${PATH}" nice -n 19 bash "${repoRoot}/cicd/cicd.bash" "${flags[@]}" > "${runDir}/fail.out" 2>&1  &&  return 1
	fRunShape "${runDir}/fail.out" FAIL  &&  grep -qE "^\[ ${stampRe}: Step 1 of [0-9]+; FAIL Build \(native debug\) \]$" "${runDir}/fail.out" \
		||  { tail -20 "${runDir}/fail.out"; return 1; }
}
fCheck dedq8 "a failing step closes with FAIL and the run stops" fFailRun

## A docker stand-in that logs and passes, so only the run's arguments are checked.
## Its run ends as the real run inside does, with a step's PASS line and a blank.
printf '#!/bin/bash\necho "$*" >> "%s"\n[[ "$1" != version ]] || echo 29\n[[ "$1" != run ]] || printf "%%s\\n\\n" "[ 20261025-000000.00: Step 5 of 5; PASS Security + supply-chain ]"\nexit 0\n' "${runDir}/docker.log" > "${runDir}/bin/docker"
chmod +x "${runDir}/bin/docker"
fContainerArgs(){
	local run
	local -a ctrFlags=(-q --quick --no-sync --no-remote --no-install --no-hooks --no-publish --no-demo --no-format)
	GITNIBLET_DOCKER="${runDir}/bin/docker" nice -n 19 bash "${repoRoot}/cicd/cicd.bash" "${ctrFlags[@]}" > "${runDir}/ctr.out" 2>&1  ||  { tail -20 "${runDir}/ctr.out"; return 1; }
	run="$(grep '^run ' "${runDir}/docker.log")"
	[[ "${run}" =~ --cpus\ ([0-9]+)\ --cpu-shares\ 256\ .*-e\ GITNIBLET_STEPS\ -e\ GITNIBLET_JOBS_CAP=([0-9]+)\ .*/etc/localtime:/etc/localtime:ro.*\ nice\ -n\ 19\ bash\  ]] \
		&&  [[ "${BASH_REMATCH[1]}" == "$(( $(nproc) / 2 ))" && "${BASH_REMATCH[2]}" == "${BASH_REMATCH[1]}" ]] \
		&&  grep -qF 'Container: running gitniblet-' "${runDir}/ctr.out" \
		||  { echo "  ${run}"; return 1; }
}
fCheck dedq9 "the container gets --cpus, --cpu-shares 256, the step list, the cap and nice" fContainerArgs

## Uses dedq9's output. The next section on this host follows the run inside after one blank, not two.
fOneBlank(){
	grep -qF 'PASS Security + supply-chain ]' "${runDir}/ctr.out"  ||  { echo "  no output from the run inside"; return 1; }
	! awk 'prev == "" && $0 == "" && NR > 1 { print "  2 blank lines at line " NR; bad = 1 } { prev = $0 } END { exit !bad }' "${runDir}/ctr.out"
}
fCheck deed8 "one blank line between the run in the container and the next section" fOneBlank

exit $((fails > 0))


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: One blank after the run in the container.
##		- 20261025 JC: Run logs go in the test's folder.
