@echo off
SETLOCAL

::	Purpose:
::		Lets 'dogfood_gitniblet' be run from cmd.exe, Win+R and a shortcut, none of
::		which can run a .ps1 directly. Runs in this window, so gitniblet's output
::		stays readable, and passes everything through.
::	History: At bottom of script.

::	Copyright (C) 2026 Jim Collier
::	Licensed under The MIT License (MIT). Full text at:
::		https://mit-license.org/
::	SPDX-License-Identifier: MIT

::----------------------------------------------------------------------------
:MAIN

	:: Four levels up from mswin\cli\by-self\cmd\ is the util dir, beside which
	:: 0_crossplatform sits. The repo keeps all three files side by side. The
	:: Dropbox spelling comes before the 'synced' junction, which Windows 11 can
	:: refuse as an untrusted mount point.
	set PSFILE=%~dp0..\..\..\..\0_crossplatform\dogfood_gitniblet.ps1
	if exist "%PSFILE%" goto :OK005
	set PSFILE=%~dp0dogfood_gitniblet.ps1
	if exist "%PSFILE%" goto :OK005
	set PSFILE=%USERPROFILE%\Dropbox\0-0\common\exec\util\0_crossplatform\dogfood_gitniblet.ps1
	if exist "%PSFILE%" goto :OK005
	set PSFILE=%USERPROFILE%\synced\0-0\common\exec\util\0_crossplatform\dogfood_gitniblet.ps1
	if exist "%PSFILE%" goto :OK005
		echo dogfood_gitniblet: dogfood_gitniblet.ps1 not found
		goto :ERROR
	:OK005

	:: PowerShell 7 when it is there, else Windows PowerShell 5.1.
	set PSEXE=pwsh.exe
	where /q pwsh.exe
	if errorlevel 1 set PSEXE=powershell.exe

	%PSEXE% -NoProfile -ExecutionPolicy Bypass -File "%PSFILE%" %*
	set RC=%ERRORLEVEL%

ENDLOCAL & exit /b %RC%

::----------------------------------------------------------------------------
:ERROR
ENDLOCAL & exit /b 1

::	History:
::		- 20261028 JC: Created from shcl's dogfood_shcl.cmd.
