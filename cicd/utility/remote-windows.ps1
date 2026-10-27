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
	[int]$Keep = 0,
	[string]$LeakCheck = '1'
)
$ErrorActionPreference = 'Stop'
## The test binaries write UTF-8. With no console this can throw, and the default then only garbles non-ASCII.
try { [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false) } catch { }
(Get-Process -Id $PID).PriorityClass = 'BelowNormal'
if ($LeakCheck -eq '0') { $env:GITNIBLET_LEAKCHECK = '0' }

## Only the folder remote-test.bash made for this run, never a link.
function Remove-RunFolder {
	Set-Location $env:SystemDrive\
	if ($Keep -eq 1) { "kept $($env:COMPUTERNAME):$Work"; return }
	$item = Get-Item -LiteralPath $Work -ErrorAction SilentlyContinue
	if ($Work -match '\\gitniblet-floss\\\d{8}-\d{6}_\d{3}$' -and $item -and $item.PSIsContainer -and -not $item.LinkType) {
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
	## A fake home, as include/temp-run.bash's fConfigHome_Use makes. The old
	## folder name is moved on any default registry lookup, which shows a leak.
	$fakeCfg = "$Work\tmp\home\.config"
	$null = New-Item -ItemType Directory "$fakeCfg\ngdb"
	$env:HOME = "$Work\tmp\home"; $env:XDG_CONFIG_HOME = $env:AppData = $fakeCfg

	## Half the cores, a quarter when busy, never past the caller's cap.
	$cores = [Environment]::ProcessorCount
	## Busy percent over a second, from GetSystemTimes. WMI and the perf counters
	## both refuse a non-admin ssh logon. Kernel time includes idle.
	$load = try {
		Add-Type -Namespace Gitniblet -Name Cpu -MemberDefinition '[DllImport("kernel32.dll")] public static extern bool GetSystemTimes(out long idle, out long kernel, out long user);'
		$i1 = $k1 = $u1 = $i2 = $k2 = $u2 = 0L
		if (-not [Gitniblet.Cpu]::GetSystemTimes([ref]$i1, [ref]$k1, [ref]$u1)) { throw 'GetSystemTimes' }
		Start-Sleep -Seconds 1
		if (-not [Gitniblet.Cpu]::GetSystemTimes([ref]$i2, [ref]$k2, [ref]$u2)) { throw 'GetSystemTimes' }
		$all = ($k2 - $k1) + ($u2 - $u1)
		if ($all -le 0) { throw 'no ticks' }
		[math]::Round(100 * ($all - ($i2 - $i1)) / $all)
	} catch { '?' }
	$jobs = [math]::Floor($cores / 2)
	if ($load -ne '?' -and $load -gt 50) { $jobs = [math]::Floor($cores / 4) }
	$jobs = [math]::Max(1, [math]::Min($jobs, $JobCap))
	$env:GOMAXPROCS = "$jobs"
	"Windows $([Environment]::OSVersion.Version) on $($env:COMPUTERNAME): $cores cores, load $load%, $jobs jobs"

	foreach ($line in Get-Content "$Work\tests\manifest") {
		$name, $dir, $pkg = $line -split ' ', 3
		if (-not $name) { continue }
		$log = "$Work\logs\$name.log"
		"@@gitniblet-test begin $pkg"
		Push-Location -LiteralPath "$Work\$dir"
		## Quoted, since PowerShell splits -test.timeout at the dot.
		& "$Work\tests\$name.test.exe" '-test.v=test2json' '-test.timeout=15m' "-test.parallel=$jobs" *> $log
		$rc = $LASTEXITCODE
		Pop-Location
		Get-Content -LiteralPath $log
		"@@gitniblet-test end $rc"
		if ($rc -ne 0) { $failed++ }
	}
	if ((Test-Path -LiteralPath "$fakeCfg\ngdb") -and -not (Test-Path -LiteralPath "$fakeCfg\gitniblet")) { "PASS config-home" }
	else { "FAIL config-home: the default config folder was used"; $failed++ }

	## The smoke script is bash. Git for Windows brings one; it isn't on PATH.
	$bash = "$env:ProgramFiles\Git\bin\bash.exe"
	if (Test-Path -LiteralPath $bash) {
		$env:GITNIBLET_SMOKE_EXPECT_DATE = '1'
		& $bash ./cicd/utility/smoke-cli.bash ./gitniblet.exe *> "$Work\logs\smoke.log"
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
##		- 20261025 JC: Load from GetSystemTimes, which a non-admin logon can read.
##		- 20261026 JC: Tests run with a fake home, checked after.
##		- 20261026 JC: LeakCheck 0 turns goleak off in the test binaries.
