#!/usr/bin/env bash

##	Purpose:
##		Smoke-test a built gitniblet binary end to end: version, usage, register the
##		small issue-tracker schema in smoke/, write a few rows, read them back, and
##		rebuild the view from the log. Runs on stock macOS bash 3.2 too, for the
##		macOS CI runner.
##	Syntax:
##		smoke-cli.bash [path to gitniblet]     ## default: bin/gitniblet
##	Notes:
##		Works in its own temp folder with its own registry (--config), inside
##		the cicd run's folder when there is one, and removes it on exit through
##		the guard in include/temp-run.bash. HOME and the config folders point
##		there too, so nothing reaches the real registry.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	SPDX-License-Identifier: AGPL-3.0-only

set -Eeuo pipefail

dirPath_Base="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exe="${1:-${dirPath_Base}/bin/gitniblet}"
[[ "${exe}" == /* ]] || exe="${PWD}/${exe}"
[[ -x "${exe}" ]] || { echo "smoke-cli: no binary at ${exe}" >&2; exit 1; }

# shellcheck source=include/temp-run.bash
source "$(dirname "${BASH_SOURCE[0]}")/include/temp-run.bash"
work="$(fTempDir_Make smoke)"
trap 'fSafeRm "${work}" > /dev/null' EXIT
## --config covers the data verbs. This covers the rest, and the next one added.
fConfigHome_Use "${work}/fake"

fail(){ echo "smoke-cli: FAIL: $*" >&2; exit 1; }
gitniblet(){ "${exe}" --config "${work}/registry" "$@"; }

export GITNIBLET_USER=smoke GITNIBLET_HOST=ci

out="$("${exe}" --version)"
echo "${out}"
ver="${out%%$'\n'*}"
[[ "${ver}" == "gitniblet v"* ]] || fail "--version: ${ver}"
[[ "${out}" == *$'\n'"Copyright © "* ]] || fail "--version has no copyright line"
## A non-release build (cicd/build.bash) must show its build number and date.
if [[ -n "${GITNIBLET_SMOKE_EXPECT_DATE:-}" ]]; then
	grep -Eq '^gitniblet v[^ ]+ build [0-9a-hjkmnp-tv-z]+ \([0-9]{4}-[0-9]{2}-[0-9]{2}\)$' <<< "${ver}" || fail "--version has no build number and date: ${ver}"
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
grep -q 'usage: gitniblet' "${work}/help.txt" || fail "--help printed no usage"
if "${exe}" no-such-verb > /dev/null 2>&1; then fail "unknown verb exited 0"; fi

cp "${dirPath_Base}/cicd/utility/smoke/issues.shcl" "${work}/"
cd "${work}"
gitniblet --init . > /dev/null || fail "--init"

gitniblet create issues person screen_name=alex full_name="Alex Kim" role=maintainer active=true > /dev/null || fail "create person"
gitniblet create issues component name=web lead=alex > /dev/null || fail "create component"
id="$(gitniblet create issues issue ref=NGD-1 title="Smoke row" status=open priority=high component=web assignee=alex opened="2026-07-10 09:00" | tail -1)"
[[ -n "${id}" ]] || fail "create issue printed no id"

gitniblet get issues issue "${id}" | grep -q 'Smoke row' || fail "get did not return the row"
gitniblet update issues issue "${id}" title="Smoke row 2" > /dev/null || fail "update"
gitniblet replay issues > /dev/null || fail "replay"
gitniblet query issues "SELECT title FROM issue WHERE id = id('${id}')" | grep -q 'Smoke row 2' || fail "query after replay"

fConfigHome_Check "${work}/fake" || fail "the default config folder was used"

echo "smoke-cli: PASS"
