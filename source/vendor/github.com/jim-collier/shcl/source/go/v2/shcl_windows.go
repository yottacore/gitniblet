// SPDX-License-Identifier: MIT
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

//go:build windows

// Windows has no rename that keeps the destination's ACLs, attributes and named
// streams; ReplaceFile exists for exactly this publish step and carries them
// onto the replacement. shcl.go is deliberately droppable on its own and so
// cannot reach a windows-only symbol, which is why this arrives as a hook: with
// this file the module upgrades the publish, without it a rename still works.
package shcl

import (
	"os"
	"syscall"
	"unsafe"
)

func init() { publishFile = windowsPublishFile }

// REPLACEFILE_WRITE_THROUGH: do not return until the change is on the disk.
const replaceFileWriteThrough = 0x1

// kernel32 by name is loaded from the system directory, not the working one -
// syscall keeps a list of the DLLs that deserve that and kernel32 is on it.
var procReplaceFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReplaceFileW")

// ReplaceFile needs the destination to exist, and it fails rather than skip a
// merge it cannot do (no WRITE_DAC, say), so a create and any failure fall back
// to the plain rename this replaces.
func windowsPublishFile(tmp, target string) error {
	if _, serr := os.Stat(target); serr == nil && replaceFileW(tmp, target) == nil {
		return nil
	}
	return os.Rename(tmp, target)
}

func replaceFileW(tmp, target string) error {
	if ferr := procReplaceFileW.Find(); ferr != nil {
		return ferr
	}
	replaced, rerr := syscall.UTF16PtrFromString(target)
	if rerr != nil {
		return rerr
	}
	replacement, perr := syscall.UTF16PtrFromString(tmp)
	if perr != nil {
		return perr
	}
	r, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(replaced)), uintptr(unsafe.Pointer(replacement)),
		0, uintptr(replaceFileWriteThrough), 0, 0)
	if r == 0 {
		return callErr
	}
	return nil
}
