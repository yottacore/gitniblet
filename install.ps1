#!/usr/bin/env pwsh
#Requires -Version 7.0

<#
.SYNOPSIS
	Download, verify and install the ngdb executable from a GitHub release.

.DESCRIPTION
	Runs on Windows, Linux and macOS under PowerShell 7. Safe to re-run: an
	install that is already at the requested version is left alone.

.PARAMETER Release
	'stable' takes the newest full release, 'dev' the newest of any kind. With
	neither, stable is preferred and dev is the fallback.

.PARAMETER Target
	'user' installs under the user profile (the default), 'system' installs for
	everyone and needs an elevated session.

.PARAMETER Arch
	Override the detected CPU architecture: x64 or arm64.

.PARAMETER Tag
	Install one exact release, e.g. v1.0.0-beta.1.

.PARAMETER List
	Show the available releases and stop.

.PARAMETER Yes
	Skip the confirmation prompt.

.EXAMPLE
	& ([scriptblock]::Create((irm 'https://raw.githubusercontent.com/jim-collier/nano-git-db/main/install.ps1')))

.EXAMPLE
	.\install.ps1 -Release dev -Target user
#>

## Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
## Licensed under The MIT License (MIT). Full text at:
##   https://mit-license.org/
## SPDX-License-Identifier: MIT

[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
	Justification = 'This is a console installer; its output is UI text, not data.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', '',
	Justification = 'Read from the script-scope functions below, which the analyzer does not follow.')]
[CmdletBinding()]
param(
	[ValidateSet('stable', 'dev')] [string] $Release,
	[ValidateSet('user', 'system')] [string] $Target,
	[ValidateSet('x64', 'amd64', 'arm64')] [string] $Arch,
	[string] $Tag,
	[switch] $List,
	[switch] $Yes
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

#-----------------------------------------------------------------------------
## Settings

$RepoOwner = 'jim-collier'
$RepoName  = 'nano-git-db'
$ExeName   = 'ngdb'
$ApiBase   = "https://api.github.com/repos/$RepoOwner/$RepoName"
$DownBase  = "https://github.com/$RepoOwner/$RepoName/releases/download"

#-----------------------------------------------------------------------------
## Output

function Write-Section { param([string] $Text) Write-Host "[ $Text ]" }
function Write-Line    { param([string] $Text = '') Write-Host $Text }

#-----------------------------------------------------------------------------
## Platform

## Release assets are named by Go's own OS and architecture words.
function Get-TargetOsName {
	if ($IsWindows) { return 'windows' }
	if ($IsMacOS)   { return 'darwin' }
	if ($IsLinux)   { return 'linux' }
	throw 'unsupported operating system'
}

function Get-TargetArch {
	if ($Arch) {
		if ($Arch -eq 'arm64') { return 'arm64' }
		return 'amd64'
	}
	switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
		'X64'   { return 'amd64' }
		'Arm64' { return 'arm64' }
		default { throw "unsupported architecture: $_ (try -Arch x64 or -Arch arm64)" }
	}
}

#-----------------------------------------------------------------------------
## Releases

## Invoke-RestMethod hands its whole JSON array down as one object, so it goes
## through a variable first - that is what makes the pipeline see one release at
## a time instead of a single array.
function Get-ReleaseList {
	$response = Invoke-RestMethod -Uri "$ApiBase/releases?per_page=100" -Headers @{ 'User-Agent' = $ExeName }
	$response | Where-Object { -not $_.draft }
}

function Resolve-ReleaseTag {
	if ($Tag) { return $Tag }
	$releases = @(Get-ReleaseList)
	if ($releases.Count -eq 0) { throw "no releases found at github.com/$RepoOwner/$RepoName" }
	if ($Release -ne 'dev') {
		$stable = $releases | Where-Object { -not $_.prerelease } | Select-Object -First 1
		## No full release yet, so fall back to the newest pre-release rather
		## than leaving a first-time user with nothing to install.
		if ($stable) { return $stable.tag_name }
	}
	return $releases[0].tag_name
}

#-----------------------------------------------------------------------------
## Install

function Get-InstallDirectory {
	param([string] $TargetOs)
	$system = ($Target -eq 'system')
	if ($TargetOs -eq 'windows') {
		if ($system) { return (Join-Path $env:ProgramFiles $ExeName) }
		return (Join-Path $env:LOCALAPPDATA "Programs\$ExeName")
	}
	if ($system) { return '/usr/local/bin' }
	return (Join-Path $HOME '.local/bin')
}

function Get-InstalledVersion {
	param([string] $ExePath)
	if (-not (Test-Path -LiteralPath $ExePath)) { return $null }
	try { return (& $ExePath --version).Split(' ')[1] } catch { return $null }
}

function Confirm-Plan {
	if ($Yes) { return }
	$reply = Read-Host 'Proceed? (y/N)'
	if ($reply -notmatch '^(y|yes)$') {
		Write-Line
		Write-Section 'Nothing was changed.'
		Write-Line
		exit 0
	}
}

## The user PATH is per-user in the registry, so a system install is left to the
## machine PATH an administrator already manages.
function Add-ToUserPath {
	param([string] $Directory)
	if (-not $IsWindows) { return }
	$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
	if ($userPath -and ($userPath -split ';' | Where-Object { $_ -eq $Directory })) { return }
	$combined = if ($userPath) { "$userPath;$Directory" } else { $Directory }
	[Environment]::SetEnvironmentVariable('Path', $combined, 'User')
	Write-Line "  Added $Directory to your PATH - open a new terminal to pick it up."
}

#-----------------------------------------------------------------------------
## Main

function Invoke-Install {
	Write-Line

	if ($List) {
		Write-Section 'Releases'
		foreach ($rel in @(Get-ReleaseList)) {
			if ($rel.prerelease) { Write-Line "  $($rel.tag_name)  (pre-release)" }
			else                 { Write-Line "  $($rel.tag_name)" }
		}
		Write-Line
		return
	}

	$targetOs   = Get-TargetOsName
	$targetArch = Get-TargetArch

	Write-Section 'Looking up the release'
	$tag     = Resolve-ReleaseTag
	$version = $tag -replace '^v', ''
	$archive = if ($targetOs -eq 'windows') {
		"$ExeName-$version-$targetOs-$targetArch.zip"
	} else {
		"$ExeName-$version-$targetOs-$targetArch.tar.gz"
	}
	$exeFile     = if ($targetOs -eq 'windows') { "$ExeName.exe" } else { $ExeName }
	$installDir  = Get-InstallDirectory -TargetOs $targetOs
	$installPath = Join-Path $installDir $exeFile
	$current     = Get-InstalledVersion -ExePath $installPath

	Write-Line
	Write-Section 'Plan'
	Write-Line "  release:  $tag"
	Write-Line "  package:  $archive"
	Write-Line "  install:  $installPath"
	if ($current) { Write-Line "  present:  $current" }
	Write-Line

	if ($current -eq $version) {
		Write-Section "$ExeName $version is already installed at $installPath."
		Write-Line
		return
	}

	Confirm-Plan

	$work = Join-Path ([System.IO.Path]::GetTempPath()) ("$ExeName-install-" + [guid]::NewGuid())
	New-Item -ItemType Directory -Path $work -Force | Out-Null
	try {
		Write-Line
		Write-Section 'Downloading'
		$archivePath   = Join-Path $work $archive
		$checksumsPath = Join-Path $work 'checksums.txt'
		Invoke-WebRequest -Uri "$DownBase/$tag/$archive"      -OutFile $archivePath
		Invoke-WebRequest -Uri "$DownBase/$tag/checksums.txt" -OutFile $checksumsPath

		Write-Section 'Verifying'
		$expected = (Get-Content $checksumsPath |
			Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } |
			Select-Object -First 1)
		if (-not $expected) { throw "$archive is not listed in checksums.txt" }
		$want = ($expected -split '\s+')[0]
		$got  = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash
		if ($got -ne $want.ToUpperInvariant()) {
			throw "checksum mismatch on $archive - the download was not what the release published"
		}

		Write-Section 'Unpacking'
		if ($archive.EndsWith('.zip')) {
			Expand-Archive -Path $archivePath -DestinationPath $work -Force
		} else {
			tar -xzf $archivePath -C $work
			if ($LASTEXITCODE -ne 0) { throw "cannot unpack $archive" }
		}
		$unpacked = Join-Path $work $exeFile
		if (-not (Test-Path -LiteralPath $unpacked)) { throw "$exeFile is not in the archive" }

		Write-Section 'Installing'
		New-Item -ItemType Directory -Path $installDir -Force | Out-Null
		Copy-Item -LiteralPath $unpacked -Destination $installPath -Force
		if (-not $IsWindows) { chmod 0755 $installPath }

		Write-Line
		Write-Section "Installed $ExeName $version to $installPath"
		if ($Target -ne 'system') { Add-ToUserPath -Directory $installDir }
		Write-Line "  Getting started:  $ExeName --help"
		Write-Line
	}
	finally {
		Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
	}
}

Invoke-Install
