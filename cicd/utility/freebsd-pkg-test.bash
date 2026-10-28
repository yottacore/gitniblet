#!/usr/bin/env bash

##	Purpose:
##		Checks freebsd-pkg.bash on a stand-in binary: the layout pkg needs,
##		the manifests, the version pkg sorts by, and that other OSes get no
##		.pkg. repro-test.bash covers same bytes twice, and remote-test.bash
##		the install on the FreeBSD box.
##	Syntax:
##		freebsd-pkg-test.bash     ## about a second
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -uo pipefail

repoRoot="$(realpath -e "$(dirname "$0")/../..")"
maker="${repoRoot}/cicd/utility/freebsd-pkg.bash"
# shellcheck source=include/temp-run.bash
source "${repoRoot}/cicd/utility/include/temp-run.bash"
runDir="$(fTempDir_Make freebsd-pkg-test)"  ||  exit 1
trap 'fSafeRm "${runDir}" > /dev/null' EXIT
declare -i fails=0
fCase(){ if [[ "${3}" == "${4}" ]]; then echo "PASS ${1} ${2}"; else echo "FAIL ${1} ${2}: got '${3}', want '${4}'"; fails+=1; fi; }
stamp=1793203398  ## 2026-10-28 16:03:18 UTC

## Prints the .pkg made for one target, or nothing.
fMake(){  ## os arch version
	local d; d="$(mktemp -d "${runDir}/case.XXXX")"  ||  return 1
	mkdir -p "${d}/dist/gitniblet_${1}_${2}"
	printf 'not really a binary\n' > "${d}/dist/gitniblet_${1}_${2}/gitniblet"
	bash "${maker}" "${1}" "${2}" "${d}/dist/gitniblet_${1}_${2}/gitniblet" "${3}" "${stamp}" > /dev/null  ||  { echo "freebsd-pkg.bash failed" >&2; return 1; }
	find "${d}/dist" -maxdepth 1 -name '*.pkg'
}
fMember(){ zstd -dcq "${1}" | tar -P -xOf - "${2}"; }

pkg="$(fMake freebsd amd64 1.0.0-beta.2)"
fCase dehz9 "named for the version, OS and arch" "${pkg##*/}" "gitniblet-1.0.0-beta.2-freebsd-amd64.pkg"
listing="$(zstd -dcq "${pkg}" | tar -P -tvf - --utc --full-time | awk '{print $1, $2, $4, $5, $6}')"
want="-rw-r--r-- root/wheel 2026-10-28 16:03:18 +COMPACT_MANIFEST
-rw-r--r-- root/wheel 2026-10-28 16:03:18 +MANIFEST
-rwxr-xr-x root/wheel 2026-10-28 16:03:18 /usr/local/bin/gitniblet
-rw-r--r-- root/wheel 2026-10-28 16:03:18 /usr/local/share/doc/gitniblet/README.md
-rw-r--r-- root/wheel 2026-10-28 16:03:18 /usr/local/share/doc/gitniblet/license.md"
fCase dehz9 "manifests first, then absolute paths as root:wheel at the commit time" "${listing}" "${want}"

manifest="$(fMember "${pkg}" +MANIFEST)"
compact="$(fMember "${pkg}" +COMPACT_MANIFEST)"
fCase dehza "the compact manifest is the manifest less its files" "${compact}" "${manifest%,\"files\":*}}"
bad="" size=0
while IFS= read -r entry; do
	f="${entry%%\":*}"; f="${f#\"}"; sum="${entry##*\$}"; sum="${sum%\"}"
	[[ "$(fMember "${pkg}" "${f}" | sha256sum)" == "${sum}  -" ]]  ||  bad+=" ${f}"
	size=$((size + $(fMember "${pkg}" "${f}" | wc -c)))
done < <(grep -oE '"/usr/local/[^"]+":"1\$[0-9a-f]{64}"' <<< "${manifest}")
fCase dehza "each file's sha256 is in the manifest" "${bad:-none wrong}" "none wrong"
fCase dehza "flatsize is the files' total" "$(grep -oE '"flatsize":[0-9]+' <<< "${manifest}")" "\"flatsize\":${size}"
fCase dehza "amd64 ABI fits any FreeBSD release" "$(grep -oE '"abi":"[^"]+"' <<< "${manifest}")" '"abi":"FreeBSD:*:amd64"'
pkg="$(fMake freebsd arm64 1.0.0)"
fCase dehza "arm64 ABI is aarch64" "$(fMember "${pkg}" +COMPACT_MANIFEST | grep -oE '"abi":"[^"]+"')" '"abi":"FreeBSD:*:aarch64"'

for pair in 1.2.3=1.2.3 1.0.0-beta.2=1.0.0.b.2 2.0.0-alpha.10=2.0.0.a.10 1.0.0-rc.1=1.0.0.r.1 1.0.0-beta.2-SNAPSHOT-1e5c0dc=1.0.0.b.2; do
	pkg="$(fMake freebsd amd64 "${pair%=*}")"
	fCase dehzb "pkg version for ${pair%=*}" "$(fMember "${pkg}" +COMPACT_MANIFEST | grep -oE '"version":"[^"]+"')" "\"version\":\"${pair#*=}\""
done

for os in linux windows darwin; do
	fCase dehzc "no .pkg for ${os}" "$(fMake "${os}" amd64 1.0.0)" ""
done

exit $((fails > 0))


##	History:
##		- 20261028 JC: Created.
