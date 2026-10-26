#!/bin/bash
#  shellcheck disable=2034  ## 'appears unused.' syncWhy is read by the script that sources this.

##	Purpose:
##		Remote sync for cicd.bash, before anything is built, per the shared CICD
##		rules. Publish pulls too, but only after the tests, so a change merged
##		upstream meanwhile would go out tested by nothing.
##		- fRemoteSync [PATH...]: run from the repo. Fetches, fast-forwards when
##		  only behind, and stops when diverged. No upstream, or a remote that
##		  doesn't answer, warns and goes on, since the tree here still builds.
##		  PATHs are what the caller has already read, such as cicd.bash itself,
##		  so a fast-forward that changes one stops the run too.
##		  Returns 0 to go on, or 1 with syncWhy set.
##		Uses the caller's fEcho_Clean.
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

declare syncWhy=""

fRemoteSync(){
	local up counts behind ahead before
	syncWhy=""
	if ! up="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2> /dev/null)"; then
		fEcho_Clean "WARNING: this branch has no upstream, so there's nothing to sync."
		return 0
	fi
	## No prompt and a time limit, so an unattended run can't sit on a password or a dead link.
	if ! GIT_TERMINAL_PROMPT=0 timeout "${GITNIBLET_SYNC_TIMEOUT:-60}" git fetch --quiet 2> /dev/null; then
		fEcho_Clean "WARNING: couldn't fetch ${up}, so this run builds the tree as it is."
		return 0
	fi
	counts="$(git rev-list --left-right --count "${up}...HEAD")"
	read -r behind ahead <<< "${counts}"
	if ! [[ "${behind:-}" =~ ^[0-9]+$ && "${ahead:-}" =~ ^[0-9]+$ ]]; then
		syncWhy="couldn't compare with ${up}."; return 1
	fi
	if ((behind == 0)); then
		fEcho_Clean "Up to date with ${up}, ${ahead} to publish."
		return 0
	fi
	if ((ahead > 0)); then
		syncWhy="diverged from ${up}, ${ahead} here and ${behind} there. Reconcile them, or give --no-sync."; return 1
	fi
	before="$(git rev-parse HEAD)"
	fEcho_Clean "Fast-forwarding ${behind} from ${up}."
	## --autostash carries uncommitted edits over the fast-forward and puts them back.
	git merge --ff-only --autostash --quiet "${up}"  ||  { syncWhy="the fast-forward from ${up} failed."; return 1; }
	## A clash with the stashed edits still returns 0, with conflict markers in the tree.
	if [[ -n "$(git ls-files -u)" ]]; then
		syncWhy="uncommitted edits clash with what came from ${up}. They're in the stash too. Sort out the conflicts and run again."; return 1
	fi
	if (($#))  &&  ! git diff --quiet "${before}" HEAD -- "$@"; then
		syncWhy="the fast-forward changed what this run had already read ($*). Run it again."; return 1
	fi
	return 0
}
