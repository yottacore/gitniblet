#!/bin/bash

# shellcheck disable=2155  ## 'Declare and assign separately to avoid masking return values.' Cumbersome and unnecessary here.
# shellcheck disable=2317  ## 'Command appears to be unreachable.' False hit on the exit inside fDie().

##	Purpose: Download, verify and install the ngdb executable from a GitHub
##		release. Works on Linux, BSD, macOS and WSL. Safe to re-run.
##	History: At bottom of script.

##	Copyright © 2026 Jim Collier
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

## Bash 3.2 compatible on purpose - that is what macOS still ships.

set -u

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Settings

declare -r repoOwner="jim-collier"
declare -r repoName="nano-git-db"
declare -r exeName="ngdb"
declare -r apiBase="https://api.github.com/repos/${repoOwner}/${repoName}"
declare -r dlBase="https://github.com/${repoOwner}/${repoName}/releases/download"

declare releaseChannel=""   ## stable | dev; empty means stable, then dev
declare installTarget=""    ## user | system; empty means chosen from privileges
declare archOverride=""
declare tagOverride=""
declare -i assumeYes=0
declare -i listOnly=0
declare workDir=""

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Output

fEcho()       { echo "[ $* ]" ;:;}
fEcho_Clean() { echo "$*" ;:;}
fErr()        { echo "[ $* ]" >&2 ;:;}
fDie()        { fEcho_Clean ""; fErr "$*"; fEcho_Clean ""; exit 1 ;:;}

fUsage(){
	fEcho_Clean ""
	fEcho_Clean "Install ${exeName} from a GitHub release."
	fEcho_Clean ""
	fEcho_Clean "  install.bash [--release stable|dev] [--target user|system] [--arch x64|arm64]"
	fEcho_Clean ""
	fEcho_Clean "  --release stable   newest full release (the default when one exists)"
	fEcho_Clean "  --release dev      newest release of any kind, pre-releases included"
	fEcho_Clean "  --tag <tag>        one exact release, e.g. v1.0.0-beta.1"
	fEcho_Clean "  --target user      install to \${HOME}/.local/bin (the default)"
	fEcho_Clean "  --target system    install to /usr/local/bin (asks for sudo)"
	fEcho_Clean "  --arch x64|arm64   override the detected CPU architecture"
	fEcho_Clean "  --list             show the available releases and stop"
	fEcho_Clean "  --yes, -y          skip the confirmation prompt"
	fEcho_Clean "  --help, -h         this text"
	fEcho_Clean ""
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Arguments

fParseArgs(){
	while (( $# )); do
		case "$1" in
			--release)    releaseChannel="${2:-}"; shift 2 || fDie "--release needs stable or dev" ;;
			--release=*)  releaseChannel="${1#*=}"; shift ;;
			--target)     installTarget="${2:-}";  shift 2 || fDie "--target needs user or system" ;;
			--target=*)   installTarget="${1#*=}"; shift ;;
			--arch)       archOverride="${2:-}";   shift 2 || fDie "--arch needs x64 or arm64" ;;
			--arch=*)     archOverride="${1#*=}";  shift ;;
			--tag)        tagOverride="${2:-}";    shift 2 || fDie "--tag needs a release tag" ;;
			--tag=*)      tagOverride="${1#*=}";   shift ;;
			--list)       listOnly=1;  shift ;;
			--yes|-y)     assumeYes=1; shift ;;
			--help|-h)    fUsage; exit 0 ;;
			*)            fUsage; fDie "unrecognized option: $1" ;;
		esac
	done

	case "${releaseChannel}" in
		""|stable|dev) ;;
		*) fDie "--release takes stable or dev, not ${releaseChannel}" ;;
	esac
	case "${installTarget}" in
		""|user|system) ;;
		*) fDie "--target takes user or system, not ${installTarget}" ;;
	esac
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Platform

## Release assets are named by Go's own OS and architecture words.
fDetectOs(){
	local kernel="$(uname -s)"
	case "${kernel}" in
		Linux)   echo "linux"   ;;
		Darwin)  echo "darwin"  ;;
		FreeBSD) echo "freebsd" ;;
		MINGW*|MSYS*|CYGWIN*)
			fDie "on Windows use install.ps1 instead - see the README" ;;
		*) fDie "unsupported system: ${kernel}" ;;
	esac
}

fDetectArch(){
	local machine="${archOverride:-$(uname -m)}"
	case "${machine}" in
		x86_64|amd64|x64) echo "amd64" ;;
		aarch64|arm64)    echo "arm64" ;;
		*) fDie "unsupported architecture: ${machine} (try --arch x64 or --arch arm64)" ;;
	esac
}

## A checksum tool is required, not optional - an unverified download is worse
## than no download.
fShaCommand(){
	if   command -v sha256sum >/dev/null 2>&1; then echo "sha256sum"
	elif command -v shasum    >/dev/null 2>&1; then echo "shasum -a 256"
	else fDie "need sha256sum or shasum to verify the download"
	fi
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Releases

## No trailing ':' here - callers branch on curl's exit status.
fFetch(){ curl -fsSL "$1"; }

## GitHub's own idea of "latest" already excludes pre-releases and drafts.
fLatestStableTag(){
	fFetch "${apiBase}/releases/latest" 2>/dev/null \
		| grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
		| head -1 | sed 's/.*"\([^"]*\)"$/\1/'
}

fLatestAnyTag(){
	fFetch "${apiBase}/releases?per_page=1" \
		| grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
		| head -1 | sed 's/.*"\([^"]*\)"$/\1/'
}

## tag_name always precedes prerelease within a release object, and neither key
## appears in the nested author or asset objects, so the matches pair up in order.
fListReleases(){
	local tag=""
	fFetch "${apiBase}/releases?per_page=100" \
		| grep -oE '"(tag_name|prerelease)"[[:space:]]*:[[:space:]]*("[^"]*"|true|false)' \
		| sed 's/.*:[[:space:]]*//; s/"//g' \
		| while read -r value; do
			if [[ -z "${tag}" ]]; then
				tag="${value}"
			else
				if [[ "${value}" == "true" ]]; then
					fEcho_Clean "  ${tag}  (pre-release)"
				else
					fEcho_Clean "  ${tag}"
				fi
				tag=""
			fi
		done
}

fResolveTag(){
	[[ -n "${tagOverride}" ]] && { echo "${tagOverride}"; return; }
	local tag=""
	if [[ "${releaseChannel}" != "dev" ]]; then
		tag="$(fLatestStableTag)"
	fi
	## No full release yet, so fall back to the newest pre-release rather than
	## leaving a first-time user with nothing to install.
	[[ -z "${tag}" ]] && tag="$(fLatestAnyTag)"
	[[ -z "${tag}" ]] && fDie "no releases found at github.com/${repoOwner}/${repoName}"
	echo "${tag}"
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Install

fInstallDir(){
	if [[ "${installTarget}" == "system" ]]; then echo "/usr/local/bin"; return; fi
	echo "${HOME}/.local/bin"
}

fInstalledVersion(){
	local exe="$1"
	[[ -x "${exe}" ]] || return 0
	"${exe}" --version 2>/dev/null | awk '{print $2}'
}

fConfirm(){
	((assumeYes)) && return 0
	[[ -t 0 ]] || fDie "not running interactively - re-run with --yes to accept this plan"
	local reply=""
	read -r -p "Proceed? (y/N) " reply
	case "${reply}" in
		y|Y|yes|YES) return 0 ;;
		*) fEcho_Clean ""; fEcho "Nothing was changed."; fEcho_Clean ""; exit 0 ;;
	esac
}

## Writing into /usr/local/bin needs root. Ask through sudo rather than telling
## the user to re-run the whole thing as root.
fInstallFile(){
	local src="$1" dst="$2"
	local dir="$(dirname "${dst}")"
	if [[ -w "${dir}" ]] || { [[ ! -e "${dir}" ]] && mkdir -p "${dir}" 2>/dev/null; }; then
		mkdir -p "${dir}" || fDie "cannot create ${dir}"
		install -m 0755 "${src}" "${dst}" || fDie "cannot write ${dst}"
		return
	fi
	command -v sudo >/dev/null 2>&1 || fDie "${dir} is not writable and sudo is not available"
	fEcho_Clean ""
	fEcho "Elevating with sudo to write ${dst}"
	sudo mkdir -p "${dir}" || fDie "cannot create ${dir}"
	sudo install -m 0755 "${src}" "${dst}" || fDie "cannot write ${dst}"
}

fCleanup(){ [[ -n "${workDir}" && -d "${workDir}" ]] && rm -rf "${workDir}" ;:;}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Main

fMain(){
	fParseArgs "$@"
	command -v curl >/dev/null 2>&1 || fDie "curl is required"
	command -v tar  >/dev/null 2>&1 || fDie "tar is required"

	fEcho_Clean ""

	if ((listOnly)); then
		fEcho "Releases"
		fListReleases
		fEcho_Clean ""
		exit 0
	fi

	## These can fail fatally, and fDie inside a command substitution would only
	## kill the subshell - so take the status and stop here instead.
	local osName="" archName="" shaCmd="" tag=""
	osName="$(fDetectOs)"     || exit 1
	archName="$(fDetectArch)" || exit 1
	shaCmd="$(fShaCommand)"   || exit 1
	[[ -z "${installTarget}" ]] && installTarget="user"

	fEcho "Looking up the release"
	tag="$(fResolveTag)" || exit 1
	local version="${tag#v}"
	local asset="${exeName}-${version}-${osName}-${archName}.tar.gz"
	local installDir="$(fInstallDir)"
	local installPath="${installDir}/${exeName}"
	local current="$(fInstalledVersion "${installPath}")"

	fEcho_Clean ""
	fEcho "Plan"
	fEcho_Clean "  release:  ${tag}"
	fEcho_Clean "  package:  ${asset}"
	fEcho_Clean "  install:  ${installPath}"
	if [[ -n "${current}" ]]; then
		fEcho_Clean "  present:  ${current}"
	fi
	fEcho_Clean ""

	if [[ "${current}" == "${version}" ]]; then
		fEcho "${exeName} ${version} is already installed at ${installPath}."
		fEcho_Clean ""
		exit 0
	fi

	fConfirm

	workDir="$(mktemp -d)" || fDie "cannot create a temporary directory"
	trap fCleanup EXIT INT TERM

	fEcho_Clean ""
	fEcho "Downloading"
	fFetch "${dlBase}/${tag}/${asset}"        > "${workDir}/${asset}"   || fDie "cannot download ${asset}"
	fFetch "${dlBase}/${tag}/checksums.txt"   > "${workDir}/checksums.txt" || fDie "cannot download checksums.txt"

	fEcho "Verifying"
	grep " ${asset}\$" "${workDir}/checksums.txt" > "${workDir}/expected.txt" \
		|| fDie "${asset} is not listed in checksums.txt"
	( cd "${workDir}" && ${shaCmd} -c expected.txt >/dev/null 2>&1 ) \
		|| fDie "checksum mismatch on ${asset} - the download was not what the release published"

	fEcho "Unpacking"
	tar -xzf "${workDir}/${asset}" -C "${workDir}" || fDie "cannot unpack ${asset}"
	[[ -f "${workDir}/${exeName}" ]] || fDie "${exeName} is not in the archive"

	fEcho "Installing"
	fInstallFile "${workDir}/${exeName}" "${installPath}"

	fEcho_Clean ""
	fEcho "Installed ${exeName} ${version} to ${installPath}"
	case ":${PATH}:" in
		*":${installDir}:"*) ;;
		*) fEcho_Clean "  ${installDir} is not on your PATH - add it to run ${exeName} by name." ;;
	esac
	fEcho_Clean "  Getting started:  ${exeName} --help"
	fEcho_Clean ""
}

fMain "$@"

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## History
##	20260805 JC: Created.
