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

##	Copyright (c) 2026 Bubbles
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

source "$(dirname "$0")/include/cpu-limit.bash"   ## GITNIBLET_JOBS + GOMAXPROCS (caps fuzz workers)
fCpuLimit heavy  ## a quarter of the cores, busy or not
source "$(dirname "$0")/include/temp-run.bash"    ## fTempDir_Make + fSafeRm
repoRoot="$(realpath -e "$(dirname "$0")/../..")"
cd "${repoRoot}/source"   ## module root (go.mod)

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

declare -A testID=()
while read -r p n i; do testID["${p} ${n}"]="${i:------}"; done < <(GOFLAGS='' go -C "${repoRoot}/cicd/utility/testid" run . ids "${repoRoot}")

tmp="$(fTempDir_Make fuzz)"; trap 'fSafeRm "${tmp}" > /dev/null' EXIT
out="${tmp}/out"
fConfigHome_Use "${tmp}"

fRunTarget() {   ## $1 = import path, $2 = target name
	go test -mod=vendor -p "${GITNIBLET_JOBS}" -parallel "${GITNIBLET_JOBS}" -run '^$' -fuzz="^${2}\$" -fuzztime="${fuzzTime}" "$1" >"${out}" 2>&1
}

## go1.26's fuzzing coordinator can report the -fuzztime deadline itself as the
## failure: the deadline error escapes because the worker context has not been
## canceled yet when the event loop wakes (golang/go#75804, fixed in go1.27, no
## backport). A genuine crash always saves the offending input, so a bare
## "context deadline exceeded" with nothing written is the toolchain, not us.
## Drop this once the toolchain pin reaches 1.27.
fSpuriousDeadline() {
	grep -q 'context deadline exceeded' "${out}" && ! grep -q 'Failing input written to' "${out}"
}

fails=0
for t in "${targets[@]}"; do
	pkg="${t%% *}"; fn="${t##* }"
	note=""
	rc=0; fRunTarget "${pkg}" "${fn}" || rc=$?
	if ((rc != 0)) && fSpuriousDeadline; then
		note=" (deadline flake, rerun)"
		rc=0; fRunTarget "${pkg}" "${fn}" || rc=$?
	fi
	line="${testID["${t}"]:------} ${pkg#github.com/yottacore/gitniblet/} ${fn} (${fuzzTime})"
	if ((rc == 0)); then
		echo "PASS ${line}${note}"
	else
		echo "FAIL ${line}"
		sed 's/^/\t/' "${out}"
		fails=$((fails + 1))
	fi
done

((fails == 0)) || { echo "fuzz.bash: ${fails} target(s) crashed"; exit 1; }
echo "fuzz.bash: all targets clean"


##	Script history:
##		- 20260709: Created.
##		- 20260804: Rerun a target once when the fuzz budget expires as a failure.
##		- 20261025: Output goes in the run folder, not a fixed /tmp file.
##		- 20261025: A quarter of the cores, and the worker count passed explicitly.
##		- 20261025: One PASS or FAIL line per target, with its test ID.
##		- 20261026: Targets run with a fake home.
