#!/usr/bin/env bash
# Build the single-file Windows .exe installer(s) from the goreleaser-built
# binaries in dist/. One setup.exe per windows arch. No-op-with-warning when
# makensis is absent (same tooling-miss pattern as the profiler stage), so a
# box without NSIS still finishes the run - CI installs nsis before calling this.
#
# Usage: windows-installer.bash <version>   (run from repo root, after goreleaser)

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT
set -euo pipefail
cd "$(dirname "$0")/../.."  ## repo root

version="${1:-0.0.0}"
nsi="cicd/utility/windows-installer.nsi"

if ! command -v makensis >/dev/null 2>&1; then
	echo "windows-installer: makensis not found - skipping (install 'nsis' to build the .exe installer)"
	exit 0
fi

made=0
for exe in dist/*windows*/ngdb.exe; do
	[[ -f "${exe}" ]] || continue
	arch="$(basename "$(dirname "${exe}")" | grep -oP 'windows_\K(amd64|arm64)' || true)"
	[[ -n "${arch}" ]] || { echo "windows-installer: cannot derive arch from ${exe} - skipping"; continue; }
	out="dist/ngdb-${version}-windows-${arch}-setup.exe"
	# Absolute paths: makensis resolves File/OutFile against its own CWD, not the
	# script dir, so relative dist/ paths would miss.
	makensis -V2 \
		-DARCH="${arch}" -DVERSION="${version}" \
		-DBINARY="$(realpath "${exe}")" -DOUTFILE="$(realpath -m "${out}")" \
		"${nsi}"
	echo "windows-installer: $(ls -lh "${out}" | awk '{print $5}')	${out}"
	made=$((made + 1))
done

[[ "${made}" -gt 0 ]] || echo "windows-installer: no dist/*windows*/ngdb.exe found (run goreleaser first)"
