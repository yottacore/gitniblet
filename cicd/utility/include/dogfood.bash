#!/bin/bash
#  shellcheck disable=2034  ## 'appears unused.' The folder lists are for the script that sources this.

##	Purpose:
##		Dogfood copies for cicd.bash, per the shared CICD rules. Each OS gets a
##		list of folders, and the first one that exists wins.
##		- fDogfood_Copy BIN DIR...: copies BIN into the first DIR that exists.
##		  Skips with one line when no DIR exists, when the copy there is the same
##		  bytes, or when it's in use. Returns 1 only when the copy fails.
##		- fDogfood_InUse FILE: 0 when a process here runs FILE or holds it open.
##		- fDogfood BIN DIST CROSS: the Linux build, then with CROSS=1 the amd64
##		  Windows and macOS builds from goreleaser's DIST, since the boxes that
##		  run those are amd64.
##		Uses the caller's fEcho_Clean.
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

## HOME is read when fDogfood runs, not here, so a test can point it elsewhere.
fDogfood_Dirs(){
	dogfoodDirs_Linux=("${HOME}/synced/0-0/common/exec/app/linux")
	dogfoodDirs_Mswin=("${HOME}/synced/0-0/common/exec/app/mswin")
	dogfoodDirs_Macos=("${HOME}/synced/0-0/common/exec/app/macos")
}

fDogfood_InUse(){
	local f="${1}"
	[[ "${f}" == /* ]]  ||  f="./${f}"  ## psmisc's fuser takes no --
	if command -v fuser > /dev/null; then fuser -s "${f}" 2> /dev/null; return; fi
	## No psmisc: one pass over /proc for the exe links and open fds.
	local real; real="$(realpath -e -- "${f}")"  ||  return 1
	[[ -n "$(find /proc/[0-9]*/exe /proc/[0-9]*/fd -maxdepth 1 -lname "${real}" -print -quit 2> /dev/null)" ]]
}

fDogfood_Copy(){
	local src="${1}" dir="" next dst
	shift
	for next in "${@}"; do [[ ! -d "${next}" ]]  ||  { dir="${next%/}"; break; }; done
	[[ -n "${dir}" ]]  ||  { fEcho_Clean "Dogfood: no ${*}, skipped."; return 0; }
	dst="${dir}/${src##*/}"
	if [[ -e "${dst}" ]]; then
		if cmp -s -- "${src}" "${dst}"; then fEcho_Clean "Dogfood: ${dst} is already this build."; return 0; fi
		if fDogfood_InUse "${dst}"; then fEcho_Clean "Dogfood: ${dst} is in use, skipped."; return 0; fi
	fi
	## Copy beside it, then rename, so a reader never sees half a binary.
	if ! { cp --preserve=mode,timestamps --reflink=auto -- "${src}" "${dst}.new.$$"  &&  mv -f -- "${dst}.new.$$" "${dst}"; }; then
		rm -f -- "${dst}.new.$$"; return 1
	fi
	fEcho_Clean "Dogfood: ${src} -> ${dst}"
}

fDogfood(){
	local bin="${1}" dist="${2}" exe="${1##*/}"
	local -i cross="${3:-0}"
	local -a dogfoodDirs_Linux dogfoodDirs_Mswin dogfoodDirs_Macos found
	fDogfood_Dirs
	fDogfood_Copy "${bin}" "${dogfoodDirs_Linux[@]}"  ||  return 1
	if ((! cross)); then fEcho_Clean "Dogfood: no Windows or macOS build this run, skipped."; return 0; fi
	found=("${dist}"/*_windows_amd64*/"${exe}.exe")
	if [[ -f "${found[0]}" ]]; then fDogfood_Copy "${found[0]}" "${dogfoodDirs_Mswin[@]}"  ||  return 1
	else fEcho_Clean "Dogfood: no Windows amd64 build in ${dist}, skipped."; fi
	found=("${dist}"/*_darwin_amd64*/"${exe}")
	if [[ -f "${found[0]}" ]]; then fDogfood_Copy "${found[0]}" "${dogfoodDirs_Macos[@]}"  ||  return 1
	else fEcho_Clean "Dogfood: no macOS amd64 build in ${dist}, skipped."; fi
}
