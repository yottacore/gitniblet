#!/usr/bin/env bash

##	Purpose:
##		CodeQL over the module and vendor/, from the bundle pinned in
##		include/tool-versions.bash. The results go to the lint artifacts as
##		codeql_<YYYYmmDD-HHMMSS>.sarif, and codeql-check fails on any result
##		that isn't in codeql-accepted.txt: every one in our own code, and the
##		security ones in vendor/.
##		The bundle is fetched once, checked by sha256, and kept in
##		GITNIBLET_CODEQL_CACHE, else ~/.cache/gitniblet. Linux x86_64 only.
##		The CodeQL terms allow this for an open source repo on GitHub.com, as
##		this one is. Not for enterprise or vendor, which are private.
##	Syntax:
##		codeql.bash [--out DIR]   ## about 2 minutes, plus the first fetch
##		  --out DIR   where the SARIF goes (default GITNIBLET_LOG_DIR, else cicd/artifacts/lint)
##	Exit: 0 nothing new, 1 a new result or a stale accepted entry, 2 couldn't run.
##	History: At bottom of script.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


set -Eeuo pipefail

meDir="$(realpath -e "$(dirname "$0")")"
repoRoot="$(realpath -e "${meDir}/../..")"
outDir="${GITNIBLET_LOG_DIR:-${repoRoot}/cicd/artifacts/lint}"
while (($#)); do case "$1" in
	--out)      outDir="${2:?--out needs a folder}"; shift 2 ;;
	--out=*)    outDir="${1#*=}"; shift ;;
	-h|--help)  grep -E '^##' "$0" | sed 's/^##\t\?//'; exit 0 ;;
	*) echo "codeql.bash: unknown option: $1" >&2; exit 2 ;;
esac; done

# shellcheck source=include/cpu-limit.bash
source "${meDir}/include/cpu-limit.bash"
# shellcheck source=include/tool-versions.bash
source "${meDir}/include/tool-versions.bash"
# shellcheck source=include/temp-run.bash
source "${meDir}/include/temp-run.bash"
# shellcheck source=include/gfs-rotate.bash
source "${meDir}/include/gfs-rotate.bash"
fCpuLimit heavy

fDie(){ echo "codeql.bash: ${1}" >&2; exit 2; }
[[ "$(uname -s)-$(uname -m)" == "Linux-x86_64" ]]  ||  fDie "the pinned bundle is Linux x86_64 only."

workDir="$(fTempDir_Make codeql)"
trap 'fSafeRm "${workDir}" > /dev/null 2>&1 || true' EXIT

## Unpacked in the temp folder, then moved beside the cache and renamed into
## place, so a fetch cut short never looks like a bundle.
cacheDir="${GITNIBLET_CODEQL_CACHE:-${XDG_CACHE_HOME:-${HOME}/.cache}/gitniblet}/codeql-${GITNIBLET_CODEQL_VER}"
codeql="${cacheDir}/codeql/codeql"
if [[ ! -x "${codeql}" ]]; then
	command -v zstd > /dev/null  ||  fDie "no zstd, to unpack the bundle."
	echo "Fetching the CodeQL ${GITNIBLET_CODEQL_VER} Go bundle"
	curl -fsSLo "${workDir}/bundle.tar.zst" \
		"https://github.com/github/codeql-action/releases/download/codeql-bundle-v${GITNIBLET_CODEQL_VER}/codeql-bundle-go-linux64.tar.zst"
	echo "${GITNIBLET_CODEQL_SHA256}  ${workDir}/bundle.tar.zst" | sha256sum -c --quiet -  ||  fDie "the bundle's sha256 isn't the pinned one."
	mkdir -p "${workDir}/bundle"  &&  tar --zstd -xf "${workDir}/bundle.tar.zst" -C "${workDir}/bundle"
	mkdir -p "${cacheDir%/*}"
	mv -T "${workDir}/bundle" "${cacheDir}.part.$$"
	mv -T "${cacheDir}.part.$$" "${cacheDir}"  ||  [[ -x "${codeql}" ]]  ||  fDie "couldn't move the bundle into ${cacheDir}."
fi
"${codeql}" version --format=terse

mkdir -p "${outDir}"
gfs_rotate "${outDir}" codeql sarif > /dev/null 2>&1  ||  true
sarif="${outDir}/codeql_$(date +%Y%m%d-%H%M%S).sarif"

## Go leaves vendor/ out of the database unless told.
echo "Database: module and vendor/, ${GITNIBLET_JOBS} threads"
CODEQL_EXTRACTOR_GO_OPTION_EXTRACT_VENDOR_DIRS=true "${codeql}" database create "${workDir}/db" --language=go --quiet \
	--source-root="${repoRoot}/source" --threads="${GITNIBLET_JOBS}" --command="go build -mod=vendor ./..." > "${workDir}/create.log" 2>&1 \
	||  { tail -n 30 "${workDir}/create.log"; fDie "the database didn't build."; }
echo "Queries: go-security-and-quality"
"${codeql}" database analyze "${workDir}/db" codeql/go-queries:codeql-suites/go-security-and-quality.qls --quiet \
	--format=sarif-latest --output="${sarif}" --threads="${GITNIBLET_JOBS}" > "${workDir}/analyze.log" 2>&1 \
	||  { tail -n 30 "${workDir}/analyze.log"; fDie "the queries didn't run."; }
echo "Results: ${sarif}"
GOFLAGS='' go -C "${meDir}/codeql-check" run . "${sarif}" "${meDir}/codeql-accepted.txt"


##	History:
##		- 20261028 JC: Created.
