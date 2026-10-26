#!/bin/bash
#  shellcheck disable=2034  ## 'appears unused.' GITNIBLET_LOAD, GITNIBLET_BUSY and GITNIBLET_JOBS_MAX are for the scripts that source this.

## Purpose:
##	- Cap any single build/test/fuzz/profile step to at most half the CPU cores,
##	  so a cicd run never pins the whole machine. A quarter when the 1-minute load
##	  is over half the CPU count, and a quarter always for the heavy steps (fuzz,
##	  -race, leak checks). Never under 1.
##	- Sets GITNIBLET_JOBS (pass as `go build|test -p "${GITNIBLET_JOBS}"`) and exports
##	  GOMAXPROCS, which also bounds go's in-package `-parallel` default and the
##	  `go test -fuzz` worker count. Child processes inherit GOMAXPROCS, so sourcing
##	  this once in cicd.bash covers every go tool it spawns; the standalone helpers
##	  source it too for when they run on their own.
##	- Sourcing runs fCpuLimit once. cicd.bash runs it again before each step, and
##	  a heavy step runs `fCpuLimit heavy`. It also sets GITNIBLET_CORES, GITNIBLET_LOAD,
##	  GITNIBLET_BUSY, and GITNIBLET_JOBS_MAX, the not-busy count a container's --cpus gets.
##	- GITNIBLET_JOBS_CAP, when set, is the most it hands out. cicd.bash sets it for
##	  the run in the container, to that container's --cpus.
##	- Sourcing also drops this process to nice 19, and to the idle I/O class
##	  where there's ionice. Whatever it starts after that inherits both.
##	- Source this file; do not execute it.
## History: At bottom of script.

##	Copyright (c) 2026 Bubbles
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

## Sets __cpuJobs and __cpuBusy for <cores> <1-minute load> [heavy] [cap], with
## no forks, so the numbers can be checked on their own (cpu-limit-test.bash).
fCpuLimit_Pick(){
	local -i cores=0 jobs=0 busy=0
	local load="${2:-}" heavy="${3:-}" cap="${4:-}" whole frac
	[[ "${1:-}" =~ ^[0-9]+$ ]]  &&  cores=$((10#${1}))
	((cores >= 1))  ||  cores=2
	## Hundredths, so the compare stays in bash. Checked as digits first, since
	## arithmetic on a raw string can run what's in it.
	whole="${load%%.*}"; frac="0"
	[[ "${load}" == *.* ]]  &&  frac="${load#*.}"
	frac="${frac}00"; frac="${frac:0:2}"
	if [[ "${whole}" =~ ^[0-9]+$ && "${frac}" =~ ^[0-9]+$ ]] && (( 10#${whole} * 100 + 10#${frac} > cores * 50 )); then busy=1; fi
	if ((busy)) || [[ -n "${heavy}" ]]; then jobs=$((cores / 4)); else jobs=$((cores / 2)); fi
	if [[ "${cap}" =~ ^[0-9]+$ ]] && ((jobs > 10#${cap})); then jobs=$((10#${cap})); fi
	((jobs >= 1))  ||  jobs=1
	__cpuJobs="${jobs}"; __cpuBusy="${busy}"
}

fCpuLimit(){
	local load=""
	if [[ -z "${GITNIBLET_CORES:-}" ]]; then
		GITNIBLET_CORES="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2)"
	fi
	if [[ -r /proc/loadavg ]]; then
		read -r load _ < /proc/loadavg
	else  ## macOS and BSD: "{ 1.23 1.45 1.67 }"
		load="$(sysctl -n vm.loadavg 2>/dev/null || true)"; load="${load#\{ }"; load="${load%% *}"
	fi
	GITNIBLET_LOAD="${load:-?}"
	fCpuLimit_Pick "${GITNIBLET_CORES}" 0 "" "${GITNIBLET_JOBS_CAP:-}"
	GITNIBLET_JOBS_MAX="${__cpuJobs}"
	fCpuLimit_Pick "${GITNIBLET_CORES}" "${load}" "${1:-}" "${GITNIBLET_JOBS_CAP:-}"
	GITNIBLET_JOBS="${__cpuJobs}"; GITNIBLET_BUSY="${__cpuBusy}"
	export GITNIBLET_JOBS GOMAXPROCS="${GITNIBLET_JOBS}"
}

fCpuLimit

renice -n 19 -p "$$" >/dev/null 2>&1 || true
if command -v ionice >/dev/null 2>&1; then ionice -c 3 -p "$$" >/dev/null 2>&1 || true; fi


##	History:
##		- 20260709: Created.
##		- 20261025: A quarter when busy or for heavy steps, rechecked per step; nice 19 and idle I/O.
