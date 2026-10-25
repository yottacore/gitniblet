#!/usr/bin/env bash

##	Purpose:
##		Smoke-test a built ngdb binary end to end: version, usage, register the
##		issue-tracker demo, write a few rows, read them back, and rebuild the view
##		from the log. Runs on stock macOS bash 3.2 too, for the macOS CI runner.
##	Syntax:
##		smoke-cli.bash [path to ngdb]     ## default: bin/ngdb
##	Notes:
##		Works in its own temp folder with its own registry (--config), inside
##		the cicd run's folder when there is one, and removes it on exit through
##		the guard in include/temp-run.bash.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	SPDX-License-Identifier: AGPL-3.0-only

set -Eeuo pipefail

dirPath_Base="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exe="${1:-${dirPath_Base}/bin/ngdb}"
[[ "${exe}" == /* ]] || exe="${PWD}/${exe}"
[[ -x "${exe}" ]] || { echo "smoke-cli: no binary at ${exe}" >&2; exit 1; }

# shellcheck source=include/temp-run.bash
source "$(dirname "${BASH_SOURCE[0]}")/include/temp-run.bash"
work="$(fTempDir_Make smoke)"
trap 'fSafeRm "${work}" > /dev/null' EXIT

fail(){ echo "smoke-cli: FAIL: $*" >&2; exit 1; }
ngdb(){ "${exe}" --config "${work}/registry" "$@"; }

export NANOGITDB_USER=smoke NANOGITDB_HOST=ci

out="$("${exe}" --version)"
echo "${out}"
ver="${out%%$'\n'*}"
[[ "${ver}" == "ngdb v"* ]] || fail "--version: ${ver}"
[[ "${out}" == *$'\n'"Copyright © "* ]] || fail "--version has no copyright line"
## A non-release build (cicd/build.bash) must show its build number and date.
if [[ -n "${NGDB_SMOKE_EXPECT_DATE:-}" ]]; then
	grep -Eq '^ngdb v[^ ]+ build [0-9a-hjkmnp-tv-z]+ \([0-9]{4}-[0-9]{2}-[0-9]{2}\)$' <<< "${ver}" || fail "--version has no build number and date: ${ver}"
	## Built from this checkout: the number is its HEAD commit's, the same as the release notes give.
	if [[ "$(git -C "${dirPath_Base}" rev-parse --show-toplevel 2> /dev/null)" == "${dirPath_Base}" ]]; then
		# shellcheck source=include/build-number.bash
		source "$(dirname "${BASH_SOURCE[0]}")/include/build-number.bash"
		want="$(cd "${dirPath_Base}" && fBuildNumber)" || fail "no build number for HEAD"
		[[ "${ver}" == *" build ${want} ("* ]] || fail "--version build number isn't HEAD's ${want}: ${ver}"
		## install.bash reads the installed version off the same line. It runs its
		## main when sourced, so take just the function.
		eval "$(sed -n '/^fInstalledVersion(){/,/^}/p' "${dirPath_Base}/install.bash")"
		srcVer="$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "${dirPath_Base}/source/app/app.go")"
		got="$(fInstalledVersion "${exe}")"
		[[ -n "${srcVer}" && "${got}" == "${srcVer}" ]] || fail "install.bash reads the version as '${got}', not '${srcVer}'"
	fi
fi

"${exe}" --help > "${work}/help.txt" || fail "--help exited non-zero"
grep -q 'usage: ngdb' "${work}/help.txt" || fail "--help printed no usage"
if "${exe}" no-such-verb > /dev/null 2>&1; then fail "unknown verb exited 0"; fi

cp "${dirPath_Base}/demos/t7em6_issue-tracker/issues.shcl" "${work}/"
cd "${work}"
ngdb --init . > /dev/null || fail "--init"

ngdb create issues person screen_name=alex full_name="Alex Kim" role=maintainer active=true > /dev/null || fail "create person"
ngdb create issues component name=web lead=alex > /dev/null || fail "create component"
id="$(ngdb create issues issue ref=NGD-1 title="Smoke row" status=open priority=high component=web assignee=alex opened="2026-07-10 09:00" | tail -1)"
[[ -n "${id}" ]] || fail "create issue printed no id"

ngdb get issues issue "${id}" | grep -q 'Smoke row' || fail "get did not return the row"
ngdb update issues issue "${id}" title="Smoke row 2" > /dev/null || fail "update"
ngdb replay issues > /dev/null || fail "replay"
ngdb query issues "SELECT title FROM issue WHERE id = id('${id}')" | grep -q 'Smoke row 2' || fail "query after replay"

echo "smoke-cli: PASS"
