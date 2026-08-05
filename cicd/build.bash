#!/usr/bin/env bash
# Two build modes:
#   (default) release: size-optimized, stripped (-s -w), -trimpath -> bin/ngdb.
#             What ships and what gets dogfooded; goreleaser rebuilds the same
#             flags per-target for packaging.
#   --debug   symbols kept (no -s -w, no -trimpath) -> bin/ngdb-debug. What the
#             test/profile stages drive when they want a readable stack.
# Cross-compile by exporting GOOS/GOARCH first; cross outputs get per-target
# names (bin/ngdb-<os>-<arch>[.exe]). Pure-Go only: CGO_ENABLED=0 keeps
# cross-compiles toolchain-free.
set -euo pipefail
source "$(dirname "$0")/utility/include/cpu-limit.bash"  ## NGDB_JOBS + GOMAXPROCS (<=half cores)
cd "$(dirname "$0")/.."  ## repo root; this script lives in cicd/

debug=0
[[ "${1:-}" == "--debug" ]] && debug=1

out="ngdb"
if [[ -n "${GOOS:-}${GOARCH:-}" ]] && [[ "${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}" != "$(go env GOHOSTOS)-$(go env GOHOSTARCH)" ]]; then
	out="ngdb-${GOOS:-$(go env GOHOSTOS)}-${GOARCH:-$(go env GOHOSTARCH)}"
	[[ "${GOOS:-}" == "windows" ]]  &&  out="${out}.exe"
elif ((debug)); then
	out="ngdb-debug"
fi

# app.Version is authoritative in source; we only stamp app.Build with provenance
# (short commit + -dirty), empty-safe so a release binary from a clean tree reads clean.
build="$(git rev-parse --short HEAD 2>/dev/null || true)"
[[ -n "${build}" && -n "$(git status --porcelain 2>/dev/null)" ]]  &&  build="${build}-dirty"

ldflags="-X github.com/jim-collier/nano-git-db/app.Build=${build}"
trim="-trimpath"
if ((debug)); then
	trim=""
else
	ldflags="-s -w ${ldflags}"
fi

# The module root lives under source/ (so vendored deps sit under source/ too).
# -mod=vendor: build only from the committed vendor/ tree, never the network.
mkdir -p bin
( cd source && CGO_ENABLED=0 go build -mod=vendor -p "${NGDB_JOBS}" ${trim} -ldflags="${ldflags}" -o "../bin/${out}" ./cmd/ngdb )
# ls, not du: on compressing filesystems (zfs) du reports allocated, not real, size.
echo "built: $(ls -lh "bin/${out}" | awk '{print $5}')	bin/${out}$( ((debug)) && echo "  (debug, unstripped)")"
