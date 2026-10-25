#!/usr/bin/env bash

##	Purpose:
##		The build number, as app.BuildNumber gives it: the commit's time as whole
##		minutes since 2000-01-01 UTC, in lowercase Crockford base 32. For what
##		isn't the binary, such as the release notes. smoke-cli.bash checks the two
##		agree. Bash 3.2 safe, since smoke-cli.bash runs on the macOS box.
##		- fBuildNumber [SECS]: prints the number for unix seconds SECS, or for
##		  the HEAD commit of the repo it runs in. Fails on no git or bad input.
##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

fBuildNumber(){
	local secs="${1:-}" n=0 out='' digits="0123456789abcdefghjkmnpqrstvwxyz"
	[[ -n "${secs}" ]]  ||  secs="$(git log -1 --format=%ct 2> /dev/null)"  ||  return 1
	[[ "${secs}" =~ ^[0-9]+$ ]]  ||  return 1
	secs=$((10#${secs}))
	((secs >= 946684800))  ||  return 1
	n=$(( (secs - 946684800) / 60 ))
	while ((n > 0)); do out="${digits:n%32:1}${out}"; n=$((n / 32)); done
	echo "${out:-0}"
}


##	History:
##		- 20261025 JC: Created, for the version line in the release notes.
