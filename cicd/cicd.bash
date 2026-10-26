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
	declare -r  exeName="gitniblet"
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
	declare  -i doQuick=0  ## --quick: skip the slow stages (cross-builds, packaging, remote tests, profiler, demo)
	declare  -i doNoArm=0  ## --no-arm: build/package amd64 only (arm64 is cheap here, so this is just a run-time trim)
	## Off switches for the stages that touch the box or the tracked tree, so a full run can stay hands-off
	declare  -i doInstall=1  doHooks=1  doPublish=1  doDemo=1  doFormat=1
	## Build through security run in the pinned image from cicd/container/ unless --no-container.
	## No docker, or an image that won't build, falls back to this host with one line.
	declare  -i doContainer=1
	declare  -i doRemote=1  ## Native tests on the test boxes for the platforms only cross-built here
	declare  -i doKeepTemp=0  ## --keep-temp: keep the run folder and the run's containers
	## run, quick, backlog-quick, backlog-full or merge-gate. The last 3 are hands-off:
	## no dogfood, publish, hooks, demo or gofmt -w, and no prompt.
	declare     runMode="run"
	declare  -a testAreas=()
	declare  -i doList=0  doSync=1
	declare     leakOpt=""            ## No leak checks yet, so this is only taken
	declare     aliasNote=""
	## --backlog-run-quick's areas, as "name|go packages", in --help order.
	declare -ra testAreaDefs=(
		"cli|./internal/cli/..."           "tui|./internal/tui/..."             "web|./internal/web/..."
		"core|./internal/core/..."         "config|./internal/core/config/..."  "crud|./internal/core/crud/..."
		"ddl|./internal/core/ddl/..."      "schema|./internal/core/schema/..."  "store|./internal/core/store/..."
		"txlog|./internal/core/txlog/..."  "seams|./app/... ./cmd/... ./donate/... ./enc/... ./gate/... ./script/..."
	)
	## Under -race. Its own step on this host, since -race needs cgo and the container has no C compiler.
	declare -ra racePkgs=("./internal/tui/...")
	declare  -i inContainer=0; [[ "${GITNIBLET_IN_CONTAINER:-}" == "1" ]] && inContainer=1  ## Set for the run started inside
	declare -r  projectName="gitniblet"
	declare -r  containerImage="gitniblet-cicd"
	declare -r  containerVolume="gitniblet-cicd-cache"
	declare -r  containerEngine="${GITNIBLET_DOCKER:-docker}"  ## Anything that takes docker's arguments
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
	fEcho_Clean "  • Fast-forward from the remote, or stop if the branch has diverged."
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
	fEcho_Clean "  (no flags)          The default run, up to about an hour: remote sync, builds,"
	fEcho_Clean "                      cross-builds and packages, lint, tests, fuzz, security,"
	fEcho_Clean "                      the TUI tests under -race, the pipeline's own tests,"
	fEcho_Clean "                      remote tests, dogfood, profiler, demo, hooks, then back up"
	fEcho_Clean "                      and publish."
	fEcho_Clean "      --quick         About 10 minutes. Skips cross-builds, packaging, the"
	fEcho_Clean "                      pipeline's own tests, remote tests, profiler and demo."
	fEcho_Clean "                      Still dogfoods the release build."
	fEcho_Clean "      --long          Also --slow. The default run, since nothing here takes"
	fEcho_Clean "                      hours."
	fEcho_Clean "      --leakcheck, --no-leakcheck"
	fEcho_Clean "                      Taken. There are no leak checks yet."
	fEcho_Clean "      --backlog-run-quick [<area>...]"
	fEcho_Clean "                      About 4 minutes: release build and CLI smoke, gofmt check,"
	fEcho_Clean "                      staticcheck, and the tests, or only the tests of the areas"
	fEcho_Clean "                      named. Also --gate. Areas:"
	local def line="                       "
	for def in "${testAreaDefs[@]}"; do line+=" ${def%%|*}"; done
	fEcho_Clean "${line}"
	fEcho_Clean "      --backlog-run-full"
	fEcho_Clean "                      Every check of the default run, the other boxes included."
	fEcho_Clean "      --merge-gate    The same checks, before a merge to main. Also --ci."
	fEcho_Clean "                      These 3 never dogfood, publish, commit, push, install,"
	fEcho_Clean "                      sync, rewrite a tracked file, or prompt."
	fEcho_Clean "      --list          Print the steps and test IDs the other flags would run,"
	fEcho_Clean "                      and run nothing."
	fEcho_Clean "  -q, --quiet         Run unattended (no prompt) and quiet; flows to publish."
	fEcho_Clean "      --no-sync       Skip the fetch and fast-forward from the remote before the"
	fEcho_Clean "                      build. A diverged branch stops the run without it."
	fEcho_Clean "      --no-arm        Build/package amd64 only (skip arm64) to trim a full run."
	fEcho_Clean "      --no-install    Skip the dogfood install of the built binary."
	fEcho_Clean "      --no-hooks      Skip the pre-publish hooks (../private/cicd-hooks)."
	fEcho_Clean "      --no-publish    Skip backup + publish: no commit, no push."
	fEcho_Clean "      --no-demo       Skip the demo recording, which rewrites assets/demo.gif."
	fEcho_Clean "      --no-format     Check gofmt only, fail on unformatted Go files rather"
	fEcho_Clean "                      than rewriting them."
	fEcho_Clean "      --no-container  Build and test on this host, not in the container."
	fEcho_Clean "      --no-remote     Skip the tests on the other boxes (Windows, FreeBSD, arm64"
	fEcho_Clean "                      Linux, Intel macOS). A box that doesn't answer is a SKIP."
	fEcho_Clean "      --keep-temp     Keep the run folder and the run's containers, and print"
	fEcho_Clean "                      where they are."
	fEcho_Clean "  -m, --message MSG   Commit message for publish (also --msg; -m MSG or -m=MSG)."
	fEcho_Clean "                      With -q and no -m, a message is auto-generated."
	fEcho_Clean "  -h, --help          Show this."
	fEcho_Clean "  -v, --version       Show version."
	fEcho_Clean ""
	fEcho_Clean "Build, lint, tests, fuzz and the security checks run in an x86_64 Debian image"
	fEcho_Clean "with Go pinned, built from cicd/container/Dockerfile the first time it's"
	fEcho_Clean "needed and tagged by that file's hash. The repo is mounted at its own path."
	fEcho_Clean "Dogfood, profiler, demo, hooks and publish stay on this host. No docker, or an"
	fEcho_Clean "image that won't build, runs it all here instead. GITNIBLET_DOCKER names another"
	fEcho_Clean "program that takes docker's arguments."
	fEcho_Clean ""
	fEcho_Clean "Each run works in \${HOST_TEMP}/gitniblet-floss/<stamp>_<NNN>/, with TMPDIR"
	fEcho_Clean "pointed at it, and removes it at the end, on failure and Ctrl+C too. It also"
	fEcho_Clean "removes run folders older than 2 days whose run.pid is gone. At the end the"
	fEcho_Clean "run's containers go, then this project's dangling images, its image tags older"
	fEcho_Clean "than the one before the current, and build cache older than a week."
	fEcho_Clean ""
	fEcho_Clean "Every step runs at nice 19 and idle I/O, on half the CPUs, or a quarter for"
	fEcho_Clean "fuzz and -race or when the 1-minute load is over half of them, checked before"
	fEcho_Clean "each step."
	fEcho_Clean ""
	fEcho_Clean "The run log goes to cicd/artifacts/lint/, or to GITNIBLET_LOG_DIR when it's set,"
	fEcho_Clean "as the pipeline's own tests do for the runs they start. GITNIBLET_HOOKS_DIR stands in"
	fEcho_Clean "for ../private/cicd-hooks the same way."
	fEcho_Clean ""
	fEcho_Clean "cicd/utility/git-hooks/pre-push runs --merge-gate before a push to main, unless"
	fEcho_Clean "the push changes only the paths in no-gate-paths beside it. A push to dev isn't"
	fEcho_Clean "gated. Turn it on once per clone with:"
	fEcho_Clean "  git config core.hooksPath cicd/utility/git-hooks"
	#           X-------------------------------------------------------------------------------X
	fEcho_Clean "" ;:;}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
fMain(){

	## Arguments: -q/--quiet (unattended + quiet, flows to publish), -m/--message
	## (hands-off commit message; also --msg, accepts -m MSG or -m=MSG).
	local cliMessage=""
	while (($#)); do case "${1}" in
		-q|--quiet)                doQuietly=1                          ; shift ;;
		--quick)                   runMode="quick"                      ; shift ;;
		--long|--slow)             runMode="run"                        ; shift ;;
		--leakcheck|--no-leakcheck)
			[[ -z "${leakOpt}" || "${leakOpt}" == "${1}" ]]  ||  fThrowError "--leakcheck and --no-leakcheck are opposites; give one."
			leakOpt="${1}"; shift ;;
		--gate|--backlog-run-quick)
			[[ "${1}" != "--gate" ]]  ||  aliasNote="--gate is another name for --backlog-run-quick."
			runMode="backlog-quick"; shift
			while (($#)) && [[ "${1}" != -* ]]; do testAreas+=("${1}"); shift; done ;;
		--backlog-run-full)        runMode="backlog-full"               ; shift ;;
		--ci|--merge-gate)
			[[ "${1}" != "--ci" ]]  ||  aliasNote="--ci is another name for --merge-gate."
			runMode="merge-gate"; shift ;;
		--list)                    doList=1                             ; shift ;;
		--no-sync)                 doSync=0                             ; shift ;;
		--no-arm)                  doNoArm=1                            ; shift ;;
		--no-install)              doInstall=0                          ; shift ;;
		--no-hooks)                doHooks=0                            ; shift ;;
		--no-publish)              doPublish=0                          ; shift ;;
		--no-demo)                 doDemo=0                             ; shift ;;
		--no-format)               doFormat=0                           ; shift ;;
		--no-container)            doContainer=0                        ; shift ;;
		--container)               doContainer=1                        ; shift ;;
		--no-remote)               doRemote=0                           ; shift ;;
		--keep-temp)               doKeepTemp=1                         ; shift ;;
		--message=*|--msg=*|-m=*)  cliMessage="${1#*=}"                 ; shift ;;
		-m|--message|--msg)        cliMessage="${2-}"; shift; (($#)) && shift ;;
		-h|--help)                 fCopyright; fAbout; fSyntax          ; return 0 ;;
		-v|--version)              fVersion                             ; return 0 ;;
		*) fThrowError "Unknown option: '${1}' (try --help)." ;;
	esac; done

	local area def known
	for area in "${testAreas[@]}"; do
		known=""
		for def in "${testAreaDefs[@]}"; do [[ "${def%%|*}" != "${area}" ]]  ||  known=1; done
		[[ -n "${known}" ]]  ||  fThrowError "Unknown area: '${area}'. --help lists them."
	done

	[[ "${runMode}" != "quick" ]]  ||  doQuick=1
	## The hands-off modes leave the box, the remote and the tracked tree alone.
	if fIsHandsOff; then
		doQuietly=1  doInstall=0  doHooks=0  doPublish=0  doDemo=0  doFormat=0  doSync=0
	fi

	## Commit message: -m wins; else auto-generate when running quiet/unattended.
	## An empty message when interactive lets the publish step open an editor.
	local commitMessage="${cliMessage}"
	if [[ -z "${commitMessage}" ]] && ((doQuietly)); then commitMessage="Updated"; fi

	if ((inContainer)); then doContainer=0; fi

	# Validate dependencies
	fMustBeInPath realpath
	## Only the publish script uses it, and a hosted runner has none.
	((inContainer || ! doPublish || doList))  ||  fMustBeInPath trash

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

	declare hooksDir="${GITNIBLET_HOOKS_DIR:-${dirPath_Base}/../private/cicd-hooks}"
	if ((doList)); then fList; return 0; fi

	## Prompt to continue
	if ((! doQuietly)); then
		fCopyright
		fAbout
		fEcho_Clean
		fEcho_Clean "Mode .........................: ${runMode}"
		fEcho_Clean "Source directory .............: ${dirPath_Source}"
		if ((isCompileProject)); then
		fEcho_Clean "Executable to build ..........: ${filePath_ExecToTestAndInstall_BuildLocation}"
		fEcho_Clean "Executable final location ....: ${filePath_ExecToTestAndInstall_FinalHome}"
		fi
		fEcho_Clean "Test script ..................: ${filePath_CICD_TestExec}"
		if ((doQuick)); then
		fEcho_Clean "Quick mode ...................: cross-builds + packaging + remote tests + profiler + demo skipped"
		fi
		if ((doNoArm)); then
		fEcho_Clean "No-arm mode ..................: arm64 builds + packages skipped (amd64 only)"
		fi
		((doInstall))  ||  fEcho_Clean "Dogfood install ..............: skipped (--no-install)"
		((doHooks))    ||  fEcho_Clean "Local hooks ..................: skipped (--no-hooks)"
		((doDemo))     ||  fEcho_Clean "Demo recording ...............: skipped (--no-demo)"
		((doFormat))   ||  fEcho_Clean "gofmt ........................: check only (--no-format)"
		((doContainer))  ||  fEcho_Clean "Container ....................: off, all on this host (--no-container)"
		((doRemote))     ||  fEcho_Clean "Remote tests .................: skipped (--no-remote)"
		((! doKeepTemp)) ||  fEcho_Clean "Temp files ...................: kept (--keep-temp)"
		if ((! doPublish)); then
		fEcho_Clean "Backup + publish .............: skipped (--no-publish)"
		else
		fEcho_Clean "Git commit and push script ...: ${gitAutomationScript}"
		if [[ -n "${commitMessage}" ]]; then
		fEcho_Clean "Publish commit message .......: \"${commitMessage}\""
		else
		fEcho_Clean "Publish commit message .......: (prompt below; blank = editor)"
		fi
		fi
		fEcho_Clean

		## Capture the commit message up front so the rest of the run finishes
		## unattended. This doubles as the go/no-go gate: Ctrl+C here aborts (there
		## is no separate y/n prompt). A blank reply leaves the editor to open at
		## publish; -m/-q skip this entirely.
		if [[ -z "${commitMessage}" ]] && ((doPublish)); then
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

	## The run's own temp folder. The run in the container gets the outer one's,
	## mounted at the same path, so it makes none of its own.
	if ((! inContainer)); then
		fTempRun_Sweep
		runDir="$(fTempRun_Make)"  ||  fThrowError "Couldn't make a run folder under '${HOST_TEMP}/${tempProject}'."
		export GITNIBLET_RUN_DIR="${runDir}"
	else
		export GITNIBLET_RUN_DIR="${GITNIBLET_RUN_DIR:-${TMPDIR:-/tmp}}"
	fi
	runStamp="${GITNIBLET_RUN_DIR##*/}"
	export TMPDIR="${GITNIBLET_RUN_DIR}" TEMP="${GITNIBLET_RUN_DIR}" TMP="${GITNIBLET_RUN_DIR}"

	## Capture the whole run to a rotated log so warnings from any stage (build,
	## vet, staticcheck, govulncheck) can be reviewed after the fact - lint-report.bash
	## reads the newest. Rotate the dir first, then tee. Best-effort: if the log can't
	## be created the run just goes uncaptured.
	## Inside the container the outer run already logs all of this.
	declare dirPath_ArtifactsLint="${GITNIBLET_LOG_DIR:-${dirPath_Base}/cicd/artifacts/lint}"
	if ((! inContainer))  &&  mkdir -p "${dirPath_ArtifactsLint}" 2>/dev/null; then
		gfs_rotate "${dirPath_ArtifactsLint}" run log >/dev/null 2>&1 || true
		## tee ignores Ctrl+C, or the cleanup at exit writes into a dead pipe and dies.
		exec > >(trap '' INT TERM; exec tee "${dirPath_ArtifactsLint}/run_${serialDT}.log") 2>&1
	fi

	## Every step is counted before the first one runs. The run in the container
	## takes the outer run's list, so its numbers carry on.
	if ((inContainer)) && [[ -n "${GITNIBLET_STEPS:-}" ]]; then fSteps_Import
	else fSteps_PlanAll; fi

	local -i useContainer=0
	if ((! inContainer)); then
		local containerLine="Container: off (--no-container), so build and test run on this host."
		if ((doContainer)); then
			if fContainer_Check; then
				useContainer=1
				fInnerArgs
				containerLine="Container: ${containerTag} for build through security, at --cpus ${GITNIBLET_JOBS_MAX}, as cicd.bash ${innerArgs[*]}"
			else
				containerLine="Container: ${containerWhy}, so build and test run on this host."
			fi
		fi
		local -a headerLines=()
		[[ -z "${aliasNote}" ]]  ||  headerLines+=("${aliasNote}")
		((${#testAreas[@]} == 0))  ||  headerLines+=("Test areas: ${testAreas[*]}")
		[[ -z "${leakOpt}" ]]  ||  headerLines+=("${leakOpt}: there are no leak checks yet.")
		fSteps_Header "${exeName} CI/CD, ${runMode}" "${headerLines[@]}" "${containerLine}" "CPUs: ${GITNIBLET_CORES}, load ${GITNIBLET_LOAD}. A step gets half, or a quarter for fuzz or when the load is over half."
	fi

	## First, so the rest tests what publish will push. Publish keeps its own pull.
	## The paths are what this run has already read, so a change to one stops it.
	if fStep "Remote sync"; then
		fRemoteSync cicd/cicd.bash cicd/utility/include cicd/container  ||  fThrowError "Remote sync: ${syncWhy}"
		fStep_End
	fi

	if ((useContainer))  &&  fContainer_Ready; then
		fStage_Container
	else
		fStages_BuildAndTest
	fi
	## The rest needs this machine, so the run inside stops here.
	((! inContainer))  ||  return 0

	if fStep "Race tests"; then
		fEcho_Clean "go test -race ${racePkgs[*]}"
		( cd "${dirPath_Source}"  &&  CGO_ENABLED=1 go test -race -json -mod=vendor -p "${GITNIBLET_JOBS}" -parallel "${GITNIBLET_JOBS}" "${racePkgs[@]}" ) \
			| GOFLAGS='' go -C "${dirPath_Base}/cicd/utility/testid" run . report "${dirPath_Base}"
		fStep_End
	fi

	## Some of these start cicd.bash itself, so one at a time, after the build.
	if fStep "Pipeline tests"; then
		local testScript
		local -a testScripts=() failedScripts=()
		fPipelineTests testScripts
		for testScript in "${testScripts[@]}"; do
			fEcho_Clean "${testScript##*/}"
			nice -n 19 bash "${testScript}"  ||  failedScripts+=("${testScript##*/}")
		done
		((${#failedScripts[@]} == 0))  ||  fThrowError "Pipeline tests failed: ${failedScripts[*]}"
		fStep_End
	fi

	## Tests on the other boxes, native and outside the container, since they need
	## this machine's host lock and ssh. A box that's away is a SKIP, not a failure.
	local remoteSummary=""
	if fStep "Remote tests"; then
		remoteSummary="${dirPath_ArtifactsLint}/remote_${serialDT}.txt"
		fEcho_Clean "Windows, FreeBSD, arm64 Linux and Intel macOS"
		"${dirPath_Base}/cicd/utility/remote-test.bash" $( ((! doKeepTemp)) || echo "--keep-temp") --summary "${remoteSummary}"  ||  fThrowError "Remote tests failed; the output is above."
		fStep_End
	fi

	if fStep "Dogfood"; then

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
		fStep_End
	fi

	## Profiler: sample the CPU-hot replay path, render a flamegraph into
	## cicd/artifacts/profiling, GFS-rotate the old ones, and print the hotspot
	## summary (flame-report.py, plain mode) into the log. Non-gating artifact -
	## any tooling miss warns and skips. Slow, so --quick skips it. cicd never
	## touches the .flame-seen marker; that startup gate is flame-report.py --check.
	if fStep "Profiler"; then
		fEcho_Clean "Flamegraph and hotspots"
		"${dirPath_Base}/cicd/utility/profile.bash"
		fStep_End
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
	if fStep "Demo recording"; then
		python3 "${dirPath_Base}/cicd/utility/demo-video/demo-video.py" --no-build
		fEcho_Clean "Demo mp4 + gif regenerated"
		fStep_End
	fi

	## Optional local pre-publish hooks (not committed, kept in ../private/cicd-hooks). Each
	## *.bash there runs here before publish so anything a hook regenerates gets
	## committed in the same run. No-op when the folder is absent. A hook gets the
	## repo base path as $1; a failing hook warns but does not abort the run.
	if fStep "Local hooks"; then
		for nextHook in "${hooksDir}"/*.bash; do
			[[ -f "${nextHook}" ]]  ||  continue
			fEcho_Clean "running $(basename "${nextHook}")"
			bash "${nextHook}" "${dirPath_Base}"  ||  fEcho_Clean "hook failed (continuing): $(basename "${nextHook}")"
		done
		fStep_End
	fi

	## Git automation script (backup, commit, push). Flow the quiet flag and the
	## resolved commit message through so an unattended run stays hands-off.
	if fStep "Backup + publish"; then
		## doPromptToContinue=0: cicd already gated up front (message captured above),
		## so the publish step keeps its remote/branch/SSH review but skips a second
		## y/n. Set as a settings variable, not a switch - that script is kept
		## identical across projects and takes no flag for this.
		local -a gitArgs=()
		if ((doQuietly));                then gitArgs+=(--quiet); fi
		if [[ -n "${commitMessage}" ]];  then gitArgs+=("--message=${commitMessage}"); fi
		## This run already passed what the pre-push hook's --merge-gate would check.
		doPromptToContinue=0  SKIP_GATE=1  "${gitAutomationScript}"  "${gitArgs[@]}"
		fStep_End
	fi

	## Done
	local -a remoteLines=()
	[[ -z "${remoteSummary}" || ! -s "${remoteSummary}" ]]  ||  mapfile -t remoteLines < "${remoteSummary}"
	fSteps_Footer "${exeName} CI/CD: done; PASS, ${#stepsOn[@]} steps" "${remoteLines[@]}"
}


## Every step in run order, on or off, with why it's off. The conditions here
## are the only ones; each step below runs when fStep finds it in this list.
fSteps_PlanAll(){
	local why quick=""  bq=""  off
	((! doQuick))  ||  quick="--quick"
	[[ "${runMode}" != "backlog-quick" ]]  ||  bq="--backlog-run-quick"
	if ((! inContainer)); then
		why="";  ((doSync))  ||  { fOff --no-sync; why="${off}"; }
		fStep_Plan "Remote sync" "${why}"
	fi
	if ((isCompileProject)); then
		fStep_Plan "Build (native debug)" "${bq}"
		fStep_Plan "Build (native release)"
		fStep_Plan "Cross-compile + package" "${bq:-${quick}}"
	fi
	fStep_Plan "Lint + format"
	fStep_Plan "Tests"
	fStep_Plan "Fuzz" "${bq}"
	fStep_Plan "Security + supply-chain" "${bq}"
	## The rest runs on this machine, never in the container.
	((! inContainer))  ||  return 0

	fRaceWhy;  why="${bq:-${raceWhy}}"
	fStep_Plan "Race tests" "${why}"
	local -a testScripts=()
	fPipelineTests testScripts
	why="";  ((${#testScripts[@]}))  ||  why="no cicd/utility/*-test.bash";  why="${bq:-${quick:-${why}}}"
	fStep_Plan "Pipeline tests" "${why}"

	why="${bq:-${quick}}";  ((doRemote))  ||  why="--no-remote"
	fStep_Plan "Remote tests" "${why}"
	why="";  ((doInstall))  ||  { fOff --no-install; why="${off}"; }
	fStep_Plan "Dogfood" "${why}"
	why="";  [[ -x "${dirPath_Base}/cicd/utility/profile.bash" ]]  ||  why="no cicd/utility/profile.bash";  why="${bq:-${quick:-${why}}}"
	fStep_Plan "Profiler" "${why}"
	why="";  [[ -x "${dirPath_Base}/cicd/utility/demo-video/demo-video.py" ]]  ||  why="no demo-video.py";  ((doDemo))  ||  { fOff --no-demo; why="${off}"; };  why="${quick:-${why}}"
	fStep_Plan "Demo recording" "${why}"
	why="";  [[ -d "${hooksDir}" ]]  ||  why="no ${hooksDir}";  ((doHooks))  ||  { fOff --no-hooks; why="${off}"; }
	fStep_Plan "Local hooks" "${why}"
	why="";  ((doPublish))  ||  { fOff --no-publish; why="${off}"; }
	fStep_Plan "Backup + publish" "${why}"
}

fIsHandsOff(){ [[ "${runMode}" == backlog-* || "${runMode}" == "merge-gate" ]]; }
## The pipeline's own tests: every *-test.bash beside cicd.bash, so a new one
## can't be left out, but remote-test.bash, which is the Remote tests step.
fPipelineTests(){
	local -n scripts_p7x="${1}"
	local f
	scripts_p7x=()
	for f in "${dirPath_Base}"/cicd/utility/*-test.bash; do
		[[ ! -f "${f}" || "${f##*/}" == "remote-test.bash" ]]  ||  scripts_p7x+=("${f}")
	done
	return 0
}
## Sets raceWhy when this host can't run -race, which needs go and a C compiler.
fRaceWhy(){
	local cc=""
	raceWhy=""
	if ! command -v go &> /dev/null; then raceWhy="no go on this host"; return 0; fi
	read -r cc _ <<< "$(go env CC 2> /dev/null)"
	command -v "${cc:-gcc}" &> /dev/null  ||  raceWhy="no ${cc:-gcc} for -race"
}
## Sets the caller's off: the flag a person would give, or the hands-off mode that turned it off.
fOff(){ if fIsHandsOff; then off="--${runMode/#backlog-/backlog-run-}"; else off="${1}"; fi; }

## --list: the plan, then the test IDs of each step that runs tests. Runs nothing.
fList(){
	fSteps_PlanAll
	local s ids pkg name id def area
	local -i n=0
	[[ -z "${aliasNote}" ]]  ||  fEcho_Clean "${aliasNote}"
	fEcho_Clean "${exeName} CI/CD, ${runMode}: ${#stepsOn[@]} steps, ${#stepsOff[@]} off"
	for s in "${stepsOn[@]}"; do fEcho_Clean "Step $((++n)): ${s}"; done
	for s in "${stepsOff[@]}"; do fEcho_Clean "Off: ${s}"; done
	command -v go &> /dev/null  ||  { fEcho_Clean "No go here, so no test IDs."; return 0; }
	ids="$(GOFLAGS='' go -C "${dirPath_Base}/cicd/utility/testid" run . ids "${dirPath_Base}")"  ||  fThrowError "Couldn't read the test IDs."

	## The areas as import path prefixes. gitniblet-testid is its own module, run only with no areas.
	local -r modPath="$(sed -n 's/^module //p' "${dirPath_Source}/go.mod")"
	local -a prefixes=()
	for area in "${testAreas[@]}"; do
		for def in "${testAreaDefs[@]}"; do
			[[ "${def%%|*}" == "${area}" ]]  ||  continue
			for pkg in ${def#*|}; do pkg="${pkg#./}"; prefixes+=("${modPath}/${pkg%/...}"); done
		done
	done
	if fStep_IsOn "Tests"; then
		fEcho_Clean "Tests:"
		while read -r pkg name id; do
			if ((${#prefixes[@]})); then
				local hit=""
				for s in "${prefixes[@]}"; do [[ "${pkg}" != "${s}" && "${pkg}" != "${s}/"* ]]  ||  hit=1; done
				[[ -n "${hit}" ]]  ||  continue
			fi
			fEcho_Clean "${id} ${pkg#"${modPath}/"} ${name}"
		done <<< "${ids}"
	fi
	if fStep_IsOn "Fuzz"; then
		fEcho_Clean "Fuzz:"
		while read -r pkg name id; do
			[[ "${name}" == Fuzz* ]]  &&  fEcho_Clean "${id} ${pkg#"${modPath}/"} ${name}"
		done <<< "${ids}"
	fi
	if fStep_IsOn "Race tests"; then
		fEcho_Clean "Race tests:"
		prefixes=()
		for pkg in "${racePkgs[@]}"; do pkg="${pkg#./}"; prefixes+=("${modPath}/${pkg%/...}"); done
		while read -r pkg name id; do
			for s in "${prefixes[@]}"; do
				[[ "${pkg}" != "${s}" && "${pkg}" != "${s}/"* ]]  ||  { fEcho_Clean "${id} ${pkg#"${modPath}/"} ${name}"; break; }
			done
		done <<< "${ids}"
	fi
	if fStep_IsOn "Pipeline tests"; then
		fEcho_Clean "Pipeline tests:"
		local testScript
		local -a testScripts=()
		fPipelineTests testScripts
		for testScript in "${testScripts[@]}"; do
			while read -r _ id; do fEcho_Clean "${id} ${testScript##*/}"; done \
				< <(grep -oE '(fCheck|fCase|PASS) [0-9a-hjkmnp-tv-z]{5} ' "${testScript}" | sort -u -k2,2)
		done
	fi
	! fStep_IsOn "Build (native release)"  ||  fEcho_Clean "Build (native release) runs the CLI smoke, cicd/utility/smoke-cli.bash, which has no per-test IDs."
	! fStep_IsOn "Remote tests"            ||  fEcho_Clean "Remote tests: the module's tests above and the CLI smoke, on each box."
	return 0
}

fStep_IsOn(){ local s; for s in "${stepsOn[@]}"; do [[ "${s}" != "${1}" ]]  ||  return 0; done; return 1; }

## Opens a step that's planned, after checking the load again. Fuzz and -race
## get a quarter of the CPUs, busy or not.
fStep(){
	fStep_Start "${1}"  ||  return 1
	local busy=""
	if [[ "${1}" == "Fuzz" || "${1}" == "Race tests" ]]; then fCpuLimit heavy; else fCpuLimit; fi
	((! GITNIBLET_BUSY))  ||  busy=", busy"
	fEcho_Clean "Jobs: ${GITNIBLET_JOBS} of ${GITNIBLET_CORES} CPUs, load ${GITNIBLET_LOAD}${busy}"
}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Build, lint, tests, fuzz and security. Runs in the container or, without
## one, here.
fStages_BuildAndTest(){

	pushd "${dirPath_Source}" 1>/dev/null

	if ((isCompileProject)); then

		## Native debug build (symbols kept, unstripped) - drives the smoke check and
		## is the artifact for manual profiling/debugging. The test, fuzz, and profiler
		## stages below compile their own debug/bench binaries via `go test`.
		if fStep "Build (native debug)"; then
			"${dirPath_Base}/cicd/build.bash" --debug
			## Minimal execution test: --version prints two lines and exits, never opening the
			## TUI. Running the app for real stays a separate manual step before the merge.
			fEcho_Clean "version check (debug):"
			"${dirPath_Base}/bin/gitniblet-debug" --version
			fStep_End
		fi

		## Native release build (size-optimized, stripped) - what gets dogfooded and
		## screenshotted; goreleaser rebuilds the same optimized flags per-target below.
		fStep "Build (native release)"  ||  fThrowError "The release build isn't planned."
		fEcho_Clean "Size-optimized"
		"${dirPath_Base}/cicd/build.bash"
		fEcho_Clean "version check (release):"
		"${filePath_ExecToTestAndInstall_BuildLocation}" --version
		fEcho_Clean "Built ${filePath_ExecToTestAndInstall_BuildLocation}"
		GITNIBLET_SMOKE_EXPECT_DATE=1 "${dirPath_Base}/cicd/utility/smoke-cli.bash" "${filePath_ExecToTestAndInstall_BuildLocation}"

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
		fStep_End

		## Cross-compile + package via goreleaser (the same .goreleaser.yaml the release
		## uses): binaries for linux/darwin/windows/freebsd (amd64+arm64), .deb/.rpm for
		## linux, a tar.gz/zip per target, and checksums - all into dist/. --snapshot =
		## build + package locally, no publish. Then the Windows single-file .exe
		## installer(s) via NSIS. Skipped under --quick; --no-arm filters the goarch line
		## to amd64 only. Deferred (need the target OS's own host): macOS .dmg, Windows
		## .msi, FreeBSD .pkg - those ship as archives for now.
		if fStep "Cross-compile + package"; then
			fEcho_Clean "goreleaser: binaries, .deb/.rpm, archives, checksums"
			local grConfig=".goreleaser.yaml"
			if ((doNoArm)); then
				grConfig="${dirPath_Base}/cicd/artifacts/goreleaser-noarm.yaml"
				mkdir -p "$(dirname "${grConfig}")"
				sed 's/\[amd64, arm64\]/[amd64]/' "${dirPath_Base}/.goreleaser.yaml" > "${grConfig}"
				fEcho_Clean "arm64 skipped (--no-arm): amd64-only matrix"
			fi
			( cd "${dirPath_Base}"  &&  GOFLAGS=  go run "${GITNIBLET_GORELEASER}" release --snapshot --clean --parallelism "${GITNIBLET_JOBS}" --config "${grConfig}" )
			fEcho_Clean "Packaged into ${dirPath_Base}/dist/ (binaries, .deb/.rpm, archives, checksums)"
			## Windows single-file .exe installer(s) from the just-built binaries (NSIS).
			( cd "${dirPath_Base}"  &&  "${dirPath_Base}/cicd/utility/windows-installer.bash" "${srcVersion:-0.0.0}" )
			fStep_End
		fi

	fi

	## Lint + format. gofmt rewrites Go source in place (the language standard;
	## Bash is never auto-formatted). staticcheck is the deeper linter and resolves
	## its own module, so it runs outside vendor mode. go vet lives in test.bash.
	## gofmt covers the whole module but vendor/, as hosted CI always did. Walking
	## vendor/ only to drop it costs 20 CPU seconds.
	fStep "Lint + format"  ||  fThrowError "Lint isn't planned."
	local -a goRoots=()
	mapfile -t goRoots < <(cd "${dirPath_Source}"  &&  find . -mindepth 1 -maxdepth 1 \( -type d ! -name vendor ! -name '.*' \) -o \( -type f -name '*.go' \))
	local unformatted  ## split so a gofmt parse error isn't masked by 'local'
	unformatted="$(cd "${dirPath_Source}"  &&  gofmt -l "${goRoots[@]}")"
	if [[ -z "${unformatted}" ]]; then
		fEcho_Clean "gofmt: clean"
	elif ((doFormat)); then
		( cd "${dirPath_Source}"  &&  mapfile -t files <<< "${unformatted}"  &&  gofmt -w "${files[@]}" )
		fEcho_Clean "gofmt: formatted in place:"$'\n'"${unformatted}"
	else
		fThrowError "gofmt needed (--no-format and the hands-off modes leave them as is):\n${unformatted}"
	fi
	( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${GITNIBLET_STATICCHECK}" ./... )
	fEcho_Clean "staticcheck: clean"
	fStep_End

	## Test
	fStep "Tests"  ||  fThrowError "Tests aren't planned."
	local -a testPkgs=() pkgs=()
	local area def
	for area in "${testAreas[@]}"; do
		for def in "${testAreaDefs[@]}"; do
			[[ "${def%%|*}" == "${area}" ]]  ||  continue
			read -r -a pkgs <<< "${def#*|}"; testPkgs+=("${pkgs[@]}")
		done
	done
	((${#testPkgs[@]} == 0))  ||  fEcho_Clean "Only the areas ${testAreas[*]}: ${testPkgs[*]}"
	"${filePath_CICD_TestExec}" "${testPkgs[@]}"
	fEcho_Clean "Tests passed"
	fStep_End

	## Fuzz the parsers/decoders with mutated input. The seed corpus already runs
	## inside test.bash (every FuzzXxx executes its seeds as a normal test); this is
	## the mutation budget, which is the slow part, so --quick skips it. A crash
	## fails the run and the minimized input lands under the package's testdata/.
	if fStep "Fuzz"; then
		"${dirPath_Base}/cicd/utility/fuzz.bash" --time 10s
		fStep_End
	fi

	## Security + supply-chain. go mod verify is offline; govulncheck and gosec
	## each pull a tool over the network. All 3 take about a minute, so --quick
	## keeps them, and only --backlog-run-quick leaves them out. Real findings fail.
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
	if fStep "Security + supply-chain"; then
		fEcho_Clean "go mod verify, govulncheck and gosec"
		( cd "${dirPath_Source}"  &&  go mod verify )
		( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${GITNIBLET_GOVULNCHECK}" ./... )
		fEcho_Clean "No known vulnerabilities (govulncheck)"
		( cd "${dirPath_Source}"  &&  GOFLAGS=  go run "${GITNIBLET_GOSEC}" -concurrency "${GITNIBLET_JOBS}" \
			-exclude=G101,G104,G124,G202,G203,G204,G301,G302,G304,G306,G703 -quiet ./... )
		fEcho_Clean "No security findings (gosec)"
		fStep_End
	fi

	popd 1>/dev/null

}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
## Sets containerTag, or containerWhy when the run should stay on this host.
## Prints nothing, so the first section can say which it is.
declare containerTag="" containerWhy=""
declare -a innerArgs=()
declare runDir="" runStamp=""
declare -i dockerUsed=0  imagePassed=0  ## For the cleanup at exit
fContainer_Check(){
	if [[ "$(uname -m)" != "x86_64" ]]; then containerWhy="the image is x86_64 only"
	elif ! command -v "${containerEngine}" &>/dev/null; then containerWhy="no ${containerEngine}"
	elif ! "${containerEngine}" version --format '{{.Server.Version}}' &>/dev/null; then containerWhy="${containerEngine} isn't answering"
	fi
	[[ -z "${containerWhy}" ]]  ||  return 1
	dockerUsed=1

	local -r recipe="${dirPath_Base}/cicd/container/Dockerfile"
	## Called as an if condition, where set -e is off, so a throw has to exit.
	[[ -f "${recipe}" ]]  ||  { fThrowError "Missing the container recipe: '${recipe}'."; exit 1; }
	## A stale pin would build with an older Go than go.mod asks for, and pass.
	local goPin modPin
	goPin="$(sed -n 's/^ARG GO_VER=//p' "${recipe}")"
	modPin="$(sed -n 's/^toolchain go//p' "${dirPath_Source}/go.mod")"
	[[ -n "${goPin}" && "${goPin}" == "${modPin}" ]]  ||  { fThrowError "GO_VER in cicd/container/Dockerfile is '${goPin}', but source/go.mod pins '${modPin}'. Bump GO_VER and GO_SHA256 to match."; exit 1; }

	## Tagged by the recipe's hash, so an edited recipe can't run on the old image.
	containerTag="${containerImage}:$(git -C "${dirPath_Base}" hash-object "${recipe}" | cut -c1-12)"
	return 0
}

## Builds the image if it isn't there yet, and makes the cache volume. Returns
## 1 with one line when the run should stay on this host after all.
fContainer_Ready(){
	if ! "${containerEngine}" image inspect "${containerTag}" &>/dev/null; then
		fEcho_Clean "Container image: building ${containerTag}"
		## nice reaches only the client; the build itself runs in the daemon.
		if ! nice -n 19 "${containerEngine}" build --label "dev.project=${projectName}" --label "dev.run=${runStamp}" -t "${containerTag}" "${dirPath_Base}/cicd/container"; then
			fEcho_Clean "Container: ${containerTag} didn't build, so build and test run on this host."; return 1
		fi
	fi
	if ! "${containerEngine}" volume inspect "${containerVolume}" &>/dev/null \
		&&  ! "${containerEngine}" volume create --label "dev.project=${projectName}" --label "dev.run=${runStamp}" "${containerVolume}" >/dev/null; then
		fEcho_Clean "Container: no cache volume ${containerVolume}, so build and test run on this host."; return 1
	fi
	return 0
}

fInnerArgs(){
	innerArgs=(-q --no-container --no-install --no-hooks --no-publish --no-demo)
	((! doQuick))   ||  innerArgs+=(--quick)
	((! doNoArm))   ||  innerArgs+=(--no-arm)
	((doFormat))    ||  innerArgs+=(--no-format)
	## The mode last, since --backlog-run-quick takes the words after it as areas.
	! fIsHandsOff   ||  innerArgs+=("--${runMode/#backlog-/backlog-run-}" "${testAreas[@]}")
	return 0
}


## The repo at its own path, as the caller's uid, so paths in logs and a
## worktree's .git file still point at something. Caches live in the volume.
## The run inside numbers its steps from this run's list, checks the load
## before each one, and never takes more than the container's --cpus.
fStage_Container(){
	fCpuLimit
	fSteps_Export
	local -a runArgs=(--init
		--name "${projectName}-${runStamp}-pipeline" --label "dev.project=${projectName}" --label "dev.run=${runStamp}"
		--cpus "${GITNIBLET_JOBS_MAX}" --cpu-shares 256 --user "$(id -u):$(id -g)"
		-v "${dirPath_Base}:${dirPath_Base}" -v "${containerVolume}:/cache" -w "${dirPath_Base}"
		-v "${runDir}:${runDir}" -e GITNIBLET_RUN_DIR -e HOST_TEMP -e TMPDIR -e TEMP -e TMP -e GITNIBLET_IN_CONTAINER=1
		-e GITNIBLET_STEPS -e "GITNIBLET_JOBS_CAP=${GITNIBLET_JOBS_MAX}")
	((doKeepTemp))  ||  runArgs+=(--rm)
	## The host's time zone, for the step lines' local time.
	[[ ! -f /etc/localtime ]]  ||  runArgs+=(-v /etc/localtime:/etc/localtime:ro)
	[[ -z "${TZ:-}" ]]  ||  runArgs+=(-e TZ)
	local gitCommon
	gitCommon="$(git -C "${dirPath_Base}" rev-parse --path-format=absolute --git-common-dir)"
	[[ "${gitCommon}" == "${dirPath_Base}/"* ]]  ||  runArgs+=(-v "${gitCommon}:${gitCommon}")

	## In the background, since Ctrl+C would only reach the container's init, and
	## the run inside would finish its current step first. Ctrl+C kills the short
	## sleep below at once, and the cleanup at exit removes the container. Not a
	## bare wait, which once sat out the whole container after a Ctrl+C (2026102520025738).
	fEcho_Clean "Container: running ${projectName}-${runStamp}-pipeline"
	"${containerEngine}" run "${runArgs[@]}" "${containerTag}" nice -n 19 bash "${dirPath_Base}/cicd/cicd.bash" "${innerArgs[@]}" &
	local ctrPid=$!
	while kill -0 "${ctrPid}" 2> /dev/null; do sleep 0.5; done
	wait "${ctrPid}"  ||  fThrowError "The run in the container failed; its output is above."
	fEcho_WasLastEchoBlank_Set 1  ## the run inside closed its last step with a blank line
	imagePassed=1
}


## Only what carries this project's label, and never rmi -f, so an image a
## container still uses stays. The run's containers go first, since they hold
## the run folder's mount.
fContainer_Cleanup(){
	((dockerUsed))  ||  return 0
	local -a ids=() removed=() oldTags=()
	local before after tag
	before="$(fDocker_Bytes)"

	mapfile -t ids < <("${containerEngine}" ps -aq --filter "label=dev.project=${projectName}" --filter "label=dev.run=${runStamp}" 2> /dev/null)
	if ((${#ids[@]})); then
		if ((doKeepTemp)); then
			"${containerEngine}" stop "${ids[@]}" &> /dev/null  ||  true
			fEcho_Clean "Kept container ${projectName}-${runStamp}-pipeline (--keep-temp)."
		else
			"${containerEngine}" rm -f "${ids[@]}" &> /dev/null  &&  removed+=("${#ids[@]} container(s)")
		fi
	fi

	mapfile -t ids < <("${containerEngine}" image ls -q --filter "label=dev.project=${projectName}" --filter dangling=true 2> /dev/null)
	if ((${#ids[@]})) && "${containerEngine}" rmi "${ids[@]}" &> /dev/null; then removed+=("${#ids[@]} dangling image(s)"); fi

	## Newest first. Keep the tag that just passed and the one before it.
	if ((imagePassed)) && [[ -n "${containerTag}" ]]; then
		mapfile -t oldTags < <("${containerEngine}" image ls "${containerImage}" --filter "label=dev.project=${projectName}" \
			--format '{{.Repository}}:{{.Tag}}' 2> /dev/null | grep -vxF -e "${containerTag}" -e "${containerImage}:<none>" | tail -n +2)
		for tag in "${oldTags[@]}"; do "${containerEngine}" rmi "${tag}" &> /dev/null  &&  removed+=("${tag}"); done
	fi

	"${containerEngine}" builder prune -f --filter until=168h &> /dev/null  ||  true

	after="$(fDocker_Bytes)"
	local freed="?"
	[[ "${before}" =~ ^[0-9]+$ && "${after}" =~ ^[0-9]+$ ]]  &&  freed="$(numfmt --to=si --suffix=B $(( before > after ? before - after : 0 )))"
	fEcho_Clean "Docker cleanup: removed ${removed[*]:-no containers or images}, plus week-old build cache; freed about ${freed}."
}

## Total of docker system df's sizes, in bytes.
fDocker_Bytes(){
	"${containerEngine}" system df --format '{{.Size}}' 2> /dev/null | numfmt --from=si --suffix=B --invalid=ignore 2> /dev/null \
		| awk '{ sub(/B$/, ""); t += $1 } END { printf "%d\n", t }'
}


#••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
fCleanup(){
	## A step still open here is the one that failed or was interrupted.
	fStep_Fail
	## Not inside the container: the outer run owns the folder and the docker side.
	if ((! inContainer)); then
		fContainer_Cleanup  ||  true
		if [[ -n "${runDir}" && -d "${runDir}" ]]; then
			if ((doKeepTemp)); then fEcho_Clean "Kept the run folder: ${runDir}"
			else fSafeRm "${runDir}" > /dev/null  ||  true; fi
		fi
	fi
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
declare -r _hr="•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••"
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

## Cap every step to at most half the cores, a quarter when busy (exports GOMAXPROCS,
## so the go tools spawned below - staticcheck/govulncheck/gosec - inherit it too).
## Also drops this run to nice 19 and idle I/O, which every step inherits.
source "${meDir}/utility/include/cpu-limit.bash"

## GFS rotation for the run-log artifacts (gfs_rotate)
source "${meDir}/utility/include/gfs-rotate.bash"

## Pinned linter/scanner versions (GITNIBLET_STATICCHECK/GOVULNCHECK/GOSEC)
source "${meDir}/utility/include/tool-versions.bash"

## Run folder and the safe removal guard (fTempRun_Make, fSafeRm)
source "${meDir}/utility/include/temp-run.bash"

## Step lines (fStep_Plan, fStep_Start, fStep_End, fStep_Fail)
source "${meDir}/utility/include/steps.bash"

## Fetch and fast-forward before the build (fRemoteSync)
source "${meDir}/utility/include/sync.bash"

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
##		- 20260701 JC: Adapted from the convert-base-v2 template: gitniblet name/paths, build.bash instead of make; cross-compile deferred to the CI/CD backlog item.
##		- 20260701 JC: build.bash moved into cicd/ and now outputs straight to bin/; dropped the staging copy.
##		- 20260701 JC: Wired cross-compile (win-amd64, linux-arm64, win-arm64), the windows zip, and govulncheck.
##		- 20260703 JC: Replaced the post-build bare launch (which opened the TUI) with a --version check. Fixed the git-automation script path - it was missing the base prefix and doubled the cicd/ segment.
##		- 20260703 JC: Blank line between major stages for readability.
##		- 20260704 JC: Added -q/--quiet (unattended, flows to publish) and -m/--message (auto-generated under -q); stage headers use fEcho.
##		- 20260704 JC: Renamed the built executable gitniblet -> gitniblet.
##		- 20260704 JC: Added --quick (skip cross-builds and govulncheck).
##		- 20260704 JC: --quick also skips screenshots; screenshots.bash moved to github/utility/; run screenshots as a stage before publish; always leave a trailing blank line.
##		- 20260704 JC: Preflight now captures the commit message up front (Ctrl+C aborts) instead of a y/n gate; section headers get the letterbox rule; passes --no-prompt to the publish step so it doesn't re-ask.
##		- 20260711 JC: Full cross-platform packaging via goreleaser (binaries for linux/darwin/windows/freebsd amd64+arm64, .deb/.rpm, archives, checksums) plus a Windows .exe installer; split native debug (test/profile) from release (dogfood/package) builds; added --no-arm.
##		- 20260804 JC: Publish step skips its second y/n via the doPromptToContinue setting; the --no-prompt switch no longer exists in that script.
##		- 20261024 JC: Added --no-install, --no-hooks, --no-publish, --no-demo and --no-format, so a full run can leave the box and the tracked tree alone.
##		- 20261024 JC: Local hooks dir is two levels up, since the repo moved into gitniblet-floss/repo.
##		- 20261025 JC: Build through security run in the pinned container by default; --no-container keeps them here.
##		- 20261025 JC: Tests on the other boxes (cicd/utility/remote-test.bash) after build and test; --no-remote skips them.
##		- 20261025 JC: Each run gets its own temp folder, removed at the end along with its containers, dangling images and old image tags; --keep-temp keeps them.
##		- 20261025 JC: Step lines with the count, time and PASS or FAIL, planned up front; the load is checked before each step.
##		- 20261025 JC: The shared flags: --backlog-run-quick with areas, --backlog-run-full, --merge-gate, --long, --list, and the taken ones. --quick keeps fuzz and security. The CLI smoke after the release build.
##		- 20261025 JC: Remote sync first: fetch and fast-forward before the build, stop when diverged; --no-sync skips it.
##		- 20261025 JC: Race tests (the TUI under -race) and Pipeline tests (cicd/utility/*-test.bash) on this host; GITNIBLET_LOG_DIR; the container is polled, not waited on.
##		- 20261025 JC: GITNIBLET_HOOKS_DIR, so the plan's tests pass where ../../private/cicd-hooks is missing.
##		- 20261026 JC: Local hooks dir is ../private/cicd-hooks again, now that the project folder has its own private/.
