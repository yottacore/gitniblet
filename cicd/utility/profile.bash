#!/usr/bin/env bash

##	Purpose:
##		Profile the CPU-hot path (tx-log replay: parse + decode + ordered SQL
##		apply), render an inferno flamegraph SVG into cicd/artifacts/profiling, GFS-
##		rotate the old ones, and print a hotspot summary (flame-report.py, plain
##		mode). A non-gating artifact stage: any tooling miss warns and skips rather
##		than failing the build. Never touches the .flame-seen marker (that gate is
##		only for flame-report.py --check at session startup).
##	Syntax:
##		profile.bash [--dir DIR] [--iters N]
##		  --dir DIR   output directory (default: cicd/artifacts/profiling)
##		  --iters N   benchmark replays to sample over (default: 30, ~7s)
##	Exit: 0 always (skips are non-fatal); the flamegraph may or may not be produced.
##	History: At bottom of script.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


set -Eeuo pipefail

meDir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
profDir="${meDir}/../artifacts/profiling"
iters=30

while (($#)); do case "$1" in
	--dir)      profDir="${2:?--dir needs a path}"; shift 2 ;;
	--dir=*)    profDir="${1#*=}"; shift ;;
	--iters)    iters="${2:?--iters needs a number}"; shift 2 ;;
	--iters=*)  iters="${1#*=}"; shift ;;
	-h|--help)  grep -E '^##' "$0" | sed 's/^##\t\?//'; exit 0 ;;
	*) echo "profile.bash: unknown option: $1" >&2; exit 2 ;;
esac; done

fSkip(){ echo "profile.bash: skipped: $1"; exit 0; }   ## non-gating artifact

## inferno-flamegraph renders the SVG; it installs to ~/.cargo/bin. Missing tool =>
## skip (the stage is optional).
inferno="$(command -v inferno-flamegraph || true)"
[[ -x "${inferno}" ]] || inferno="${HOME}/.cargo/bin/inferno-flamegraph"
[[ -x "${inferno}" ]] || fSkip "inferno-flamegraph not found (cargo install inferno)"
command -v python3 >/dev/null || fSkip "python3 not found"

source "${meDir}/include/cpu-limit.bash"   ## NGDB_JOBS + GOMAXPROCS (<=half cores)
source "${meDir}/include/gfs-rotate.bash"

cd "${meDir}/../../source"                              ## module root (go.mod)
stamp="$(date +%Y%m%d-%H%M%S)"
tmp="$(mktemp -d)"; trap 'rm -rf "${tmp}"' EXIT

## Sample the replay benchmark. A build/run failure here is an app problem, but
## the stage stays non-gating - it warns and skips so a profiler hiccup never
## blocks a publish (tests already gate correctness).
if ! go test -mod=vendor -p "${NGDB_JOBS}" -run '^$' -bench '^BenchmarkReplay$' -benchtime="${iters}x" \
	-cpuprofile="${tmp}/cpu.prof" -o "${tmp}/bench.test" ./internal/core/crud/ >"${tmp}/bench.log" 2>&1; then
	sed 's/^/  /' "${tmp}/bench.log"
	fSkip "benchmark run failed"
fi

mkdir -p "${profDir}"
out="${profDir}/flame_${stamp}_frequent.svg"
if ! go tool pprof -traces "${tmp}/bench.test" "${tmp}/cpu.prof" 2>/dev/null \
	| python3 "${meDir}/pprof-to-folded.py" \
	| "${inferno}" --title "nano-git-db tx-log replay (${iters}x)" >"${out}" 2>/dev/null; then
	rm -f "${out}"; fSkip "flamegraph render failed"
fi
[[ -s "${out}" ]] || { rm -f "${out}"; fSkip "empty flamegraph"; }

## || true keeps set -e suspended through gfs_rotate (its internal globbing/shopt
## returns non-zero on empty matches, which would otherwise abort here).
gfs_rotate "${profDir}" flame svg || true
## The just-written file keeps its timestamp; rotation only changes the role
## suffix, so resolve the actual name rather than assuming one.
newest="$(ls -1 "${profDir}"/flame_"${stamp}"_*.svg 2>/dev/null | head -1)"
echo "flamegraph: ${newest:-${out}}"
echo

## Plain-mode hotspot summary next to the new flamegraph (does NOT record .flame-seen).
python3 "${meDir}/flame-report.py" --dir "${profDir}" || echo "profile.bash: hotspot report unavailable"


##	Script history:
##		- 20260709: Created.
