# Build and test tools for cicd.bash, which runs its build through security
# stages in this image unless given --no-container. Dogfood, profiler, demo,
# hooks and publish stay on the host.
#
# The tag is this file's git hash, so any edit here means a new image.
# GO_VER has to match the toolchain line in source/go.mod; cicd.bash checks.
#
# Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
# SPDX-License-Identifier: MIT

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f

# Apt is pinned only as far as the base image is. The Go toolchain is what
# matters, and it comes from the tarball below. nsis is for the Windows installer.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		bash ca-certificates curl git nsis \
	&& rm -rf /var/lib/apt/lists/*

ARG GO_VER=1.26.10
ARG GO_SHA256=ccfe7ae822d2c2f41f4fe7c956199967a4dac22d0ee0d2b2bfdb440aab3b20c5
RUN curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" \
	&& echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
	&& tar -C /opt -xzf /tmp/go.tgz \
	&& rm /tmp/go.tgz

# Runs as the caller's uid, which has no passwd entry here. /cache is where the
# named volume goes, for the module and build caches, and a new volume takes
# this dir's mode, so anyone can write.
RUN mkdir -m 1777 /cache /cache/home
ENV PATH=/opt/go/bin:${PATH} \
	HOME=/cache/home \
	GOTOOLCHAIN=local
WORKDIR /
