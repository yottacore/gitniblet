#!/bin/bash
#  shellcheck disable=2001  ## 'See if you can use ${variable//search/replace} instead.' Complains about good uses of sed.
#  shellcheck disable=2016  ## 'Expressions don't expand in single quotes, use double quotes for that.' I know, and I often want an explicit '$'.
#  shellcheck disable=2034  ## 'variable appears unused.' Complains about valid use of variable indirection (e.g. later use of local -n var=$1)
#  shellcheck disable=2046  ## 'Quote to prevent word-splitting.' (OK for integers.)
#  shellcheck disable=2086  ## 'Double quote to prevent globbing and word splitting.' (OK for integers.)
#  shellcheck disable=2119  ## 'Use foo "$@" if function's $1 should mean script's $1.' Confusing and inapplicable.
#  shellcheck disable=2120  ## 'Foo references arguments, but none are ever passed.' Valid function argument overloading.
#  shellcheck disable=2128  ## 'Expanding an array without an index only gives the element in the index 0.' False hits on associative arrays.
#  shellcheck disable=2155  ## 'Declare and assign separately to avoid masking return values.' Cumbersome and unnecessary. For integers it's sometimes required to even come into existence for counters.
#  shellcheck disable=2162  ## 'read without -r will mangle backslashes.'
#  shellcheck disable=2178  ## 'Variable was used as an array but is now assigned a string.' False hits on associative arrays with e.g. 'local -n assocArray=$1'.
#  shellcheck disable=2181  ## 'Check exit code directly, not indirectly with $?.'
#  shellcheck disable=2317  ## 'Can't reach.' (I.e. an 'exit' is used for debugging - and makes an unusable visual mess.)
## shellcheck disable=2002  ## 'Useless use of cat.'
## shellcheck disable=2004  ## '$/${} is unnecessary on arithmetic variables.' Inappropriate complaining?
## shellcheck disable=2053  ## 'Quote the right-hand sid of = in [[ ]] to prevent glob matching.' Disable for Yoda Notation.
## shellcheck disable=2143  ## 'Use grep -q instead of echo | grep'

##	Purpose: Wrapper for build, test, copy to local for dogfood, push to github. Calls test.sh, no need to call that separately.
##	History: At bottom of this file. (Note: History for this is maintained outside of [or in addition to] git project.)

##	Copyright © 2022-2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under the GNU General Public License v2.0 or later. Full text at:
##		https://spdx.org/licenses/GPL-2.0-or-later.html
##	SPDX-License-Identifier: GPL-2.0-or-later


#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Constants
if [[ -z "${doQuietly+x}" ]]; then

	## Settings (relative paths defined here will be verified and resolved later)
	declare -ri isCompileProject=1  ## 1: E.g. C++, Rust, Go, etc.  0: E.g. Python, Bash, etc.
	declare -r  exeName="ngdb"
	declare     dirPath_Base=".."
	declare     dirPath_Source="${dirPath_Base}/source"
	declare     filePath_ExecToTestAndInstall_BuildLocation="${dirPath_Base}/bin/${exeName}"  ## cicd/build.bash outputs straight to bin/
	declare     filePath_ExecToTestAndInstall_FinalHome="${dirPath_Base}/bin/${exeName}"
	declare     filePath_CICD_TestExec="${dirPath_Base}/cicd/test.bash"
	declare     gitAutomationScript="${dirPath_Base}/cicd/utility/n8git_backup-and-publish"
	declare -ra preferredInstallPaths_Bash=("${HOME}/synced/0-0/common/exec/util/linux/bash"                    "/usr/local/sbin/"                                )  ## First one that exists, wins
	declare -ra preferredInstallPaths_Linux_x8664=("${HOME}/synced/0-0/common/exec/util/linux/bin"              "/usr/local/sbin/"                                )  ## First one that exists, wins

	## Generic constants
	declare  -i doQuietly=0
	declare  -i doQuick=0  ## --quick: skip the slow stages (cross-builds, fuzz, profiler, govulncheck, gosec)
	declare  -i doNoArm=0  ## --no-arm: build/package amd64 only (arm64 is cheap here, so this is just a run-time trim)
	declare  -i doPromptToContinue=1
	declare -r  thisVersion="1.0.0-beta3"         ## Put you script's semantic version here.
	declare -r  thisBuild="1mzfd9c"
	declare -r  thisCopyrightYear="2026"           ## Put your copyright date here.
	declare -r  thisAuthor="Jim Collier"           ## Put your copyright name here.
	declare -ri atLeastOneArgRequired=0
	declare -ri doAsSudo=0
	declare  -i wasShown_Version=0  wasShown_Copyright=0  wasShown_About=0  wasShown_Syntax=0
fi


##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


## Version, copyright, about, syntax (minified but not obfuscated)
## Note: Echoing rather than HEREDOC is preferrable because - while slower - that's not
##       an issue in this context, and more importantly, HEREDOC is too hard to manage
##       indentation, esp. for the fSyntax() section.

fVersion(){ { ((doQuietly)) || ((wasShown_Version)); } && return; wasShown_Version=1;
	fEcho_Clean "${meName} v${thisVersion} build ${thisBuild}" ;:;}

fCopyright(){ { ((doQuietly)) || ((wasShown_Copyright)); } && return; wasShown_Copyright=1; wasShown_Version=1
	fEcho_Clean ""
	## Don't show version info, because it can confuse user with the version of the product being built.
	fEcho_Clean "${meName}, Copyright © ${thisCopyrightYear} ${thisAuthor}."
	fEcho_Clean "Licensed under The MIT License (MIT). Full text at:"
	fEcho_Clean "  https://mit-license.org/"
	fEcho_Clean "No warranty."
	fEcho_Clean "" ;:;}

fAbout(){ { ((doQuietly)) || ((wasShown_About)); } && return; wasShown_About=1;
	fEcho_Clean ""
	#           X-------------------------------------------------------------------------------X
	fEcho_Clean "CI/CD and dogfood:"
	if ((isCompileProject)); then
		fEcho_Clean "  • Builds the program. If successful:"
		fEcho_Clean "  • Cross-compile more versions. If those succeed:"
	fi
	fEcho_Clean "  • Run automated tests. If tests pass:"
	fEcho_Clean "  • Update locally-installed version to what was just compiled for dogfood."
	fEcho_Clean "  • Run git automation script (e.g. commit and push)."
	#           X-------------------------------------------------------------------------------X
	fEcho_Clean "" ;:;}

fSyntax(){  { ((doQuietly)) || ((wasShown_Syntax)); } && return; wasShown_Syntax=1;
	fEcho_Clean ""
	#           X-------------------------------------------------------------------------------X
	fEcho_Clean "Usage: ${meName} [options]"
	fEcho_Clean "  -q, --quiet         Run unattended (no prompt) and quiet; flows to publish."
	fEcho_Clean "      --quick         Skip the slow stages: cross-builds, packaging, fuzz,"
	fEcho_Clean "                      profiler, demo recording, govulncheck + gosec."
	fEcho_Clean "      --no-arm        Build/package amd64 only (skip arm64) to trim a full run."
	fEcho_Clean "  -m, --message MSG   Commit message for publish (also --msg; -m MSG or -m=MSG)."
	fEcho_Clean "                      With -q and no -m, a message is auto-generated."
	fEcho_Clean "  -h, --help          Show this."
	fEcho_Clean "  -v, --version       Show version."
	#           X-------------------------------------------------------------------------------X
	fEcho_Clean "" ;:;}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
fMain(){

	## Arguments: -q/--quiet (unattended + quiet, flows to publish), -m/--message
	## (hands-off commit message; also --msg, accepts -m MSG or -m=MSG).
	local cliMessage=""
	while (($#)); do case "${1}" in
		-q|--quiet)                doQuietly=1                          ; shift ;;
		--quick)                   doQuick=1                            ; shift ;;
		--no-arm)                  doNoArm=1                            ; shift ;;
		--message=*|--msg=*|-m=*)  cliMessage="${1#*=}"                 ; shift ;;
		-m|--message|--msg)        cliMessage="${2-}"; shift; (($#)) && shift ;;
		-h|--help)                 fCopyright; fAbout; fSyntax          ; return 0 ;;
		-v|--version)              fVersion                             ; return 0 ;;
		*) fThrowError "Unknown option: '${1}' (try --help)." ;;
	esac; done

	## Commit message: -m wins; else auto-generate when running quiet/unattended.
	## An empty message when interactive lets the publish step open an editor.
	local commitMessage="${cliMessage}"
	if [[ -z "${commitMessage}" ]] && ((doQuietly)); then commitMessage="Updated"; fi

	# Validate dependencies
	fMustBeInPath realpath
	fMustBeInPath trash

	## Resolve paths
	fResolvePath  dirPath_Base                                 "${dirPath_Base}"                                   ; readonly dirPath_Base
	fResolvePath  dirPath_Source                               "${dirPath_Source}"                                 ; readonly dirPath_Source
	fResolvePath  filePath_CICD_TestExec                       "${filePath_CICD_TestExec}"                         ; readonly filePath_CICD_TestExec
	fResolvePath  gitAutomationScript                          "${gitAutomationScript}"                            ; readonly gitAutomationScript
	fResolvePath  filePath_ExecToTestAndInstall_BuildLocation  "${filePath_ExecToTestAndInstall_BuildLocation}"  0 ; readonly filePath_ExecToTestAndInstall_BuildLocation
	fResolvePath  filePath_ExecToTestAndInstall_FinalHome      "${filePath_ExecToTestAndInstall_FinalHome}"      0 ; readonly filePath_ExecToTestAndInstall_FinalHome

	## Validate
	[[ -d "${dirPath_Base}"            ]]  ||  fThrowError "Path not found: '${dirPath_Base}'"
	[[ -d "${dirPath_Source}"          ]]  ||  fThrowError "Path not found: '${dirPath_Source}'"
	[[ -f "${filePath_CICD_TestExec}"  ]]  ||  fThrowError "File not found: '${filePath_CICD_TestExec}'"
	[[ -n "${gitAutomationScript}"     ]]  ||  fThrowError "Git automation script not found where specified or in path: '${gitAutomationScript}'."

	## Prompt to continue
	if ((! doQuietly)); then
		fCopyright
		fAbout
		fEcho_Clean
		fEcho_Clean "Source directory .............: ${dirPath_Source}"
		if ((isCompileProject)); then
		fEcho_Clean "Executable to build ..........: ${filePath_ExecToTestAndInstall_BuildLocation}"
		fEcho_Clean "Executable final location ....: ${filePath_ExecToTestAndInstall_FinalHome}"
		fi
		fEcho_Clean "Test script ..................: ${filePath_CICD_TestExec}"
		if ((doQuick)); then
		fEcho_Clean "Quick mode ...................: cross-builds + packaging + fuzz + profiler + demo + govulncheck + gosec skipped"
		fi
		if ((doNoArm)); then
		fEcho_Clean "No-arm mode ..................: arm64 builds + packages skipped (amd64 only)"
		fi
		fEcho_Clean "Git commit and push script ...: ${gitAutomationScript}"
		if [[ -n "${commitMessage}" ]]; then
		fEcho_Clean "Publish commit message .......: \"${commitMessage}\""
		else
		fEcho_Clean "Publish commit message .......: (prompt below; blank = editor)"
		fi
		fEcho_Clean

		## Capture the commit message up front so the rest of the run finishes
		## unattended. This doubles as the go/no-go gate: Ctrl+C here aborts (there
		## is no separate y/n prompt). A blank reply leaves the editor to open at
		## publish; -m/-q skip this entirely.
		if [[ -z "${commitMessage}" ]]; then
			read -r -p "Publish commit message (blank = editor; Ctrl+C aborts): " reply
			fEcho_ResetBlankCounter
			[[ -n "${reply}" ]]  &&  commitMessage="${reply}"
		fi
		fEcho_Clean
	fi

	####
	#### MAKEITSO
	####

	cd "${dirPath_Base}"

	## Capture the whole run to a rotated log so warnings from any stage (build,
	## vet, staticcheck, govulncheck) can be reviewed after the fact - lint-report.bash
	## reads the newest. Rotate the dir first, then tee. Best-effort: if the log can't
	## be created the run just goes uncaptured.
	declare dirPath_ArtifactsLint="${dirPath_Base}/cicd/artifacts/lint"
	if mkdir -p "${dirPath_ArtifactsLint}" 2>/dev/null; then
		gfs_rotate "${dirPath_ArtifactsLint}" run log >/dev/null 2>&1 || true
		exec > >(tee "${dirPath_ArtifactsLint}/run_${serialDT}.log") 2>&1
	fi

	pushd "${dirPath_Source}" 1>/dev/null

	if ((isCompileProject)); then

		## Native debug build (symbols kept, unstripped) - drives the smoke check and
		## is the artifact for manual profiling/debugging. The test, fuzz, and profiler
		## stages below compile their own debug/bench binaries via `go test`.
		fEcho_Section "Build (native debug)"
		"${dirPath_Base}/cicd/build.bash" --debug
		## Minimal execution test: --version prints one line and exits, never opening the
		## TUI. Running the app for real stays a separate manual step before the merge.
		fEcho_Clean "version check (debug):"
		"${dirPath_Base}/bin/ngdb-debug" --version

		## Native release build (size-optimized, stripped) - what gets dogfooded and
		## screenshotted; goreleaser rebuilds the same optimized flags per-target below.
		fEcho_Section "Build (native release, size-optimized)"
		"${dirPath_Base}/cicd/build.bash"
		fEcho_Clean "version check (release):"
		"${filePath_ExecToTestAndInstall_BuildLocation}" --version
		fEcho_Clean "Built ${filePath_ExecToTestAndInstall_BuildLocation}"

		## Bump guard: app.Version is authoritative in source; a release tags v<Version>.
		## If that tag already exists the version wasn't bumped since the last release -
		## warn (non-fatal) so a merge to main doesn't try to re-cut an existing version.
		srcVersion="$(grep -oP 'var Version = "\K[^"]+' "${dirPath_Source}/app/app.go" 2>/dev/null || true)"
		if [[ -n "${srcVersion}" ]] && git rev-parse -q --verify "refs/tags/v${srcVersion}" >/dev/null 2>&1; then
			fEcho_Clean "WARNING: app.Version ${srcVersion} already tagged (v${srcVersion}) - bump it in source/app/app.go before releasing."
		fi
		## The README release badge is dynamic (tracks the release tag). If a hardcoded
		## version badge is ever added, it must match app.Version - the release enforces
		## this, warn locally too. (Skips cleanly when neither file/badge is present.)
		badgeVersion="$(grep -oiP 'badge/[Vv]ersion-\K[0-9]+\.[0-9]+\.[0-9]+' "${dirPath_Base}/README.md" 2>/dev/null | head -1 || true)"
		if [[ -n "${badgeVersion}" && "${badgeVersion}" != "${srcVersion}" ]]; then
			fEcho_Clean "WARNING: README version badge ${badgeVersion} != app.Version ${srcVersion}."
		fi

		## Cross-compile + package via goreleaser (the same .goreleaser.yaml the release
		## uses): binaries for linux/darwin/windows/freebsd (amd64+arm64), .deb/.rpm for
		## linux, a tar.gz/zip per target, and checksums - all into dist/. --snapshot =
		## build + package locally, no publish. Then the Windows single-file .exe
		## installer(s) via NSIS. Skipped under --quick; --no-arm filters the goarch line
		## to amd64 only. Deferred (need the target OS's own host): macOS .dmg, Windows
		## .msi, FreeBSD .pkg - those ship as archives for now.
		if ((doQuick)); then
			fEcho_Section "Cross-compile + package (skipped: --quick)"
		else
			fEcho_Section "Cross-compile + package (goreleaser: binaries, .deb/.rpm, archives, checksums)"
			local grConfig=".goreleaser.yaml"
			if ((doNoArm)); then
				grConfig="${dirPath_Base}/cicd/artifacts/goreleaser-noarm.yaml"
				mkdir -p "$(dirname "${grConfig}")"
				sed 's/\[amd64, arm64\]/[amd64]/' "${dirPath_Base}/.goreleaser.yaml" > "${grConfig}"
				fEcho_Clean "arm64 skipped (--no-arm): amd64-only matrix"
			fi
			( cd "${dirPath_Base}"  &&  GOFLAGS=  go run "${NGDB_GORELEASER}" release --snapshot --clean --config "${grConfig}" )
			fEcho_Clean "Packaged into ${dirPath_Base}/dist/ (binaries, .deb/.rpm, archives, checksums)"
			## Windows single-file .exe installer(s) from the just-built binaries (NSIS).
			( cd "${dirPath_Base}"  &&  "${dirPath_Base}/cicd/utility/windows-installer.bash" "${srcVersion:-0.0.0}" )
		fi

	fi

	## Lint + format. gofmt rewrites Go source in place (the language standard;
	## Bash is never auto-formatted). staticcheck is the deeper linter and resolves
	## its own module, so it runs outside vendor mode. go vet lives in test.bash.
	## Under --quick, staticcheck's network fetch is skipped (gofmt still runs).
	fEcho_Section "Lint + format"
	( cd "${dirPath_Source}"  &&  gofmt -w cmd internal )
	fEcho_Clean "gofmt: formatted in place"
	if ((doQuick)); then
		fEcho_Clean "staticcheck skipped (--quick)"
	else
		( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${NGDB_STATICCHECK}" ./... )
		fEcho_Clean "staticcheck: clean"
	fi

	## Test
	fEcho_Section "Tests"
	"${filePath_CICD_TestExec}"
	fEcho_Clean "Tests passed"

	## Fuzz the parsers/decoders with mutated input. The seed corpus already runs
	## inside test.bash (every FuzzXxx executes its seeds as a normal test); this is
	## the mutation budget, which is the slow part, so --quick skips it. A crash
	## fails the run and the minimized input lands under the package's testdata/.
	if ((doQuick)); then
		fEcho_Section "Fuzz (skipped: --quick)"
	else
		fEcho_Section "Fuzz"
		"${dirPath_Base}/cicd/utility/fuzz.bash" --time 10s
	fi

	## Security + supply-chain. go mod verify is quick and offline; govulncheck and
	## gosec each pull a tool over the network and are the slow ones, so --quick
	## skips them (the offline checks all live in test.bash). Real findings fail.
	##
	## gosec excludes below are all justified for this local-first, single-user,
	## git-synced tool:
	##   G104            unchecked errors on cleanup/stderr paths - idiomatic Go
	##   G204            subprocess with variable args - we shell out to git by design
	##   G304/G703       open user-named DDL/log/config paths - that IS the program
	##   G301/G302/G306  file/dir perms - the tx-log is git-committed and readable by
	##                   design; real secrets (webusers.shcl, keys) write 0600 at their sites
	##   G202            SQL identifier concat - SQLite can't bind identifiers; all pass
	##                   through quoteIdent and every value binds as ? (see txlog.applyOne)
	##   G203            template.HTML/CSS - only wraps dev-computed layout CSS, never user input
	##   G124            cookie Secure - set under HTTPS; local mode is loopback-only HTTP
	##   G101            false positive on a filename constant
	fEcho_Section "Security + supply-chain (go mod verify$( ((doQuick)) || echo " + govulncheck + gosec"))"
	( cd "${dirPath_Source}"  &&  go mod verify )
	if ((doQuick)); then
		fEcho_Clean "Module verified (govulncheck + gosec skipped: --quick)"
	else
		( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${NGDB_GOVULNCHECK}" ./... )
		fEcho_Clean "No known vulnerabilities (govulncheck)"
		( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${NGDB_GOSEC}" \
			-exclude=G101,G104,G124,G202,G203,G204,G301,G302,G304,G306,G703 -quiet ./... )
		fEcho_Clean "No security findings (gosec)"
	fi

	popd 1>/dev/null

		fEcho_Section "Dogfood (install locally)"

	## Linux x86_64
	for nextPath in "${preferredInstallPaths_Linux_x8664[@]}"; do
		if [[ -d "${nextPath}" ]]; then
			fEcho_Clean "copying ${filePath_ExecToTestAndInstall_FinalHome} -> ${nextPath%%/}/"
			if { ! cp -av --update=older --reflink=auto "${filePath_ExecToTestAndInstall_FinalHome}"  "${nextPath%%/}/"; }  &&  [[ "${nextPath}" != "${HOME}/"* ]]; then
				sudo cp -av --update=older --reflink=auto "${filePath_ExecToTestAndInstall_FinalHome}"  "${nextPath%%/}/"
			fi
			ls  -lA  --color=always  --human-readable  --time-style=+"%Y-%m-%d %H:%M:%S"  "$(which "$(basename "${filePath_ExecToTestAndInstall_FinalHome}")")"
			fEcho_Clean "Installed (linux x86_64) -> ${nextPath%%/}/"
			break
		fi
	done;:

	## Profiler: sample the CPU-hot replay path, render a flamegraph into
	## cicd/artifacts/profiling, GFS-rotate the old ones, and print the hotspot
	## summary (flame-report.py, plain mode) into the log. Non-gating artifact -
	## any tooling miss warns and skips. Slow, so --quick skips it. cicd never
	## touches the .flame-seen marker; that startup gate is flame-report.py --check.
	if ((doQuick)); then
		fEcho_Section "Profiler (skipped: --quick)"
	elif [[ -x "${dirPath_Base}/cicd/utility/profile.bash" ]]; then
		fEcho_Section "Profiler (flamegraph + hotspots)"
		"${dirPath_Base}/cicd/utility/profile.bash"
	fi

	## Screenshots stage disabled - the animated demo gif at the top of the README
	## covers this, so the static thumbnails aren't worth restaging every build.
	## Left here (commented) in case per-shot thumbnails are wanted again later.
	#if ((doQuick)); then
	#	fEcho_Section "Screenshots (skipped: --quick)"
	#elif [[ -x "${dirPath_Base}/utility/screenshots.bash" ]]; then
	#	fEcho_Section "Screenshots"
	#	"${dirPath_Base}/utility/screenshots.bash" --no-build
	#	fEcho_Clean "Screenshots regenerated"
	#fi

	## Demo recording: mp4 + looping gif of the TUI-then-CLI walkthrough. mp4 + the
	## full gif land in ../private/demo-video/{video,gif} (GFS-rotated, not
	## committed); the latest gif is copied to assets/demo.gif for the README.
	## Slow (headless capture + two encodes), so --quick skips it.
	if ((doQuick)); then
		fEcho_Section "Demo recording (skipped: --quick)"
	elif [[ -x "${dirPath_Base}/cicd/utility/demo-video/demo-video.py" ]]; then
		fEcho_Section "Demo recording"
		python3 "${dirPath_Base}/cicd/utility/demo-video/demo-video.py" --no-build
		fEcho_Clean "Demo mp4 + gif regenerated"
	fi

	## Optional local pre-publish hooks (not committed, kept under ../private). Each
	## *.bash there runs here before publish so anything a hook regenerates gets
	## committed in the same run. No-op when the folder is absent. A hook gets the
	## repo base path as $1; a failing hook warns but does not abort the run.
	declare hooksDir="${dirPath_Base}/../private/cicd-hooks"
	if [[ -d "${hooksDir}" ]]; then
		fEcho_Section "Local hooks"
		for nextHook in "${hooksDir}"/*.bash; do
			[[ -f "${nextHook}" ]]  ||  continue
			fEcho_Clean "running $(basename "${nextHook}")"
			bash "${nextHook}" "${dirPath_Base}"  ||  fEcho_Clean "hook failed (continuing): $(basename "${nextHook}")"
		done
	fi

	## Git automation script (backup, commit, push). Flow the quiet flag and the
	## resolved commit message through so an unattended run stays hands-off.
	fEcho_Section "Backup + publish"
	## doPromptToContinue=0: cicd already gated up front (message captured above),
	## so the publish step keeps its remote/branch/SSH review but skips a second
	## y/n. Set as a settings variable, not a switch - that script is kept
	## identical across projects and takes no flag for this.
	local -a gitArgs=()
	if ((doQuietly));                then gitArgs+=(--quiet); fi
	if [[ -n "${commitMessage}" ]];  then gitArgs+=("--message=${commitMessage}"); fi
	doPromptToContinue=0  "${gitAutomationScript}"  "${gitArgs[@]}"

	## Done
	fEcho_Section "${exeName} CI/CD: done."
}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
fCleanup(){
	## Always leave a trailing blank line for breathing room before the prompt
	## (fEcho_Clean collapses it if the last line was already blank).
	fEcho_Clean
}








#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Generic functions
fResolvePath(){
	##	Purpose:
	##		- Resolves an argument to a canonical full path, while being careful to not be too broad as to resolve to something else with the same name.
	##		- Resolution priority:
	##			- Exactly as specified.
	##			- "[this script's path]/lib/[specified name if given without a path]"
	##			- "[this script's path]/include/[specified name if given without a path]"
	##			- "[this script's path]/includes/[specified name if given without a path]"
	##			- If specified a name without a path: Find in $PATH
	##			- If doesn't have to exist, and still haven't found it, then just canonicalize original argument
	local -n parentVarName_ResolvedPath_t4rej=${1:-}  ; shift || true  ## Parent variable to store fully resolved path in.
	local    nameOrPath="${1:-}"                      ; shift || true  ## File or folder path (relative or absolute). If an executable file, can be just a name to search in $PATH, to fully resolve.
	local -i mustExist=${1:-0}                        ; shift || true  ## 1 [default]: path must exist or error occurs. 0: Just rationalize paths, doesn't have to exist.
	[[   -z "${nameOrPath}" ]]  &&  { echo -e "\nError in $(basename "${BASH_SOURCE[0]}")·${FUNCNAME[0]}(): No file or directory specified to resolve.\n"; fEcho_WasLastEchoBlank_Set 1; return 1; }
	local -r mePath_t4rmy="$(dirname "${BASH_SOURCE[0]}")"
	local -i isExeWithNoPath=0 ; [[ "${nameOrPath}" == "$(basename "${nameOrPath}")" ]] && isExeWithNoPath=1 ; readonly isExeWithNoPath
	local    testPath="${nameOrPath}"
	{ [[ ! -e "${testPath}"   ]]                          ; }  &&  testPath="${mePath_t4rmy}/${nameOrPath}"
	{ [[ ! -e "${testPath}"   ]] && ((isExeWithNoPath))   ; }  &&  testPath="${mePath_t4rmy}/lib/${nameOrPath}"
	{ [[ ! -e "${testPath}"   ]] && ((isExeWithNoPath))   ; }  &&  testPath="${mePath_t4rmy}/include/${nameOrPath}"
	{ [[ ! -e "${testPath}"   ]] && ((isExeWithNoPath))   ; }  &&  testPath="${mePath_t4rmy}/includes/${nameOrPath}"
	{ [[ ! -e "${testPath}"   ]] && ((isExeWithNoPath))   ; }  &&  testPath="$(which "${nameOrPath}" 2>/dev/null || true)"
	{ [[ ! -e "${testPath}"   ]] && ((mustExist))         ; }  &&  { echo -e "\nError in $(basename "${BASH_SOURCE[0]}")·${FUNCNAME[0]}(): Could not resolve path '${nameOrPath}' [£ǝŔc].\n"; fEcho_WasLastEchoBlank_Set 1; return 1; }
	{ [[ ! -e "${testPath}"   ]] || [[ -z "${testPath}" ]]; }  &&  testPath="${nameOrPath}"  ## Revert to original definition
	if ((mustExist)); then testPath="$(realpath -e "${testPath}" 2>/dev/null || true)"
	else                   testPath="$(realpath -m "${testPath}" 2>/dev/null || true)"; fi
	## Last check to fail on
	{ [[ -z "${testPath}" ]] || { [[ ! -e "${testPath}" ]] && ((mustExist)); }; }  &&  { echo -e "\nError in $(basename "${BASH_SOURCE[0]}")·${FUNCNAME[0]}(): Could not resolve path '${nameOrPath}' [£ǝŔs].\n"; fEcho_WasLastEchoBlank_Set 1; return 1; }
	## Success
	#echo "testPath: '${testPath}'"
	#fPressAnyKeyToContinue
	parentVarName_ResolvedPath_t4rej="${testPath}"
}
fDoesDirHaveContents(){
	[[   -z "${1}" ]]  &&  fThrowError  "No directory specified as arg 1."  "fDoesDirHaveContents"
	[[ ! -d "${1}" ]]                                     && return 1
	[[   -z "$(ls -1A "${1%%/}/" 2>/dev/null || true)" ]] && return 1
	return 0; }
fBuildQuotedParams(){
	local -n varName_1mtkp9p=$1 ; shift || true
	local -i maxIdx=0
	for i in {1..32}; do [[ -n "${!i}" ]] && maxIdx=$i; done;:
	for i in $(seq 1 $maxIdx); do ((i > 1)) && varName_1mtkp9p="${varName_1mtkp9p}  "; varName_1mtkp9p="${varName_1mtkp9p}\"${!i}\""; done;:; }
fRunGUI(){ #( (nohup "$*" &>/dev/null) & disown);
	local -r toExec="${1}" ; shift || true
	local quotedParams=""; fBuildQuotedParams  quotedParams   "${1}"  "${2}"  "${3}"  "${4}"  "${5}"  "${6}"  "${7}"  "${8}"  "${9}"  "${10}"  "${11}"  "${12}"  "${13}"  "${14}"  "${15}"  "${16}"  "${17}"  "${18}"  "${19}"  "${20}"  "${21}"  "${22}"  "${23}"  "${24}"  "${25}"  "${26}"  "${27}"  "${28}"  "${29}"  "${30}"  "${31}"  "${32}"
	( (eval "'${toExec}'  ${quotedParams}  &>/dev/null") & disown ) &>/dev/null; }
fRunGuiAsSudo(){
	local -r toExec="${1:-}"  ## Only used for testing validity. When executed, it's just another "parameter".
	[[ -z "${toExec}" ]]                                                                &&  { echo -e "\nError in '$(basename "${0}").${FUNCNAME[0]}.()': No GUI executable specified to run. \n"                          ; exit 1; }
	{ [[ ! -x "${toExec}" ]] && [[ -z "$(which "${toExec}" 2>/dev/null || true)" ]]; }  &&  { echo -e "\nError in '$(basename "${0}").${FUNCNAME[0]}.()': Cannot find executable explicitly or in \$PATH: '${toExec}'. \n" ; exit 1; }
	local quotedParams=""; fBuildQuotedParams  quotedParams   "${1}"  "${2}"  "${3}"  "${4}"  "${5}"  "${6}"  "${7}"  "${8}"  "${9}"  "${10}"  "${11}"  "${12}"  "${13}"  "${14}"  "${15}"  "${16}"  "${17}"  "${18}"  "${19}"  "${20}"  "${21}"  "${22}"  "${23}"  "${24}"  "${25}"  "${26}"  "${27}"  "${28}"  "${29}"  "${30}"  "${31}"  "${32}"
	sudo true; ( (eval "sudo  DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/0/bus XDG_RUNTIME_DIR=/run/user/0  ${quotedParams}  &>/dev/null") & disown ) &>/dev/null; }
fMustBeInPath(){
	##	Unit tests passed on: 20250704.
	local -r programToCheckForInPath="${1:-}"
	if [[ -z "${programToCheckForInPath}" ]]; then
		fThrowError "Not program specified."  "${FUNCNAME[0]}"; return 1
	elif [[ -z "$(which ${programToCheckForInPath} 2>/dev/null || true)" ]]; then
		fThrowError "Not found in path: ${programToCheckForInPath}"; return 1
	fi ;:;}
fIntroPromptToContinue(){
	{ ((doQuietly)) || ((! doPromptToContinue)); } && return 0
	local -r extraInfoString="${1:-}"
	{ fEcho_Clean; fCopyright; fAbout; fEcho_Clean; }
	[[ -n "${extraInfoString}" ]]  &&  { fEcho_Clean; fEcho_Clean "${extraInfoString}"; }
	fPromptYN "Continue? (y|n): "  ||  { fEcho "User aborted."; return 1; }; }
fPromptYN(){
	((doQuietly)) && return 0
	local promptStr="${1:-}"
	[[ -z "${promptStr}" ]] && promptStr="Continue? (y|n): "
	read -r -p "${promptStr}" userAnswer
	fEcho_ResetBlankCounter
	{ [[ "${userAnswer,,}" == "y" ]] && return 0; } || return 1; }
fPressAnyKeyToContinue(){
	((doQuietly)) && return 0
	local promptStr="${1:-}"
	[[ -z "${promptStr}" ]] && promptStr="Press any key to continue ..."
	read -n 1 -s -p "${promptStr}" userAnswer
	fEcho_Clean_Force
	}

## Echo-related (minified but not obfuscated)
declare -gi _wasLastEchoBlank=0
declare -gi _isEchoInRawInlineMode=0
fEcho_ResetBlankCounter()     { _wasLastEchoBlank=0;      }
fEcho_WasLastEchoBlank_Set()  { { [[ "${1:-}" == "1" ]] && _wasLastEchoBlank=1; } || _wasLastEchoBlank=0;  }
fEcho_WasLastEchoBlank_Get()  { { ((_wasLastEchoBlank > 0)) && return 0; } || return 1; }
fEcho_IsInRawInlineMode_Set() { { [[ "${1:-}" == "1" ]] && _isEchoInRawInlineMode=1; } || { _isEchoInRawInlineMode=0; _wasLastEchoBlank=0; echo; }; }  ## Script it telling fEcho* that something is going to be echoing to the screen in non-linefeed mode without its knowledge. (E.g. "echo -n 'something: '".)
fEcho_IsInRawInlineMode_Get() { { ((_isEchoInRawInlineMode)) && return 0; } || return 1; }
fEcho_Clean_byref(){
	## Validate nameref args:
	[[ -v 1  ]] || fThrowError "Calling function must pass a nameref to supply the input value to this function, as arg1 (string to echo)."
	## Gather args
	local -n ptr_ToEcho_t5jf2=$1
	## Logic
	((_isEchoInRawInlineMode)) && fEcho_IsInRawInlineMode_Set 0
	if [[ -n "${ptr_ToEcho_t5jf2}" ]]; then
		echo -e "${ptr_ToEcho_t5jf2}"
		_wasLastEchoBlank=0
	elif [[ $_wasLastEchoBlank -eq 0 ]]; then
		echo
		_wasLastEchoBlank=1
	fi
}
fEcho_Clean()        { local -r toEcho="${1:-}"; fEcho_Clean_byref toEcho; }
fEcho()              { { [[ -z "${1:-}" ]] && fEcho_Clean ""; } || { local -r toEcho="[ ${1:-} ]"; fEcho_Clean_byref toEcho; }; }
fEcho_Force()        { _wasLastEchoBlank=0; fEcho "${1:-}"; }
fEcho_Clean_Force()  { _wasLastEchoBlank=0; local -r toEcho="${1:-}"; fEcho_Clean_byref toEcho; }
declare -r _hr="••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••"
fEcho_Section()      { fEcho_Clean; fEcho_Clean_Force "${_hr}"; fEcho "${1:-}"; }

## Error-handling
declare -i _wasCleanupRun=0  ## Managed internally by this suite.
declare -i _doExitOnThrow=0    ## Managed internally by this suite.
declare -i _ErrVal=0         ## Set by this suite, but managed by calling functions. Think of it as an extended '$?' that doesn't immediately clear.
_fSingleExitPoint(){
	local -r signal="${1:-}"
	local -r lineNum="${2:-}"
	local -r exitCode="${3:-}"
	local -r errMsg="${4:-}"
	local -r errCommand="$BASH_COMMAND"
	_ErrVal=$exitCode
	if [[ "${signal}" == "INT" ]]; then
		fEcho_Force
		echo "User interrupted." >&2
		fEcho_ResetBlankCounter
		fCleanup  ## User cleanup
		exit 1
	elif [[ "${exitCode}" != "0" ]] && [[ "${exitCode}" != "1" ]]; then  ## Clunky string compare is less likely to fail than integer
		fEcho_Clean
		echo -e "Signal .....: '${signal}'"      >&2
		echo -e "Err# .......: '${exitCode}'"    >&2
	#	echo -e "Message ....: '${errMsg}'"      >&2
		echo -e "At line# ...: '${lineNum}'"     >&2
		echo -e "Command# ...: '${errCommand}'"  >&2
		fEcho_Clean_Force
		fCleanup  ## User cleanup
	else
		fCleanup  ## User cleanup
	fi ;}
_fTrap_Exit(){
	if [[ "${_wasCleanupRun}" == "0" ]]; then  ## String compare is less to fail than integer
		_wasCleanupRun=1
		_fSingleExitPoint "${@}"
	fi ;}
_fTrap_Error(){
	if [[ "${_wasCleanupRun}" == "0" ]]; then  ## String compare is less to fail than integer
		_wasCleanupRun=1
		fEcho_ResetBlankCounter
		_fSingleExitPoint "${@}"
	fi ;}
_fTrap_Error_Ignore(){ _ErrVal=1; true;  return 0; }
_fTrap_Error_Soft(){   _ErrVal=1; false; return 1; }
fThrowError(){
	local errMsg="${1:-}"           ; [[ -z "${errMsg}"      ]] && errMsg="An error occurred."
	local meNameLocal="${meName:-}"
	[[ -z "${meNameLocal}" ]] && meNameLocal="$(basename "${BASH_SOURCE[0]}")"
	[[ -n "${meNameLocal}" ]] && errMsg="${meNameLocal}: ${errMsg}"
	local callStack=""
	for (( i = 1; i < ${#FUNCNAME[@]}; i++ )); do
		[[ "${FUNCNAME[i]}" =~ main|source ]] && continue
		[[ -n "${callStack}" ]] && callStack="${callStack}, "; callStack="${callStack}${FUNCNAME[i]}()"
	done;:
	[[ -n "${callStack}" ]] && callStack="Reverse call stack: ${callStack}"
	fEcho_Clean; echo -e "${errMsg}\n${callStack}" >&2; fEcho_ResetBlankCounter
	_ErrVal=1
	{ ((_doExitOnThrow)) && exit 1; } || return 1; }
fDefineTrap_Error_Fatal(){        :; _ErrVal=0; _doExitOnThrow=0; trap '_fTrap_Error         ERR    ${LINENO}  $?  $_' ERR; set -e; } ## Standard; exits script on any caught error; but 'set -e' has known inconsistencies catching or ignoring errors.
fDefineTrap_Error_ExitOnThrow(){  :; _ErrVal=0; _doExitOnThrow=0; trap '_fTrap_Error         ERR    ${LINENO}  $?  $_' ERR; set +e; } ## Only exits script on fThrowError().
fDefineTrap_Error_Soft(){         :; _ErrVal=0; _doExitOnThrow=0; trap '_fTrap_Error_Soft    ERR    ${LINENO}  $?  $_' ERR; set -e; } ## Returns error code of 1 on error.
fDefineTrap_Error_Ignore(){       :; _ErrVal=0; _doExitOnThrow=0; trap '_fTrap_Error_Ignore  ERR    ${LINENO}  $?  $_' ERR; set +e; } ## Eats errors and returns true.
fDefineTrap_Error_Fatal
trap '_fTrap_Error SIGHUP  ${LINENO} $? $_' SIGHUP
trap '_fTrap_Error SIGINT  ${LINENO} $? $_' SIGINT    ## CTRL+C
trap '_fTrap_Error SIGTERM ${LINENO} $? $_' SIGTERM
trap '_fTrap_Exit  EXIT    ${LINENO} $? $_' EXIT
trap '_fTrap_Exit  INT     ${LINENO} $? $_' INT
trap '_fTrap_Exit  TERM    ${LINENO} $? $_' TERM


#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
# Script entry point

## Bash environment settings (comment out what you don't want)
 set -u  #..................: Require variable declaration. Stronger than mere linting. But can struggle if functions are in sourced files.
 set -e  #..................: Exit on errors. This is inconsistent (made a little better with settings below), so eventually may move to 'set +e' (which is more constant work and mental overhead).
 set -E  #..................: Propagate ERR trap settings into functions, command substitutions, and subshells.
 set   -o pipefail  #.......: Make sure all stages of piped commands also fail the same.
 shopt -s inherit_errexit  #: Propagate 'set -e' ........ into functions, command substitutions, and subshells. Will fail on Bash <4.4.
 shopt -s dotglob  #........: Include usually-hidden 'dotfiles' in '*' glob operations - usually desired.
 shopt -s globstar  #.......: ** matches more stuff including recursion.

## Check if sourced
declare -i isSourced; { (return 0 2>/dev/null) && isSourced=1; } || isSourced=0

## Common constants but detect if already set
if [[ -z "${serialDT+x}"     ]]; then
	declare -r mePath="$(realpath -e "${BASH_SOURCE[0]}")"
	declare -r meName="$(basename "${mePath}")"
	declare -r meDir="$(dirname "${mePath}")"
	declare -r serialDT="$(date "+%Y%m%d-%H%M%S")"
fi

## Make sure relative paths work
cd "${meDir}"

## Cap every step to at most half the cores (exports GOMAXPROCS, so the go tools
## spawned below - staticcheck/govulncheck/gosec - inherit it too).
source "${meDir}/utility/include/cpu-limit.bash"

## GFS rotation for the run-log artifacts (gfs_rotate)
source "${meDir}/utility/include/gfs-rotate.bash"

## Pinned linter/scanner versions (NGDB_STATICCHECK/GOVULNCHECK/GOSEC)
source "${meDir}/utility/include/tool-versions.bash"

## Invoke main
fMain  "${@}"


##	Script history:
##		- 20260420 JC: Created.
##		- 20260421 JC: Finished.
##		- 20260428 JC: Added extracting Windows exe from zip, to list of dir candidates.
##		- 20260503 JC: Added explicit $dirPath_Base variable, and 'cd'ing to it.
##		- 20260511 JC: Renamed to *.bash to make it clear it's not a POSIX shell.
##		- 20260519 JC:
##			- Removed some template cruft.
##			- Better cp args.
##			- Updated fEcho functions.
##			- Changed license from GPL2 to MIT.
##		- 20260701 JC: Adapted from the convert-base-v2 template: ngdb name/paths, build.bash instead of make; cross-compile deferred to the CI/CD backlog item.
##		- 20260701 JC: build.bash moved into cicd/ and now outputs straight to bin/; dropped the staging copy.
##		- 20260701 JC: Wired cross-compile (win-amd64, linux-arm64, win-arm64), the windows zip, and govulncheck.
##		- 20260703 JC: Replaced the post-build bare launch (which opened the TUI) with a --version check. Fixed the git-automation script path - it was missing the base prefix and doubled the cicd/ segment.
##		- 20260703 JC: Blank line between major stages for readability.
##		- 20260704 JC: Added -q/--quiet (unattended, flows to publish) and -m/--message (auto-generated under -q); stage headers use fEcho.
##		- 20260704 JC: Renamed the built executable nanogitdb -> ngdb.
##		- 20260704 JC: Added --quick (skip cross-builds and govulncheck).
##		- 20260704 JC: --quick also skips screenshots; screenshots.bash moved to github/utility/; run screenshots as a stage before publish; always leave a trailing blank line.
##		- 20260704 JC: Preflight now captures the commit message up front (Ctrl+C aborts) instead of a y/n gate; section headers get the letterbox rule; passes --no-prompt to the publish step so it doesn't re-ask.
##		- 20260711 JC: Full cross-platform packaging via goreleaser (binaries for linux/darwin/windows/freebsd amd64+arm64, .deb/.rpm, archives, checksums) plus a Windows .exe installer; split native debug (test/profile) from release (dogfood/package) builds; added --no-arm.
##		- 20260804 JC: Publish step skips its second y/n via the doPromptToContinue setting; the --no-prompt switch no longer exists in that script.
