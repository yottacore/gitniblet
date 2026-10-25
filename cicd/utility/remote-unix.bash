#!/usr/bin/env bash

##	Purpose:
##		The test box's side of remote-test.bash, on FreeBSD, macOS and Linux.
##		Runs each Go test binary from its package folder, then the CLI smoke on
##		the cross-built ngdb, and removes its work folder after.
##	Syntax:
##		remote-unix.bash <work folder> <most jobs> <keep: 0|1>
##	Notes:
##		Stock bash 3.2 on macOS runs this, so no bash 4 features.
##	History: At bottom of this file.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Euo pipefail

work="${1:?work folder}"; jobCap="${2:-4}"; keep="${3:-0}"

## Only the folder remote-test.bash made for this run, never a link.
fCleanup(){
	cd / || return
	if [[ "${keep}" == "1" ]]; then echo "kept $(hostname):${work}"; return; fi
	if [[ "${work}" =~ ^/[A-Za-z0-9._/-]+/nano-git-db-floss/[0-9]{8}-[0-9]{6}_[0-9]{3}$ && -d "${work}" && ! -L "${work}" ]]; then
		if [[ "$(uname -s)" == "Linux" ]]; then rm -rf --one-file-system -- "${work}"; else rm -rf -x -- "${work}"; fi
	else
		echo "remote-unix: left ${work}, not a run folder" >&2
	fi
}
trap fCleanup EXIT

cd "${work}" || exit 1
mkdir -p tmp logs
export TMPDIR="${work}/tmp"

## Half the cores, a quarter when busy, never past the caller's cap.
cores="$(nproc 2> /dev/null || sysctl -n hw.ncpu 2> /dev/null || echo 2)"
load="$(cut -d' ' -f1 /proc/loadavg 2> /dev/null || sysctl -n vm.loadavg 2> /dev/null | tr -d '{}' | awk '{print $1}')"
jobs=$((cores / 2))
if awk -v l="${load:-0}" -v c="${cores}" 'BEGIN { exit !(l > c / 2) }'; then jobs=$((cores / 4)); fi
((jobs <= jobCap)) || jobs="${jobCap}"
((jobs >= 1)) || jobs=1
export GOMAXPROCS="${jobs}"
echo "$(uname -sm) on $(hostname): ${cores} cores, load ${load:-?}, ${jobs} jobs"

failed=0
while read -r name dir; do
	[[ -n "${name}" ]] || continue
	start="$(date +%s)"
	if (cd "${work}/${dir}" && nice -n 19 "${work}/tests/${name}.test" -test.timeout 15m > "${work}/logs/${name}.log" 2>&1); then
		echo "PASS ${name} ($(($(date +%s) - start))s)"
	else
		echo "FAIL ${name} ($(($(date +%s) - start))s)"
		tail -n 40 "${work}/logs/${name}.log"
		failed=$((failed + 1))
	fi
done < "${work}/tests/manifest"

if NGDB_SMOKE_EXPECT_DATE=1 nice -n 19 bash "${work}/cicd/utility/smoke-cli.bash" "${work}/ngdb" > "${work}/logs/smoke.log" 2>&1; then
	echo "PASS smoke-cli"
else
	echo "FAIL smoke-cli"
	tail -n 20 "${work}/logs/smoke.log"
	failed=$((failed + 1))
fi

((failed == 0))


##	History:
##		- 20261025 JC: Created.
