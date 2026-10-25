#!/usr/bin/env bash
#  shellcheck disable=1091  ## 'Not following.' The include is found at run time.
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes.' That's PowerShell for the box.
#  shellcheck disable=2029  ## 'Expands on the client side.' On purpose, for the box's work folder.
#  shellcheck disable=2155  ## 'Declare and assign separately.' Fine for these readonly paths.

##	Purpose:
##		Run the Go tests and the CLI smoke on the test boxes, natively, for the
##		platforms this box only cross-builds. Builds the test binaries and ngdb
##		per target here, then holds each box through the shared host lock's VM
##		wrap for the whole time it's used, which also puts a VM back as found.
##		Each box's test output goes through testid remote here, since the boxes
##		have no Go, for one PASS, FAIL or SKIP line per test with its ID.
##	Syntax:
##		remote-test.bash [--keep-temp] [--summary FILE] [target...]
##		Targets: windows freebsd linux-arm64 macos. Default: all of them.
##	Notes:
##		- A box that doesn't answer, or can't be held, is a SKIP line, never a
##		  failure. Only a test that ran and failed fails the run.
##		- The wrap helper is NGDB_VM_WRAP, else the *_vm-state.bash in the synced
##		  util folder. Without one every target is skipped.
##		- NGDB_REMOTE_WAIT is how long to wait in line for a box, default 900s.
##	History: At bottom of this file.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Eeuo pipefail

declare -r meFile="$(realpath -e "${BASH_SOURCE[0]}")"
declare -r dirPath_Base="$(realpath -e "$(dirname "${meFile}")/../..")"
declare -r skipCode=75  ## from the per-box run: the box didn't answer

## target | GOOS | GOARCH | ssh destinations, first that answers | most test jobs there
## b26 before vmKt6m: it's real hardware, and vmKt6m ignores the ACPI shutdown.
## arm64 is emulated, so every guest core costs several here.
declare -ra targetTable=(
	"windows     | windows | amd64 | wintest@b29w                  | 4"
	"freebsd     | freebsd | amd64 | bsdtest@vmFreeBSD             | 4"
	"linux-arm64 | linux   | arm64 | collierjr@vmDebARM64          | 2"
	"macos       | darwin  | amd64 | mactest@b26 mactest@vmKt6m    | 4"
)
declare -ra sshOpts=(-o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=30 -o ServerAliveCountMax=4)

fEcho(){ printf '%s\n' "${*}"; }
fWarn(){ printf 'WARNING: %s\n' "${*}" >&2; }
fDie(){  printf 'remote-test: %s\n' "${*}" >&2; exit 1; }

fTrim(){ local -n s_t9qx="${1}"; s_t9qx="${s_t9qx#"${s_t9qx%%[![:space:]]*}"}"; s_t9qx="${s_t9qx%"${s_t9qx##*[![:space:]]}"}"; }

## Fills target, goos, goarch, dests and jobCap in the caller from one table row.
fParseRow(){
	local -a f; IFS='|' read -r -a f <<< "${1}"
	target="${f[0]}"; goos="${f[1]}"; goarch="${f[2]}"; dests="${f[3]}"; jobCap="${f[4]}"
	fTrim target; fTrim goos; fTrim goarch; fTrim dests; fTrim jobCap
}


## Test binaries, ngdb, the source the tests read, and the box-side runner, in one tarball.
fBuildBundle(){
	local -r tgt="${1}" os="${2}" arch="${3}"
	local -r stage="${runDir}/${tgt}/bundle"
	mkdir -p "${stage}/tests" "${stage}/cicd/utility"
	(	cd "${dirPath_Base}/source"
		export CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" GOFLAGS=-mod=vendor
		nice -n 19 go test -c -p "${NGDB_JOBS}" -ldflags='-s -w' -o "${stage}/tests/" ./... > "${runDir}/${tgt}/build.log" 2>&1 \
			||  { grep -v 'no test files' "${runDir}/${tgt}/build.log" >&2; fDie "${tgt}: test build failed"; }
		## Name -> folder, since a test binary runs from its own package folder, and the import path its test IDs go by.
		go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.Name}} {{.Dir}} {{.ImportPath}}{{end}}' ./... | sed "s#${PWD}#source#" > "${stage}/tests/manifest"
	)
	## build.bash stamps the commit time and date, and names a cross build ngdb-<os>-<arch>.
	GOOS="${os}" GOARCH="${arch}" nice -n 19 bash "${dirPath_Base}/cicd/build.bash" > /dev/null
	if [[ "${os}" == "windows" ]]; then
		cp "${dirPath_Base}/bin/ngdb-${os}-${arch}.exe" "${stage}/ngdb.exe"
		cp "${dirPath_Base}/cicd/utility/remote-windows.ps1" "${runDir}/${tgt}/"  ## it unpacks the bundle, so it goes on its own
	else
		cp "${dirPath_Base}/bin/ngdb-${os}-${arch}" "${stage}/ngdb"
	fi
	mkdir -p "${stage}/cicd/utility/include"
	cp "${dirPath_Base}/cicd/utility/smoke-cli.bash" "${dirPath_Base}/cicd/utility/remote-unix.bash" "${stage}/cicd/utility/"
	cp -r "${dirPath_Base}/cicd/utility/smoke" "${stage}/cicd/utility/"
	cp "${dirPath_Base}/cicd/utility/include/temp-run.bash" "${dirPath_Base}/cicd/utility/include/build-number.bash" "${stage}/cicd/utility/include/"
	## The ddl and seed tests parse project/example.shcl, from 4 levels up their package folder.
	tar -C "${dirPath_Base}" --exclude=./source/vendor -cf - ./source ./project/example.shcl | tar -C "${stage}" -xf -
	tar -C "${stage}" -czf "${runDir}/${tgt}/bundle.tgz" .
	[[ -s "${stage}/tests/manifest" ]]  ||  fDie "${tgt}: no test packages found"
}


## Runs inside the wrap, so the box is held and awake. Writes a status file the
## caller reads, since the wrap's own exit also covers putting the VM back.
fOnBox(){
	local -r tgt="${1}" dest="${2}" jobCap="${3}"
	local -r statusFile="${runDir}/${tgt}/status"
	echo "started" > "${statusFile}"
	if ! ssh "${sshOpts[@]}" "${dest}" exit 0 < /dev/null &> /dev/null; then
		echo "noanswer" > "${statusFile}"; return "${skipCode}"
	fi
	[[ "${tgt}" != "windows" ]]  ||  { fOnBox_Windows "${tgt}" "${dest}" "${jobCap}"; return; }
	## The box's own temp root per Temp files, as sh, since a login shell may be csh.
	local work
	work="$(ssh "${sshOpts[@]}" "${dest}" sh -s <<- 'EOF'
		r="${HOST_TEMP:-${TMPDIR:-/tmp}}"; r="${r%/}/nano-git-db-floss"
		mkdir -p "${r}" && cd "${r}" && r="$(pwd -P)" || exit 1
		s="$(date +%Y%m%d-%H%M%S)"; n=1
		while ! mkdir "$(printf '%s/%s_%03d' "${r}" "${s}" "${n}")" 2> /dev/null; do
			n=$((n + 1)); [ "${n}" -le 999 ] || exit 1
		done
		printf '%s/%s_%03d\n' "${r}" "${s}" "${n}"
	EOF
	)" || { echo "fail" > "${statusFile}"; fWarn "${tgt}: couldn't make a work folder on ${dest}"; return 1; }
	work="${work##*$'\n'}"  ## a noisy login can print first
	[[ "${work}" =~ ^/[A-Za-z0-9._/-]+/nano-git-db-floss/[0-9]{8}-[0-9]{6}_[0-9]{3}$ ]]  ||  { echo "fail" > "${statusFile}"; fWarn "${tgt}: odd work folder '${work}'"; return 1; }
	fEcho "${tgt}: ${dest}:${work}"

	## cat, not scp: a login that prints breaks scp and sftp.
	local rc=0
	ssh "${sshOpts[@]}" "${dest}" "cat > '${work}/bundle.tgz'" < "${runDir}/${tgt}/bundle.tgz"  || rc=1
	if ((rc == 0)); then
		timeout 2400 ssh "${sshOpts[@]}" "${dest}" \
			"cd '${work}' && tar -xzf bundle.tgz && bash ./cicd/utility/remote-unix.bash '${work}' ${jobCap} ${keepTemp}" < /dev/null \
			| "${runDir}/testid" remote "${dirPath_Base}"  ||  rc=$?
	fi
	if ((rc == 0)); then echo "pass" > "${statusFile}"; else echo "fail" > "${statusFile}"; fi
	return "${rc}"
}

## ssh there opens cmd, so the setup goes as encoded PowerShell.
fOnBox_Windows(){
	local -r tgt="${1}" dest="${2}" jobCap="${3}"
	local -r statusFile="${runDir}/${tgt}/status"
	local -r mkWork='$r = if ($env:HOST_TEMP) { $env:HOST_TEMP } else { $env:TEMP }
		$r = Join-Path $r nano-git-db-floss; $null = New-Item -ItemType Directory -Force $r
		$s = Get-Date -Format yyyyMMdd-HHmmss
		for ($n = 1; $n -le 999; $n++) {
			$p = "{0}\{1}_{2:d3}" -f $r, $s, $n
			try { $null = New-Item -ItemType Directory $p -ErrorAction Stop; $p; break } catch { }
		}'
	local work
	work="$(ssh "${sshOpts[@]}" "${dest}" pwsh -NoProfile -EncodedCommand "$(iconv -f utf-8 -t utf-16le <<< "${mkWork}" | base64 -w0)" < /dev/null 2> /dev/null | tr -d '\r')" \
		||  { echo "fail" > "${statusFile}"; fWarn "${tgt}: couldn't make a work folder on ${dest}"; return 1; }
	work="${work##*$'\n'}"
	[[ "${work}" =~ ^[A-Za-z]:\\[A-Za-z0-9._~\\-]+\\nano-git-db-floss\\[0-9]{8}-[0-9]{6}_[0-9]{3}$ ]]  ||  { echo "fail" > "${statusFile}"; fWarn "${tgt}: odd work folder '${work}'"; return 1; }
	fEcho "${tgt}: ${dest}:${work}"

	local rc=0
	scp -q "${sshOpts[@]}" "${runDir}/${tgt}/bundle.tgz" "${runDir}/${tgt}/remote-windows.ps1" "${dest}:${work//\\//}/" 2> /dev/null  ||  rc=1
	if ((rc == 0)); then
		timeout 2400 ssh "${sshOpts[@]}" "${dest}" \
			pwsh -NoProfile -ExecutionPolicy Bypass -File "${work}\\remote-windows.ps1" "${work}" "${jobCap}" "${keepTemp}" < /dev/null 2> >(grep -v '^\*\*' >&2) \
			| "${runDir}/testid" remote "${dirPath_Base}"  ||  rc=$?  ## it drops the CRs too
	fi
	if ((rc == 0)); then echo "pass" > "${statusFile}"; else echo "fail" > "${statusFile}"; fi
	return "${rc}"
}


fMain(){
	local -a want=()
	local summaryFile=""
	keepTemp=0
	while (($#)); do case "${1}" in
		--on-box)     shift; runDir="${1}"; keepTemp="${4}"; fOnBox "${2}" "${3}" "${5}"; return ;;
		--keep-temp)  keepTemp=1; shift ;;
		--summary)    summaryFile="${2:-}"; shift 2 ;;
		-h|--help)    sed -n 's/^##\t\{0,1\}//p' "${meFile}" | sed -n '/^Purpose:/,/^History:/p' | sed '$d'; return 0 ;;
		-*)           fDie "unknown option '${1}'" ;;
		*)            want+=("${1}"); shift ;;
	esac; done

	local -a summary=()
	local -i failed=0
	local row target goos goarch dests jobCap
	local -a rows=()
	for row in "${targetTable[@]}"; do
		fParseRow "${row}"
		if ((${#want[@]})) && [[ " ${want[*]} " != *" ${target} "* ]]; then continue; fi
		rows+=("${row}")
	done
	((${#rows[@]}))  ||  fDie "no such target: ${want[*]}"

	local vmWrap="${NGDB_VM_WRAP:-}"
	if [[ -z "${vmWrap}" ]]; then
		local -a found=("${HOME}"/synced/0-0/common/exec/util/linux/bash/*_vm-state.bash)
		[[ -x "${found[0]}" ]]  &&  vmWrap="${found[0]}"
	fi
	if [[ -z "${vmWrap}" ]] || ! command -v go &> /dev/null; then
		local why="no VM and host lock wrap helper here"; [[ -n "${vmWrap}" ]] && why="no go here"
		for row in "${rows[@]}"; do fParseRow "${row}"; summary+=("SKIP ${target}: ${why}"); fWarn "${target}: ${why}"; done
		fSummary; return 0
	fi

	## Inside the cicd run's folder when there is one.
	runDir="$(fTempDir_Make remote)"  ||  fDie "no run folder under ${HOST_TEMP}/${tempProject}"
	if ((keepTemp)); then
		trap 'echo "kept ${runDir}"' EXIT
	else
		trap 'fSafeRm "${runDir}" > /dev/null || true' EXIT
	fi

	## Build everything before holding any box. testid is its own module, outside vendor mode.
	GOFLAGS='' nice -n 19 go -C "${dirPath_Base}/cicd/utility/testid" build -o "${runDir}/testid" .  ||  fDie "testid build failed"
	for row in "${rows[@]}"; do
		fParseRow "${row}"
		fEcho "${target}: building ${goos}/${goarch}"
		fBuildBundle "${target}" "${goos}" "${goarch}"
	done

	local dest host status wrapRc hosts
	for row in "${rows[@]}"; do
		fParseRow "${row}"
		status="noanswer"; hosts=""
		for dest in ${dests}; do hosts+="${hosts:+, }${dest#*@}"; done
		for dest in ${dests}; do
			host="${dest#*@}"
			rm -f "${runDir}/${target}/status"
			wrapRc=0
			"${vmWrap}" wrap "${host}" --wait "${NGDB_REMOTE_WAIT:-900}" -- \
				bash "${meFile}" --on-box "${runDir}" "${target}" "${dest}" "${keepTemp}" "${jobCap}" || wrapRc=$?
			status="$(cat "${runDir}/${target}/status" 2> /dev/null || echo "unheld")"
			case "${status}" in
				pass)  summary+=("PASS ${target} on ${host}") ;;
				fail)  summary+=("FAIL ${target} on ${host}"); failed+=1 ;;
				started|noanswer) fWarn "${target}: ${host} didn't answer"; status="noanswer"; continue ;;
				*)     fWarn "${target}: couldn't hold or wake ${host} (exit ${wrapRc})"; status="noanswer"; continue ;;
			esac
			## The status says the tests' result, so a non-zero wrap after a pass is the put-back.
			if [[ "${status}" == "pass" ]] && ((wrapRc != 0)); then fWarn "${target}: ${host} may not be back as it was found"; fi
			break
		done
		[[ "${status}" != "noanswer" ]]  ||  summary+=("SKIP ${target}: no answer from ${hosts}")
	done

	fSummary
	((failed == 0))
}

fSummary(){
	fEcho ""
	fEcho "Remote tests:"
	local line; for line in "${summary[@]}"; do fEcho "  ${line}"; done
	[[ -z "${summaryFile}" ]]  ||  printf '%s\n' "${summary[@]}" 2> /dev/null > "${summaryFile}"  ||  fWarn "couldn't write ${summaryFile}"
}

source "$(dirname "${meFile}")/include/cpu-limit.bash"
source "$(dirname "${meFile}")/include/temp-run.bash"  ## fTempDir_Make + fSafeRm
fMain "${@}"


##	History:
##		- 20261025 JC: Created. FreeBSD, Linux arm64 and Intel macOS.
##		- 20261025 JC: Run folders under nano-git-db-floss, here and on the boxes, through the shared guard.
##		- 20261025 JC: One line per test with its ID, through testid remote.
