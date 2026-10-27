#!/usr/bin/env bash

##	Purpose:
##		Checks 2 goreleaser builds of one commit give the same bytes. Builds
##		HEAD twice, from 2 fresh clones checked out at different times, with
##		the working tree's .goreleaser.yaml cut down to linux and windows on
##		amd64. Then compares checksums.txt, which covers the archives and the
##		.deb and .rpm, and the binary's mtime against the commit time.
##	Syntax:
##		repro-test.bash     ## about a minute with a warm build cache
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
# shellcheck source=include/tool-versions.bash
source "${repoRoot}/cicd/utility/include/tool-versions.bash"
# shellcheck source=include/cpu-limit.bash
source "${repoRoot}/cicd/utility/include/cpu-limit.bash"
runDir="$(fTempDir_Make repro-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT
declare -i fails=0

fBuild(){  ## n
	local tree="${runDir}/tree${1}"
	git clone -q --shared "${repoRoot}" "${tree}"  ||  return 1
	sed -e 's/^    goos: \[.*\]/    goos: [linux, windows]/' -e 's/\[amd64, arm64\]/[amd64]/' "${repoRoot}/.goreleaser.yaml" > "${runDir}/goreleaser${1}.yaml"
	printf 'dist: %s\n' "${runDir}/dist${1}" >> "${runDir}/goreleaser${1}.yaml"
	( cd "${tree}"  &&  GOFLAGS='' GITNIBLET_BUILD=0 go run "${GITNIBLET_GORELEASER}" release --snapshot --clean \
		--parallelism "${GITNIBLET_JOBS}" --config "${runDir}/goreleaser${1}.yaml" ) > "${runDir}/build${1}.log" 2>&1
}
fBuild 1  &&  sleep 2  &&  fBuild 2
rc=$?
if ((rc)); then echo "FAIL defr0 2 builds of one commit give the same bytes: a build failed"; tail -20 "${runDir}"/build*.log; exit 1; fi

if [[ -s "${runDir}/dist1/checksums.txt" ]] && cmp -s "${runDir}/dist1/checksums.txt" "${runDir}/dist2/checksums.txt"
	then echo "PASS defr0 2 builds of one commit give the same bytes"
	else echo "FAIL defr0 2 builds of one commit give the same bytes"; diff "${runDir}/dist1/checksums.txt" "${runDir}/dist2/checksums.txt"; fails+=1; fi

commitTime="$(git -C "${repoRoot}" log -1 --format=%ct)"
bin="$(find "${runDir}/dist1" -type f -name gitniblet -path '*linux_amd64*' | head -1)"
if [[ -n "${bin}" && "$(stat -c %Y "${bin}")" == "${commitTime}" ]]
	then echo "PASS defr1 the binary's mtime is the commit time"
	else echo "FAIL defr1 the binary's mtime is the commit time: ${bin:-no binary}"; fails+=1; fi

exit $((fails > 0))


##	History:
##		- 20261026 JC: Created.
