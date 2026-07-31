#!/usr/bin/env bash

##	Purpose:
##		Run every Go fuzz target in the module for a short budget, so cicd exercises
##		the parsers/decoders with mutated input, not just the seed corpus (the seeds
##		already run in the normal `go test` suite). Targets are discovered, not
##		listed, so a new FuzzXxx is picked up with no change here.
##	Syntax:
##		fuzz.bash [--time DURATION]
##		  --time DURATION   per-target fuzz budget (Go duration, default 10s)
##	Exit: 0 all clean, 1 a target found a crash (input saved under its testdata/).
##	History: At bottom of script.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


set -Eeuo pipefail

fuzzTime="10s"
while (($#)); do case "$1" in
	--time)     fuzzTime="${2:?--time needs a duration}"; shift 2 ;;
	--time=*)   fuzzTime="${1#*=}"; shift ;;
	-h|--help)  grep -E '^##' "$0" | sed 's/^##\t\?//'; exit 0 ;;
	*) echo "fuzz.bash: unknown option: $1" >&2; exit 2 ;;
esac; done

source "$(dirname "$0")/include/cpu-limit.bash"   ## NGDB_JOBS + GOMAXPROCS (<=half cores; caps fuzz workers)
cd "$(dirname "$0")/../../source"   ## module root (go.mod)

## Discover "<import-path> <FuzzName>" pairs. `go test -list` prints the matching
## function names for a package first, then that package's "ok <import-path>" line,
## so buffer the names and flush them to the package once its line appears.
declare -a targets=()
declare -a pending=()
while IFS= read -r line; do
	case "$line" in
		Fuzz*)                 pending+=("$line") ;;
		ok*|FAIL*|"?"*)        pkg="$(awk '{print $2}' <<<"$line")"
		                       for fn in "${pending[@]:-}"; do [[ -n "$fn" ]] && targets+=("$pkg $fn"); done
		                       pending=() ;;
	esac
done < <(go test -mod=vendor -list '^Fuzz' ./... 2>/dev/null)

if ((${#targets[@]} == 0)); then
	echo "fuzz.bash: no fuzz targets found"; exit 0
fi

fails=0
for t in "${targets[@]}"; do
	pkg="${t%% *}"; fn="${t##* }"
	printf '  %-52s %-20s ' "${pkg#github.com/jim-collier/nano-git-db/}" "${fn} (${fuzzTime})"
	if go test -mod=vendor -p "${NGDB_JOBS}" -run '^$' -fuzz="^${fn}\$" -fuzztime="${fuzzTime}" "${pkg}" >/tmp/ngdb-fuzz.$$ 2>&1; then
		echo "ok"
	else
		echo "CRASH"
		sed 's/^/      /' /tmp/ngdb-fuzz.$$
		fails=$((fails + 1))
	fi
done
rm -f /tmp/ngdb-fuzz.$$

((fails == 0)) || { echo "fuzz.bash: ${fails} target(s) crashed"; exit 1; }
echo "fuzz.bash: all targets clean"


##	Script history:
##		- 20260709: Created.
