#!/usr/bin/env bash
#  shellcheck disable=2329  ## 'This function is never invoked.' Each is called through fCheck.
#  shellcheck disable=2015  ## 'A && B || C is not if-then-else.' C is only the note on a failed check.

##	Purpose:
##		Checks include/sync.bash's remote sync against scratch repos, each with
##		a bare origin and a second clone that pushes to it, and that cicd.bash
##		plans it first, off with --no-sync and in the hands-off modes.
##	Syntax:
##		sync-test.bash     ## about 5 seconds
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
cicd="${repoRoot}/cicd/cicd.bash"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
# shellcheck source=include/sync.bash
source "${repoRoot}/cicd/utility/include/sync.bash"
runDir="$(fTempDir_Make sync-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT

fEcho_Clean(){ echo "$*"; }
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
declare -i fails=0
fCheck(){  ## id, name, then a command that passes or not. A subshell, so set -u stops only that check.
	local id="$1" name="$2"; shift 2
	if ( "$@" ); then echo "PASS ${id} ${name}"; else echo "FAIL ${id} ${name}"; fails+=1; fi
}

## <name>: origin.git, work (the run's clone) and other, each at one commit.
fRepos(){
	local d="${runDir}/$1"
	git init -q --bare -b main "${d}/origin.git"  &&  git init -q -b main "${d}/work"  &&  cd "${d}/work"  ||  return 1
	git config user.email t@example.invalid; git config user.name t
	printf '1\n2\n3\n4\n5\n' > a.txt  &&  git add a.txt  &&  git commit -qm a  &&  git remote add origin "${d}/origin.git"  &&  git push -q -u origin main 2> /dev/null  ||  return 1
	git clone -q "${d}/origin.git" "${d}/other"  ||  return 1
	git -C "${d}/other" config user.email t@example.invalid; git -C "${d}/other" config user.name t
}
## Commits <file> in other and pushes it, so work is behind.
fUpstream(){ (cd "${runDir}/$1/other"  &&  mkdir -p "$(dirname "$2")"  &&  echo "$3" > "$2"  &&  git add "$2"  &&  git commit -qm u  &&  git push -q origin main 2> /dev/null); }

fNoUpstream(){
	local out head
	fRepos noup  &&  git switch -q -c loner  ||  return 1
	head="$(git rev-parse HEAD)"
	out="$(fRemoteSync)"  ||  { echo "  returned 1: ${syncWhy}"; return 1; }
	[[ "${out}" == "WARNING: "*"no upstream"* && "$(git rev-parse HEAD)" == "${head}" ]]  ||  { echo "  ${out}"; return 1; }
}
fCheck deeea "a branch with no upstream warns and goes on" fNoUpstream

fUpToDate(){
	local out
	fRepos same  &&  echo b > b.txt  &&  git add b.txt  &&  git commit -qm b  ||  return 1
	out="$(fRemoteSync)"  &&  [[ "${out}" == "Up to date with origin/main, 1 to publish." ]]  ||  { echo "  ${out} ${syncWhy}"; return 1; }
}
fCheck deeeb "up to date says so and goes on, with the count to publish" fUpToDate

## The edit is to a file the fast-forward changes too, so it only goes through stashed.
fBehind(){
	local out
	fRepos behind  &&  fUpstream behind a.txt $'1\n2\n3\n4\nup'  &&  printf 'edit\n2\n3\n4\n5\n' > a.txt  ||  return 1
	out="$(fRemoteSync 2>&1)"  ||  { echo "  returned 1: ${syncWhy}"; return 1; }
	[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" && "$(< a.txt)" == $'edit\n2\n3\n4\nup' && "${out}" == *"Fast-forwarding 1 from origin/main."* ]] \
		||  { echo "  ${out}"; git status --short; return 1; }
}
fCheck deeec "behind fast-forwards and keeps uncommitted edits" fBehind

fClash(){
	fRepos clash  &&  fUpstream clash a.txt $'1\n2\n3\n4\nup'  &&  printf '1\n2\n3\n4\nedit\n' > a.txt  ||  return 1
	if fRemoteSync > /dev/null 2>&1; then echo "  went on"; return 1; fi
	[[ "${syncWhy}" == "uncommitted edits clash"* && -n "$(git stash list)" ]]  ||  { echo "  ${syncWhy}"; return 1; }
}
fCheck deeeh "uncommitted edits that clash with the fast-forward stop the run" fClash

fDiverged(){
	local head
	fRepos div  &&  fUpstream div u.txt up  &&  echo b > b.txt  &&  git add b.txt  &&  git commit -qm b  ||  return 1
	head="$(git rev-parse HEAD)"
	if fRemoteSync > /dev/null; then echo "  went on"; return 1; fi
	[[ "${syncWhy}" == "diverged from origin/main, 1 here and 1 there."* && "$(git rev-parse HEAD)" == "${head}" ]]  ||  { echo "  ${syncWhy}"; return 1; }
}
fCheck deeed "diverged stops, and leaves HEAD where it was" fDiverged

fOffline(){
	local out head
	fRepos off  &&  fUpstream off u.txt up  &&  git remote set-url origin "${runDir}/off/gone.git"  ||  return 1
	head="$(git rev-parse HEAD)"
	out="$(fRemoteSync)"  ||  { echo "  returned 1: ${syncWhy}"; return 1; }
	[[ "${out}" == "WARNING: couldn't fetch origin/main"* && "$(git rev-parse HEAD)" == "${head}" ]]  ||  { echo "  ${out}"; return 1; }
}
fCheck deeee "a remote that can't be fetched warns and goes on" fOffline

## A fast-forward that changes a path the caller already read stops the run.
## One that doesn't goes on.
fLoaded(){
	fRepos loaded  &&  fUpstream loaded cicd/include/x.bash new  ||  return 1
	if fRemoteSync cicd/cicd.bash cicd/include > /dev/null; then echo "  went on"; return 1; fi
	[[ "${syncWhy}" == *"Run it again." && "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]]  ||  { echo "  ${syncWhy}"; return 1; }
	fUpstream loaded source/y.go new  &&  fRemoteSync cicd/cicd.bash cicd/include > /dev/null  ||  { echo "  stopped on another path: ${syncWhy}"; return 1; }
}
fCheck deeef "a fast-forward that changes what the run already read stops it" fLoaded

fPlanned(){
	local bad=0 mode
	[[ "$(bash "${cicd}" --list 2>&1 | grep -m1 '^Step ')" == "Step 1: Remote sync" ]]  ||  { echo "  not first by default"; bad=1; }
	[[ "$(bash "${cicd}" --list --quick 2>&1 | grep -m1 '^Step ')" == "Step 1: Remote sync" ]]  ||  { echo "  not in --quick"; bad=1; }
	for mode in --no-sync --backlog-run-quick --backlog-run-full --merge-gate; do
		grep -qxF "Off: Remote sync (${mode})" <<< "$(bash "${cicd}" --list "${mode}" 2>&1)"  ||  { echo "  on with ${mode}"; bad=1; }
	done
	return "${bad}"
}
fCheck deeeg "cicd.bash syncs first, and not with --no-sync or a hands-off mode" fPlanned

## cicd.bash for real, in a scratch repo of just cicd/, whose upstream is an
## unrelated commit. It stops at step 1, then with an upstream at its own
## commit it passes step 1 and goes on to the build, which has nothing to build.
fWholeRun(){
	local d="${runDir}/whole" out orphan
	local -a flags=(-q --quick --no-container --no-remote --no-install --no-hooks --no-publish --no-demo --no-format)
	mkdir -p "${d}/work/source"  &&  touch "${d}/work/source/go.mod"  ||  return 1
	{ git -C "${repoRoot}" ls-files cicd; echo cicd/utility/include/sync.bash; } | sort -u | tar -C "${repoRoot}" -cf - -T - | tar -C "${d}/work" -xf -  ||  return 1
	git init -q --bare -b main "${d}/up.git"  &&  cd "${d}/work"  &&  git init -q -b main  ||  return 1
	git config user.email t@example.invalid; git config user.name t
	git add -A  &&  git commit -qm a  &&  git remote add origin "${d}/up.git"  ||  return 1
	orphan="$(git commit-tree -m o "$(git mktree < /dev/null)")"  &&  git push -q origin "${orphan}:refs/heads/main" 2> /dev/null  &&  git fetch -q  &&  git branch -q -u origin/main  ||  return 1
	out="$(bash cicd/cicd.bash "${flags[@]}" 2>&1)"  &&  { echo "  passed when diverged"; return 1; }
	grep -qE ': Step 1 of [0-9]+; FAIL Remote sync \]$' <<< "${out}"  &&  grep -q 'Remote sync: diverged from origin/main, 1 here and 1 there' <<< "${out}"  ||  { tail -5 <<< "${out}"; return 1; }
	git push -q origin HEAD:refs/heads/same 2> /dev/null  &&  git fetch -q  &&  git branch -q -u origin/same  ||  return 1
	out="$(bash cicd/cicd.bash "${flags[@]}" 2>&1)"
	grep -qE ': Step 1 of [0-9]+; PASS Remote sync \]$' <<< "${out}"  &&  grep -qE ': Step 2 of [0-9]+; Starting ' <<< "${out}"  ||  { tail -5 <<< "${out}"; return 1; }
}
fCheck deeei "cicd.bash stops at step 1 when diverged, and goes on when up to date" fWholeRun

((fails == 0))
