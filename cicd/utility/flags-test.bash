#!/usr/bin/env bash
#  shellcheck disable=2329  ## 'This function is never invoked.' Each is called through fCheck.
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' The stand-in's body expands when it runs.
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.

##	Purpose:
##		Checks the shared flag set through cicd.bash --list, which plans the run
##		and runs nothing, and the pre-push hook's choice to gate a push or not,
##		against a scratch repo with GITNIBLET_GATE_DRYRUN=1.
##	Syntax:
##		flags-test.bash     ## about 5 seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
cicd="${repoRoot}/cicd/cicd.bash"
hook="${repoRoot}/cicd/utility/git-hooks/pre-push"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make flags-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT
## The plans below expect Local hooks on, which needs the hooks folder. A
## hosted runner has no ../private/cicd-hooks, so stand one in.
export GITNIBLET_HOOKS_DIR="${runDir}/hooks"
mkdir -p "${GITNIBLET_HOOKS_DIR}"  ||  exit 1

declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not. A subshell, so set -u stops only that check.
	local id="$1" name="$2"; shift 2
	if ( "$@" ); then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## The steps --list plans, joined by |, then the off ones.
fPlan(){ bash "${cicd}" --list "$@" 2>&1 | sed -n 's/^Step [0-9]*: //p' | paste -sd '|'; }
fOff(){  bash "${cicd}" --list "$@" 2>&1 | sed -n 's/^Off: //p' | paste -sd '|'; }
fWant(){ [[ "$1" == "$2" ]]  ||  { echo "  got:  $1"; echo "  want: $2"; return 1; }; }

fHandsOff(){
	local mode bad=0 off
	for mode in --backlog-run-quick --backlog-run-full --merge-gate --gate --ci; do
		off="$(fOff "${mode}")"
		[[ "|${off}|" == *"|Dogfood ("* && "|${off}|" == *"|Demo recording ("* && "|${off}|" == *"|Local hooks ("* && "|${off}|" == *"|Backup + publish ("* ]]  ||  { echo "  ${mode}: ${off}"; bad=1; }
		[[ "$(fPlan "${mode}")" != *Dogfood* ]]  ||  bad=1
	done
	fWant "$(fPlan --backlog-run-quick)" 'Build (native release)|Lint + format|Tests'  ||  bad=1
	fWant "$(fPlan --backlog-run-full)" 'Build (native debug)|Build (native release)|Cross-compile + package|Lint + format|Tests|Fuzz|Security + supply-chain|Race tests|Pipeline tests|Remote tests|Profiler'  ||  bad=1
	fWant "$(fPlan --merge-gate)" "$(fPlan --backlog-run-full)"  ||  bad=1
	return "${bad}"
}
fCheck deecd "the hands-off modes plan no dogfood, demo, hooks or publish; the quick gate is build, lint and tests" fHandsOff

fOtherModes(){
	local bad=0 full
	full="$(fPlan)"
	fWant "$(fPlan --long)" "${full}"  ||  bad=1
	fWant "$(fPlan --slow)" "${full}"  ||  bad=1
	[[ "${full}" == *"|Cross-compile + package|"*"|Remote tests|Dogfood|Profiler|"* ]]  ||  { echo "  default: ${full}"; bad=1; }
	fWant "$(fPlan --quick --no-remote)" "$(fPlan --quick)"  ||  bad=1
	[[ "$(fPlan --quick)" == *"|Fuzz|Security + supply-chain|Race tests|Dogfood|"* ]]  ||  { echo "  quick: $(fPlan --quick)"; bad=1; }
	[[ "$(fOff --quick)" == 'Cross-compile + package (--quick)|Pipeline tests (--quick)|Remote tests (--quick)|Profiler (--quick)|Demo recording (--quick)' ]]  ||  { echo "  quick off: $(fOff --quick)"; bad=1; }
	[[ "$(fOff --no-remote)" == *'Remote tests (--no-remote)'* ]]  ||  bad=1
	return "${bad}"
}
fCheck deece "--long and --slow are the default run; --quick keeps fuzz, security and dogfood" fOtherModes

## --no-format isn't here, since it changes what lint does, not which steps run.
fOffFlags(){
	local bad=0 pair flag name full want
	full="$(fPlan)"
	for pair in '--no-install:Dogfood' '--no-hooks:Local hooks' '--no-publish:Backup + publish' '--no-demo:Demo recording'; do
		flag="${pair%%:*}"; name="${pair#*:}"
		want="|${full}|"; want="${want/"|${name}|"/|}"; want="${want#|}"; want="${want%|}"
		[[ "${want}" != "${full}" ]]  ||  { echo "  ${name} isn't in the default plan"; bad=1; }
		fWant "$(fPlan "${flag}")" "${want}"  ||  bad=1
		fWant "$(fOff "${flag}")" "${name} (${flag})"  ||  bad=1
	done
	return "${bad}"
}
fCheck deee3 "--no-install, --no-hooks, --no-publish and --no-demo each turn off only their own step" fOffFlags

fNoHooksDir(){
	local full want missing="${runDir}/no-hooks"
	full="$(fPlan)"
	want="|${full}|"; want="${want/"|Local hooks|"/|}"; want="${want#|}"; want="${want%|}"
	[[ "${want}" != "${full}" ]]  ||  { echo "  Local hooks isn't in the default plan"; return 1; }
	fWant "$(GITNIBLET_HOOKS_DIR="${missing}" fPlan)" "${want}"  &&  fWant "$(GITNIBLET_HOOKS_DIR="${missing}" fOff)" "Local hooks (no ${missing})"
}
fCheck deehh "with no hooks folder, Local hooks is off and the rest of the plan is the same" fNoHooksDir

## Every *-test.bash beside cicd.bash, so a new one can't be left out, but
## remote-test.bash, which is the Remote tests step and uses the real boxes.
fRaceAndPipeline(){
	local bad=0 out script id want
	out="$(bash "${cicd}" --list 2>&1)"
	[[ "|$(fPlan)|" == *"|Security + supply-chain|Race tests|Pipeline tests|Remote tests|"* ]]  ||  { echo "  default: $(fPlan)"; bad=1; }
	[[ "$(fOff --backlog-run-quick)" == *'Race tests (--backlog-run-quick)|Pipeline tests (--backlog-run-quick)'* ]]  ||  { echo "  quick gate off: $(fOff --backlog-run-quick)"; bad=1; }
	[[ "|$(CC=nosuchcc fOff)|" == *'|Race tests (no nosuchcc for -race)|'* ]]  ||  { echo "  no cc: $(CC=nosuchcc fOff)"; bad=1; }
	sed -n '/^Race tests:$/,/^Pipeline tests:$/p' <<< "${out}" | grep -qE '^dec9z internal/tui TestLookupsShowInTUI$'  ||  { echo "  race list lacks dec9z"; bad=1; }
	! sed -n '/^Race tests:$/,/^Pipeline tests:$/p' <<< "${out}" | grep -E '^[0-9a-z]{5} ' | grep -qv ' internal/tui '  ||  { echo "  race list goes past the TUI"; bad=1; }
	! grep -q 'remote-test\.bash$' <<< "${out}"  ||  { echo "  remote-test.bash is listed"; bad=1; }
	for script in "${repoRoot}"/cicd/utility/*-test.bash; do
		[[ "${script##*/}" != "remote-test.bash" ]]  ||  continue
		id=""; read -r _ id _ < <(grep -oE '(fCheck|fCase|PASS) [0-9a-hjkmnp-tv-z]{5} ' "${script}")
		want="${id} ${script##*/}"
		sed -n '/^Pipeline tests:$/,$p' <<< "${out}" | grep -qxF "${want}"  ||  { echo "  pipeline list lacks ${want}"; bad=1; }
	done
	return "${bad}"
}
fCheck deef1 "race and pipeline tests run in the default run, not the quick gate, and --list names their IDs" fRaceAndPipeline

fAliases(){
	local out bad=0
	out="$(bash "${cicd}" --list --gate 2>&1)"; [[ "$(head -1 <<< "${out}")" == "--gate is another name for --backlog-run-quick." ]]  ||  bad=1
	out="$(bash "${cicd}" --list --ci 2>&1)";   [[ "$(head -1 <<< "${out}")" == "--ci is another name for --merge-gate." ]]  ||  bad=1
	out="$(bash "${cicd}" --list --merge-gate 2>&1)"; ! grep -q 'another name' <<< "${out}"  ||  bad=1
	bash "${cicd}" --list --no-sync --leakcheck --no-container --keep-temp > /dev/null 2>&1  ||  { echo "  a taken flag failed"; bad=1; }
	! bash "${cicd}" --list --leakcheck --no-leakcheck > /dev/null 2>&1  ||  { echo "  both leak flags passed"; bad=1; }
	return "${bad}"
}
fCheck deecf "--gate and --ci say what they stand for, and the taken flags parse" fAliases

fLeakFlags(){
	local bad=0
	fLeakLine(){ bash "${cicd}" --list --no-sync --no-container "$@" 2>&1 | grep '^Leak checks: ' ; }
	[[ "$(unset GITNIBLET_LEAKCHECK; fLeakLine)" == "Leak checks: on" ]]  ||  { echo "  default isn't on"; bad=1; }
	[[ "$(unset GITNIBLET_LEAKCHECK; fLeakLine --no-leakcheck)" == "Leak checks: off" ]]  ||  { echo "  --no-leakcheck isn't off"; bad=1; }
	[[ "$(GITNIBLET_LEAKCHECK=0 fLeakLine)" == "Leak checks: off" ]]  ||  { echo "  GITNIBLET_LEAKCHECK=0 isn't off"; bad=1; }
	[[ "$(GITNIBLET_LEAKCHECK=0 fLeakLine --leakcheck)" == "Leak checks: on" ]]  ||  { echo "  --leakcheck doesn't win over the env"; bad=1; }
	return "${bad}"
}
fCheck defqc "--no-leakcheck and GITNIBLET_LEAKCHECK=0 turn the leak checks off, and --leakcheck back on" fLeakFlags

fAreas(){
	local out bad=0 area
	out="$(bash "${cicd}" --list --backlog-run-quick cli 2>&1 | grep -E '^[0-9a-z]{5} ')"
	[[ -n "${out}" ]]  &&  ! grep -qv ' internal/cli ' <<< "${out}"  ||  { echo "  cli listed: $(grep -v ' internal/cli ' <<< "${out}" | head -3)"; bad=1; }
	[[ "$(bash "${cicd}" --list --backlog-run-quick 2>&1 | grep -cE '^[0-9a-z]{5} ')" -gt "$(wc -l <<< "${out}")" ]]  ||  bad=1
	! bash "${cicd}" --list --backlog-run-quick nosucharea > /dev/null 2>&1  ||  { echo "  an unknown area passed"; bad=1; }
	out="$(bash "${cicd}" --help 2>&1)"
	for area in cli tui web core config crud ddl schema store txlog seams; do
		grep -qE "^ +.*\\b${area}\\b" <<< "${out}"  ||  { echo "  --help lacks ${area}"; bad=1; }
	done
	return "${bad}"
}
fCheck deecg "areas narrow the tests --list shows, an unknown one fails, and --help names each" fAreas

## A scratch repo: main at a code change, then a docs-only change on top.
repo="${runDir}/repo"
git init -q -b main "${repo}"  &&  cd "${repo}"  &&  {
	git config user.email t@example.invalid; git config user.name t
	echo 'package x' > x.go; git add x.go; git commit -qm a
	echo doc > README.md; mkdir -p assets project; echo art > assets/a.png; echo d > project/design.md; git add README.md assets/a.png project/design.md; git commit -qm b
	echo 'package x // y' > x.go; echo e > project/example.shcl; git add x.go project/example.shcl; git commit -qm c
}  ||  { echo "FAIL deech can't make the scratch repo"; exit 1; }
first="$(git rev-parse HEAD~2)"; docs="$(git rev-parse HEAD~1)"; code="$(git rev-parse HEAD)"
zero=0000000000000000000000000000000000000000
fHook(){ printf '%s\n' "$2" | GITNIBLET_GATE_DRYRUN=1 SKIP_GATE="${1}" bash "${hook}" 2>&1; }
fHookCases(){
	local bad=0 out
	out="$(fHook 0 "refs/heads/dev ${code} refs/heads/dev ${first}")";   [[ -z "${out}" ]]                 ||  { echo "  dev: ${out}"; bad=1; }
	out="$(fHook 0 "refs/heads/main ${docs} refs/heads/main ${first}")"; [[ "${out}" == *"no gate"* ]]     ||  { echo "  docs: ${out}"; bad=1; }
	out="$(fHook 0 "refs/heads/main ${code} refs/heads/main ${docs}")";  [[ "${out}" == *"gating"*"for x.go"* || "${out}" == *"gating"*"for project/example.shcl"* ]]  ||  { echo "  code: ${out}"; bad=1; }
	out="$(fHook 0 "refs/heads/main ${code} refs/heads/main ${zero}")";  [[ "${out}" == *"gating"*"(a new main)"* ]]  ||  { echo "  new: ${out}"; bad=1; }
	out="$(fHook 0 "(delete) ${zero} refs/heads/main ${code}")";         [[ -z "${out}" ]]                 ||  { echo "  delete: ${out}"; bad=1; }
	out="$(fHook 1 "refs/heads/main ${code} refs/heads/main ${docs}")";  [[ -z "${out}" ]]                 ||  { echo "  SKIP_GATE: ${out}"; bad=1; }
	return "${bad}"
}
fCheck deech "pre-push gates main only, skips docs and art, and gates project/example.shcl" fHookCases

## The scratch repo's only code change is example.shcl, which stays gated.
fShclGated(){
	local out
	git -C "${repo}" checkout -q -b s "${docs}"  &&  echo e2 > "${repo}/project/example.shcl"  &&  git -C "${repo}" add project/example.shcl  &&  git -C "${repo}" commit -qm s  ||  return 1
	out="$(fHook 0 "refs/heads/s $(git -C "${repo}" rev-parse HEAD) refs/heads/main ${docs}")"
	[[ "${out}" == *"gating"*"for project/example.shcl"* ]]  ||  { echo "  ${out}"; return 1; }
}
fCheck deecj "a push to main of project/example.shcl alone is gated" fShclGated

## The gate for real, with a stand-in cicd.bash committed in the scratch repo:
## it runs in a worktree of the pushed commit, its exit is the push's, and the
## worktree and run folder go afterwards.
fHookRuns(){
	local out rc sha bad=0 tree
	mkdir -p "${repo}/cicd"
	printf '#!/bin/bash\ncd "$(dirname "$0")"\necho "args $* in $(pwd -P) at $(git rev-parse HEAD)" >> "%s"\nexit "$(cat "%s")"\n' "${runDir}/stub.log" "${runDir}/stub.rc" > "${repo}/cicd/cicd.bash"
	git -C "${repo}" add cicd/cicd.bash  &&  git -C "${repo}" commit -qm stub  ||  return 1
	sha="$(git -C "${repo}" rev-parse HEAD)"
	for rc in 0 1; do
		echo "${rc}" > "${runDir}/stub.rc"
		printf '%s\n' "refs/heads/s ${sha} refs/heads/main ${docs}" | SKIP_GATE=0 bash "${hook}" > /dev/null 2>&1
		[[ "$?" == "${rc}" ]]  ||  { echo "  stub exit ${rc} didn't carry"; bad=1; }
	done
	out="$(cat "${runDir}/stub.log")"
	[[ "$(grep -c "^args --merge-gate in ${HOST_TEMP}/${tempProject}/[0-9_-]*/tree/cicd at ${sha}\$" <<< "${out}")" == 2 ]]  ||  { echo "  ${out}"; bad=1; }
	tree="$(sed -n '1s/.* in \(.*\)\/cicd at .*/\1/p' <<< "${out}")"
	[[ -n "${tree}" && ! -e "${tree%/tree}" ]]  ||  { echo "  left ${tree%/tree}"; bad=1; }
	[[ "$(git -C "${repo}" worktree list | wc -l)" == 1 ]]  ||  { git -C "${repo}" worktree list | sed 's/^/  /'; bad=1; }
	return "${bad}"
}
fCheck deeck "pre-push runs the gate in a worktree of the pushed commit, passes on its exit, and cleans up" fHookRuns

((fails == 0))  ||  { echo "flags-test.bash: ${fails} failed"; exit 1; }
echo "flags-test.bash: PASS"


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: Race tests and Pipeline tests in the plans.
##		- 20261025 JC: A stand-in hooks folder, so the plans match on a hosted runner.
##		- 20261026 JC: The leak flags turn the leak checks on and off.
