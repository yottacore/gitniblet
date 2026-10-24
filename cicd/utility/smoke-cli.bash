#!/usr/bin/env bash

##	Purpose:
##		Smoke-test a built ngdb binary end to end: version, usage, register the
##		issue-tracker demo, write a few rows, read them back, and rebuild the view
##		from the log. Runs on stock macOS bash 3.2 too, for the macOS CI runner.
##	Syntax:
##		smoke-cli.bash [path to ngdb]     ## default: bin/ngdb
##	Notes:
##		Works in its own mktemp folder with its own registry (--config), and
##		removes only that folder on exit.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	SPDX-License-Identifier: AGPL-3.0-only

set -Eeuo pipefail

dirPath_Base="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exe="${1:-${dirPath_Base}/bin/ngdb}"
[[ "${exe}" == /* ]] || exe="${PWD}/${exe}"
[[ -x "${exe}" ]] || { echo "smoke-cli: no binary at ${exe}" >&2; exit 1; }

tmpRoot="${TMPDIR:-/tmp}"; tmpRoot="${tmpRoot%/}"
work="$(mktemp -d "${tmpRoot}/ngdb-smoke.XXXXXX")"
fCleanup(){
	## Only the folder mktemp made above, never a link or anything outside it.
	if [[ -n "${work}" && "${work}" == "${tmpRoot}"/ngdb-smoke.* && -d "${work}" && ! -L "${work}" ]]; then
		rm -rf -- "${work}"
	fi
}
trap fCleanup EXIT

fail(){ echo "smoke-cli: FAIL: $*" >&2; exit 1; }
ngdb(){ "${exe}" --config "${work}/registry" "$@"; }

export NANOGITDB_USER=smoke NANOGITDB_HOST=ci

ver="$("${exe}" --version)"
echo "${ver}"
[[ "${ver}" == "ngdb "* ]] || fail "--version: ${ver}"
## A non-release build (cicd/build.bash) must show its build date.
if [[ -n "${NGDB_SMOKE_EXPECT_DATE:-}" ]]; then
	grep -Eq ' \([0-9]{4}-[0-9]{2}-[0-9]{2}\)$' <<< "${ver}" || fail "--version has no build date: ${ver}"
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
