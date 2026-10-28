#!/usr/bin/env bash

##	Purpose:
##		Makes a FreeBSD .pkg from one goreleaser-built freebsd binary, beside
##		the archives in dist/, as gitniblet-<version>-freebsd-<arch>.pkg.
##		goreleaser runs it as a post-build hook for every target, and it does
##		nothing for the other OSes. nfpm has no FreeBSD packager, so this
##		writes pkg's own format: a tar with +COMPACT_MANIFEST and +MANIFEST
##		first, then the files by their installed paths, zstd-compressed.
##		- Every entry gets the commit's time, owner root:wheel, and a fixed
##		  order, so 2 builds of one commit give the same bytes.
##		- The ABI is FreeBSD:*:<arch>, since the binary is static Go.
##	Syntax:
##		freebsd-pkg.bash <os> <arch> <binary> <version> <commit unix time>
##	History: At bottom of this file.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Eeuo pipefail

fDie(){ printf 'freebsd-pkg: %s\n' "${*}" >&2; exit 1; }

## Ports' own DISTVERSION -> PORTVERSION rule, so pkg sorts 1.0.0-beta.2 before
## 1.0.0. A snapshot keeps its release's version; --version tells builds apart.
fPkgVersion(){
	local v="${1%%-SNAPSHOT*}"
	sed -E 's/([a-z])[a-z]+/\1/g; s/([0-9])([a-z])/\1.\2/g; s/[^a-z0-9+]+/./g; s/^\.|\.$//g' <<< "${v,,}"
}

## JSON string, for the few fields here, which hold no control characters.
fJson(){ local s="${1//\\/\\\\}"; s="${s//\"/\\\"}"; s="${s//$'\n'/\\n}"; printf '"%s"' "${s}"; }

fMain(){
	(($# == 5))  ||  fDie "usage: freebsd-pkg.bash <os> <arch> <binary> <version> <commit unix time>"
	local -r os="${1}" arch="${2}" bin="${3}" version="${4}" stamp="${5}"
	[[ "${os}" == "freebsd" ]]  ||  return 0
	[[ -f "${bin}" ]]  ||  fDie "no binary at ${bin}"
	[[ "${stamp}" =~ ^[0-9]+$ ]]  ||  fDie "commit time isn't a unix time: '${stamp}'"
	local abiArch
	case "${arch}" in
		amd64) abiArch="amd64" ;;
		arm64) abiArch="aarch64" ;;
		*)     fDie "no FreeBSD ABI name for ${arch}" ;;
	esac
	local -r repoRoot="$(realpath -e "$(dirname "${BASH_SOURCE[0]}")/../..")"
	local -r dist="$(realpath -e "$(dirname "${bin}")/..")"
	local -r out="${dist}/gitniblet-${version}-freebsd-${arch}.pkg"
	local -r stage="${dist}/gitniblet_freebsd_${arch}_pkg"
	local -r prefix="/usr/local"

	## Inside dist/, which goreleaser --clean empties, so no temp folder to track.
	[[ ! -e "${stage}" ]]  ||  fDie "${stage} is already there"
	mkdir -p "${stage}${prefix}/bin" "${stage}${prefix}/share/doc/gitniblet"
	install -m 0755 "${bin}" "${stage}${prefix}/bin/gitniblet"
	install -m 0644 "${repoRoot}/README.md" "${repoRoot}/license.md" "${stage}${prefix}/share/doc/gitniblet/"
	local -ra files=(
		"${prefix}/bin/gitniblet"
		"${prefix}/share/doc/gitniblet/README.md"
		"${prefix}/share/doc/gitniblet/license.md"
	)

	local f sum fileList="" sep=""
	local -i flatsize=0
	for f in "${files[@]}"; do
		flatsize+=$(stat -c %s "${stage}${f}")
		sum="$(sha256sum "${stage}${f}")"
		fileList+="${sep}$(fJson "${f}"):$(fJson "1\$${sum%% *}")"; sep=","
	done

	local -r head="$(printf '"name":"gitniblet","origin":"databases/gitniblet","version":%s,"comment":%s,"maintainer":"release@gitniblet.local","www":"https://github.com/yottacore/gitniblet","abi":%s,"prefix":%s,"flatsize":%d,"licenselogic":"single","licenses":["AGPLv3+"],"desc":%s,"categories":["databases"]' \
		"$(fJson "$(fPkgVersion "${version}")")" \
		"$(fJson "Local-first database on an append-only, git-syncable transaction log")" \
		"$(fJson "FreeBSD:*:${abiArch}")" "$(fJson "${prefix}")" "${flatsize}" \
		"$(fJson "gitniblet is a local-first database on an append-only, git-syncable transaction log.")")"
	printf '{%s}' "${head}" > "${stage}/+COMPACT_MANIFEST"
	printf '{%s,"files":{%s}}' "${head}" "${fileList}" > "${stage}/+MANIFEST"
	chmod 0644 "${stage}/+COMPACT_MANIFEST" "${stage}/+MANIFEST"

	## pkg wants the 2 manifests first and absolute names. ustar, since GNU pax
	## headers carry atime and ctime.
	local -a names=(+COMPACT_MANIFEST +MANIFEST)
	for f in "${files[@]}"; do names+=(".${f}"); done
	tar -C "${stage}" --format=ustar -P --no-recursion --transform='s,^\./,/,' \
		--owner=root:0 --group=wheel:0 --mtime="@${stamp}" -cf - "${names[@]}" \
		| zstd -q -T1 -3 -c > "${out}.part"
	mv -f "${out}.part" "${out}"
	touch -d "@${stamp}" "${out}"
	echo "freebsd-pkg: ${out}"
}

fMain "${@}"


##	History:
##		- 20261028 JC: Created.
