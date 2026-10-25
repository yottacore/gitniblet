##	Purpose:
##		The Windows side of remote-test.bash. Unpacks the bundle, runs each Go
##		test binary from its package folder, then the CLI smoke through Git's
##		bash, and removes its work folder after.
##		- Each binary's -test.v=test2json output goes to stdout between a begin
##		  and an end line, for testid remote on the calling host, which prints
##		  one line per test with its ID.
##	Syntax:
##		pwsh -NoProfile -File remote-windows.ps1 <work folder> <most jobs> <keep: 0|1>
##	History: At bottom of this file.

##	Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

param(
	[Parameter(Mandatory)][string]$Work,
	[int]$JobCap = 4,
	[int]$Keep = 0
)
$ErrorActionPreference = 'Stop'
## The test binaries write UTF-8. With no console this can throw, and the default then only garbles non-ASCII.
try { [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false) } catch { }
(Get-Process -Id $PID).PriorityClass = 'BelowNormal'

## Only the folder remote-test.bash made for this run, never a link.
function Remove-RunFolder {
	Set-Location $env:SystemDrive\
	if ($Keep -eq 1) { "kept $($env:COMPUTERNAME):$Work"; return }
	$item = Get-Item -LiteralPath $Work -ErrorAction SilentlyContinue
	if ($Work -match '\\nano-git-db-floss\\\d{8}-\d{6}_\d{3}$' -and $item -and $item.PSIsContainer -and -not $item.LinkType) {
		Remove-Item -LiteralPath $Work -Recurse -Force
	} else {
		Write-Warning "remote-windows: left $Work, not a run folder"
	}
}

$failed = 0
try {
	Set-Location -LiteralPath $Work
	tar -xzf bundle.tgz
	if ($LASTEXITCODE -ne 0) { throw "tar failed" }
	$null = New-Item -ItemType Directory tmp, logs
	$env:TMP = $env:TEMP = $env:TMPDIR = "$Work\tmp"

	## Half the cores, a quarter when busy, never past the caller's cap.
	$cores = [Environment]::ProcessorCount
	## WMI refuses a non-admin ssh logon, so the load may be unknown.
	$load = try { (Get-CimInstance Win32_Processor -ErrorAction Stop | Measure-Object LoadPercentage -Average).Average } catch { '?' }
	$jobs = [math]::Floor($cores / 2)
	if ($load -ne '?' -and $load -gt 50) { $jobs = [math]::Floor($cores / 4) }
	$jobs = [math]::Max(1, [math]::Min($jobs, $JobCap))
	$env:GOMAXPROCS = "$jobs"
	"Windows $([Environment]::OSVersion.Version) on $($env:COMPUTERNAME): $cores cores, load $load%, $jobs jobs"

	foreach ($line in Get-Content "$Work\tests\manifest") {
		$name, $dir, $pkg = $line -split ' ', 3
		if (-not $name) { continue }
		$log = "$Work\logs\$name.log"
		"@@ngdb-test begin $pkg"
		Push-Location -LiteralPath "$Work\$dir"
		## Quoted, since PowerShell splits -test.timeout at the dot.
		& "$Work\tests\$name.test.exe" '-test.v=test2json' '-test.timeout=15m' "-test.parallel=$jobs" *> $log
		$rc = $LASTEXITCODE
		Pop-Location
		Get-Content -LiteralPath $log
		"@@ngdb-test end $rc"
		if ($rc -ne 0) { $failed++ }
	}

	## The smoke script is bash. Git for Windows brings one; it isn't on PATH.
	$bash = "$env:ProgramFiles\Git\bin\bash.exe"
	if (Test-Path -LiteralPath $bash) {
		$env:NGDB_SMOKE_EXPECT_DATE = '1'
		& $bash ./cicd/utility/smoke-cli.bash ./ngdb.exe *> "$Work\logs\smoke.log"
		if ($LASTEXITCODE -eq 0) { "PASS smoke-cli" }
		else { "FAIL smoke-cli"; Get-Content "$Work\logs\smoke.log" -Tail 20; $failed++ }
	} else {
		"SKIP smoke-cli: no Git bash at $bash"
	}
} catch {
	"FAIL remote-windows: $_"
	$failed++
} finally {
	Remove-RunFolder
}
exit [int]($failed -ne 0)


##	History:
##		- 20261025 JC: Created.
##		- 20261025 JC: -test.parallel passed.
##		- 20261025 JC: Raw test2json output per binary, for one line per test on the host.
