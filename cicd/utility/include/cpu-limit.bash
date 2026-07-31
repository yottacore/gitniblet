#!/bin/bash

## Purpose:
##	- Cap any single build/test/fuzz/profile step to at most half the CPU cores,
##	  so a cicd run never pins the whole machine.
##	- Sets NGDB_JOBS (pass as `go build|test -p "${NGDB_JOBS}"`) and exports
##	  GOMAXPROCS, which also bounds go's in-package `-parallel` default and the
##	  `go test -fuzz` worker count. Child processes inherit GOMAXPROCS, so sourcing
##	  this once in cicd.bash covers every go tool it spawns; the standalone helpers
##	  source it too for when they run on their own.
##	- Source this file; do not execute it.
## History: At bottom of script.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

## Half the online cores, floor of 1. `if` (not `&&`) so a set -e caller is safe.
_ngdb_cores="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
NGDB_JOBS=$(( _ngdb_cores / 2 ))
if (( NGDB_JOBS < 1 )); then NGDB_JOBS=1; fi
export NGDB_JOBS
export GOMAXPROCS="${NGDB_JOBS}"
unset _ngdb_cores


##	History:
##		- 20260709: Created.
