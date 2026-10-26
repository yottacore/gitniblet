#!/usr/bin/env bash
#  shellcheck disable=2034  ## 'appears unused.' tempProject is for the scripts that source this.

##	Purpose:
##		Run folders and guarded removal, per the shared Temp files and Safe
##		removal rules. Sourced by cicd.bash, its helpers and the demo scripts.
##		Bash 3.2 safe, since smoke-cli.bash sources it on the macOS box.
##		- fTempRun_Make: makes ${HOST_TEMP}/gitniblet-floss/<stamp>_<NNN>/ with
##		  run.pid in it, and prints its path.
##		- fTempDir_Make LABEL: a folder for a helper. Inside the cicd run's folder
##		  (GITNIBLET_RUN_DIR) when there is one, else a run folder of its own.
##		- fTempRun_Sweep: removes this project's run folders older than 2 days
##		  whose run.pid names no live process.
##		- fSafeRm PATH: the repo's copy of the safe removal guard. Takes one
##		  absolute path under a folder named tmp, or under this project's own
##		  temp root, which the macOS and Windows temp folders need.
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

tempProject="gitniblet-floss"

## Resolved once, since macOS's temp folder sits behind the /var link.
HOST_TEMP="${HOST_TEMP:-${TMPDIR:-/tmp}}"
HOST_TEMP="$(cd -P -- "${HOST_TEMP}" 2> /dev/null && pwd -P || printf '%s' "${HOST_TEMP%/}")"
export HOST_TEMP

fTempRun_Make(){
	local root="${HOST_TEMP}/${tempProject}" stamp dir
	local -i n=1
	mkdir -p -- "${root}"  ||  return 1
	stamp="$(date +%Y%m%d-%H%M%S)"
	until dir="$(printf '%s/%s_%03d' "${root}" "${stamp}" "${n}")"; mkdir -- "${dir}" 2> /dev/null; do
		((++n <= 999))  ||  { printf 'temp-run: no free run folder under %s\n' "${root}" >&2; return 1; }
	done
	echo "$$" > "${dir}/run.pid"
	printf '%s\n' "${dir}"
}

fTempDir_Make(){
	if [[ -n "${GITNIBLET_RUN_DIR:-}" && -d "${GITNIBLET_RUN_DIR}" ]]; then
		mktemp -d "${GITNIBLET_RUN_DIR}/${1:-tmp}.XXXXXX"
	else
		fTempRun_Make
	fi
}

fTempRun_Sweep(){
	local root="${HOST_TEMP}/${tempProject}" dir pid
	[[ -d "${root}" ]]  ||  return 0
	while IFS= read -r dir; do
		pid=""; [[ -f "${dir}/run.pid" ]] && { read -r pid < "${dir}/run.pid" || true; }
		if [[ "${pid}" =~ ^[0-9]+$ ]] && kill -0 "${pid}" 2> /dev/null; then continue; fi
		fSafeRm "${dir}"  ||  true
	done < <(find "${root}" -mindepth 1 -maxdepth 1 -type d -mmin +2880 \
		-name '[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-[0-9][0-9][0-9][0-9][0-9][0-9]_[0-9][0-9][0-9]')
}

fSafeRm(){
	local p="${1:-}" real='' own="${HOST_TEMP:?}/${tempProject:?}"
	if [[ "${#}" -ne 1 || -z "${p}" || "${p}" == *..* || "${p}" == *//* || "${p}" == *$'\n'* ]]; then
		printf 'safe-rm: refused, bad path: "%s"\n' "${p}" >&2; return 1
	fi
	p="${p%/}"
	if ! [[ "${p}" =~ ^/(.+/)?tmp/[^/]+(/[^/]+)*$ || "${p}" == "${own}"/?* ]]; then
		printf 'safe-rm: refused, not under a tmp folder: %s\n' "${p}" >&2; return 1
	fi
	if [[ -L "${p}" ]]; then printf 'safe-rm: refused, a link: %s\n' "${p}" >&2; return 1; fi
	## cd -P rather than realpath -e, which macOS and the BSDs lack.
	if [[ -d "${p}" ]]; then real="$(cd -P -- "${p}" 2> /dev/null && pwd -P)"
	elif [[ -e "${p}" ]]; then real="$(cd -P -- "${p%/*}" 2> /dev/null && pwd -P)/${p##*/}"
	fi
	[[ -n "${real}" ]]  ||  { printf 'safe-rm: refused, not there: %s\n' "${p}" >&2; return 1; }
	if [[ "${real}" != "${p}" ]]; then printf 'safe-rm: refused, goes through a link: %s\n' "${p}" >&2; return 1; fi
	case "$(uname -s)" in
		Darwin|*BSD)  rm -rf -x -- "${p}" ;;
		*)            rm -rf --one-file-system -- "${p}" ;;
	esac  &&  printf 'removed %s\n' "${p}"
}


##	History:
##		- 20261025 JC: Created, for the run folder and the guard every cicd script shares.
