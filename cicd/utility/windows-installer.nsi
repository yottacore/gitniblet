# Single-file Windows installer for ngdb. The app is one static pure-Go binary,
# so "bundle the runtimes" is inherent - there is nothing else to ship. Compiled
# on Linux with makensis (no wine). Driven by windows-installer.bash, which
# passes the binary, arch, version, and output path as /D defines.
#
# Re-running upgrades an existing install: the prior copy is uninstalled first
# (registry-detected), then the new binary lands in the same dir and PATH entry.

# Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
# Licensed under The MIT License (MIT). Full text at:
#	https://mit-license.org/
# SPDX-License-Identifier: MIT

!ifndef ARCH
	!define ARCH "amd64"
!endif
!ifndef VERSION
	!define VERSION "0.0.0"
!endif
!ifndef BINARY
	!error "BINARY define required (path to ngdb.exe)"
!endif
!ifndef OUTFILE
	!define OUTFILE "ngdb-setup.exe"
!endif

!include "MUI2.nsh"
!include "StrFunc.nsh"
${StrStr}

Name "nano-git-db (${ARCH})"
OutFile "${OUTFILE}"
Unicode true
InstallDir "$PROGRAMFILES64\ngdb"
InstallDirRegKey HKLM "Software\ngdb" "InstallDir"
RequestExecutionLevel admin

!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\ngdb"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

# Upgrade path: if a prior version registered an uninstaller, run it silently
# before installing the new copy so we never stack old files or PATH entries.
Function .onInit
	ReadRegStr $0 HKLM "${UNINST_KEY}" "UninstallString"
	StrCmp $0 "" done 0
	ReadRegStr $1 HKLM "Software\ngdb" "InstallDir"
	ExecWait '"$0" /S _?=$1'
	done:
FunctionEnd

Section "Install"
	SetOutPath "$INSTDIR"
	File "/oname=ngdb.exe" "${BINARY}"

	WriteRegStr HKLM "Software\ngdb" "InstallDir" "$INSTDIR"
	WriteRegStr HKLM "${UNINST_KEY}" "DisplayName"     "nano-git-db"
	WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion"  "${VERSION}"
	WriteRegStr HKLM "${UNINST_KEY}" "Publisher"       "nano-git-db"
	WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" "$INSTDIR\uninstall.exe"
	WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon"     "$INSTDIR\ngdb.exe"
	WriteUninstaller "$INSTDIR\uninstall.exe"

	# Add the install dir to the system PATH, but only once (guard against a
	# duplicate if the same dir is already present).
	ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path"
	${StrStr} $1 "$0" "$INSTDIR"
	StrCmp $1 "" 0 pathdone
		WriteRegExpandStr HKLM "SYSTEM\CurrentControlSet\Control\Session Manager\Environment" "Path" "$0;$INSTDIR"
		SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
	pathdone:
SectionEnd

Section "Uninstall"
	Delete "$INSTDIR\ngdb.exe"
	Delete "$INSTDIR\uninstall.exe"
	RMDir  "$INSTDIR"
	DeleteRegKey HKLM "${UNINST_KEY}"
	DeleteRegKey HKLM "Software\ngdb"
SectionEnd
