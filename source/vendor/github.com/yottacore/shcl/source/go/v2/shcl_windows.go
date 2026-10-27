// SPDX-License-Identifier: MIT
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

//go:build windows

// Windows has no rename that keeps the destination's ACLs, attributes and named
// streams; ReplaceFile exists for exactly this publish step and copies them
// onto the replacement. shcl.go is deliberately droppable on its own and so
// cannot reach a windows-only symbol, which is why this arrives as a hook: with
// this file the module upgrades the publish, without it a rename still works.

package shcl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func init() {
	publishFile = windowsPublishFile
	publishNewFile = windowsPublishNewFile
	carriedAttrs = windowsCarriedAttrs
	restoreAttrs = windowsRestoreAttrs
	notADiskFile = windowsNotADiskFile
	createTemp = windowsCreateTemp
	osWriteStatus = windowsWriteStatus
}

// The system's own code, from the table every binding shares: Go files a
// held file and a missing network path under no reason at all.
func windowsWriteStatus(err error) (WriteStatus, bool) {
	var code syscall.Errno
	if !errors.As(err, &code) {
		return WriteOk, false
	}
	switch code {
	case 2, 3, 15, 53, 67, 267:
		return WriteNotFound, true
	case 5, 32, 33, 1224:
		return WritePermissionDenied, true
	case 39, 112, 1295:
		return WriteDiskFull, true
	case 19:
		return WriteReadOnly, true
	}
	return WriteOk, false
}

// A device has the same ARCHIVE bit an ordinary file does, so an attribute
// test cannot tell them apart. The handle is the OS's own answer, and it keeps
// an exotic but real path (a volume-prefixed `\\.\C:\dir\file`) out of the
// device case. CON refuses that open outright (ERROR_INVALID_PARAMETER) and a
// serial port nobody has answers not-found, so a failed open cannot mean
// "nothing is there": a reserved name resolves into the device namespace from
// whatever directory it is typed in, and the full path is where that shows.
func windowsNotADiskFile(path string) bool {
	const fileReadAttributes = 0x80
	p, perr := syscall.UTF16PtrFromString(path)
	if perr != nil {
		return false
	}
	h, herr := syscall.CreateFile(p, fileReadAttributes,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if herr == nil {
		kind, kerr := syscall.GetFileType(h)
		_ = syscall.CloseHandle(h) // opened for attributes only, so nothing is lost
		return kerr == nil && kind != syscall.FILE_TYPE_DISK
	}
	full, ferr := syscall.FullPath(path)
	if ferr != nil || !strings.HasPrefix(full, `\\.\`) {
		return false
	}
	// A drive after the prefix is a volume path to a file that is not there
	// yet, which a create has to be let through (20260926 item 8).
	rest := full[4:]
	drive := len(rest) >= 2 && rest[1] == ':' && ((rest[0] >= 'A' && rest[0] <= 'Z') || (rest[0] >= 'a' && rest[0] <= 'z'))
	return !drive
}

var (
	advapi32                         = syscall.NewLazyDLL("advapi32.dll")
	procGetFileSecurityW             = advapi32.NewProc("GetFileSecurityW")
	procSetFileSecurityW             = advapi32.NewProc("SetFileSecurityW")
	procGetSecurityDescriptorControl = advapi32.NewProc("GetSecurityDescriptorControl")
	procSetSecurityDescriptorControl = advapi32.NewProc("SetSecurityDescriptorControl")
)

// windowsCreateTemp is the exclusive create of a save's temp file. Over an
// existing file a plain create takes the directory's ACL, so the new text of a
// private config sat where others could read it until the replace, and a
// fallback move published that wider ACL. The temp is born with the file's
// DACL instead. Best effort: when that cannot be read, the directory's it is,
// as for a new file.
func windowsCreateTemp(from, tmp string, perm os.FileMode) (*os.File, error) {
	if from == "" {
		return os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	}
	const (
		daclSecurityInformation = 0x4
		seDaclAutoInheritReq    = 0x0100
		seDaclAutoInherited     = 0x0400
	)
	dst, err := syscall.UTF16PtrFromString(tmp)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: tmp, Err: err}
	}
	// uint64 words, since a security descriptor wants more than byte alignment.
	var sd []uint64
	if src, serr := syscall.UTF16PtrFromString(from); serr == nil {
		var need uint32
		_, _, _ = procGetFileSecurityW.Call(uintptr(unsafe.Pointer(src)), daclSecurityInformation, 0, 0, uintptr(unsafe.Pointer(&need)))
		if need > 0 {
			sd = make([]uint64, (need+7)/8)
			r, _, _ := procGetFileSecurityW.Call(uintptr(unsafe.Pointer(src)), daclSecurityInformation,
				uintptr(unsafe.Pointer(&sd[0])), uintptr(len(sd)*8), uintptr(unsafe.Pointer(&need)))
			if r == 0 {
				sd = nil
			}
		}
	}
	var sa *syscall.SecurityAttributes
	if sd != nil {
		sa = &syscall.SecurityAttributes{
			Length:             uint32(unsafe.Sizeof(syscall.SecurityAttributes{})),
			SecurityDescriptor: uintptr(unsafe.Pointer(&sd[0])),
		}
	}
	h, err := syscall.CreateFile(dst, syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		sa, syscall.CREATE_NEW, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: tmp, Err: err}
	}
	// A create takes the ACEs but drops the auto-inherited mark, and without it
	// a later change to the directory's ACL does not reach the file once a
	// fallback move makes the temp the config. Setting the same DACL again with
	// the request bit puts it back.
	if sd != nil {
		var control uint16
		var revision uint32
		p := uintptr(unsafe.Pointer(&sd[0]))
		r, _, _ := procGetSecurityDescriptorControl.Call(p, uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
		if r != 0 && control&seDaclAutoInherited != 0 {
			if r, _, _ = procSetSecurityDescriptorControl.Call(p, seDaclAutoInheritReq, seDaclAutoInheritReq); r != 0 {
				_, _, _ = procSetFileSecurityW.Call(uintptr(unsafe.Pointer(dst)), daclSecurityInformation, p)
			}
		}
	}
	return os.NewFile(uintptr(h), tmp), nil
}

// A move without MOVEFILE_REPLACE_EXISTING refuses a target that exists, which
// is all a create needs, and it works on volumes with no hard links.
func windowsPublishNewFile(tmp, target string) error {
	from, ferr := syscall.UTF16PtrFromString(tmp)
	if ferr != nil {
		return ferr
	}
	to, terr := syscall.UTF16PtrFromString(target)
	if terr != nil {
		return terr
	}
	return syscall.MoveFile(from, to)
}

// ReplaceFile's documented preserve list is creation time, short name, object
// id, DACLs, security attributes, encryption, compression and named streams -
// not the basic attributes - and the fallback rename keeps nothing at all, so
// hidden and system are re-applied by hand after the publish. Read-only is
// handled separately: it has to come OFF before the publish.
const carriedFileAttrs = syscall.FILE_ATTRIBUTE_HIDDEN | syscall.FILE_ATTRIBUTE_SYSTEM

func windowsCarriedAttrs(fi os.FileInfo) uint32 {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return 0
	}
	return d.FileAttributes & carriedFileAttrs
}

func windowsRestoreAttrs(path string, bits uint32) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	now, aerr := syscall.GetFileAttributes(p)
	if aerr != nil {
		return
	}
	// Best effort, like the mode on POSIX: the save itself went through.
	_ = syscall.SetFileAttributes(p, now|bits)
}

// REPLACEFILE_WRITE_THROUGH: asked for, and documented by Microsoft as not
// supported by ReplaceFile. Durability here rests on the file's own fsync
// before the publish; the fallback move's own WRITE_THROUGH is the one that
// means something.
const replaceFileWriteThrough = 0x1

// kernel32 by name is loaded from the system directory, not the working one -
// syscall keeps a list of the DLLs that deserve that and kernel32 is on it.
var procReplaceFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReplaceFileW")

// A scanner or an indexer that opens the fresh temp file blocks the replace and
// the rename both, for a moment, and one try made that a failed save.
const (
	publishTries   = 5
	publishPauseMs = 50
)

// ReplaceFile needs the destination to exist, and it fails rather than skip a
// merge it cannot do (no WRITE_DAC, say), so a create and any failure fall back
// to the plain rename this replaces.
//
// ReplaceFile is given a backup name, because without one a failure between its
// two moves deletes the old file (1176) or leaves it under a name nobody is
// told (1177). With one, 1177 leaves it at the backup and nothing at the
// target, so the old file is put back. If even that fails, neither file is
// removed and the error says where they are.
func windowsPublishFile(tmp, target string) error {
	backup := backupName(tmp)
	movedAway := false
	var err error
	for tries := 1; ; tries++ {
		if !movedAway && isThere(target) {
			// The save has gone through, so a backup that will not come off
			// is left rather than failing it.
			if replaceFileW(tmp, target, backup) == nil {
				_ = os.Remove(backup)
				return nil
			}
			movedAway = !isThere(target) && isThere(backup)
		}
		if err = os.Rename(tmp, target); err == nil {
			if movedAway {
				_ = os.Remove(backup)
			}
			return nil
		}
		if tries == publishTries {
			break
		}
		time.Sleep(publishPauseMs * time.Millisecond)
	}
	if isThere(target) || (movedAway && windowsPublishNewFile(backup, target) == nil) {
		_ = os.Remove(tmp) // the old file is in place, and err is the one to report
		return err
	}
	if movedAway {
		return fmt.Errorf("%w; the old file is left at %s and the new text at %s", osReason(err), backup, tmp)
	}
	return fmt.Errorf("%w; the new text is left at %s", osReason(err), tmp)
}

func isThere(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The temp name with its `.tmp` swapped for `.bak`, so the two sit side by side
// and are the same length. The last `.tmp` is the one the save added.
func backupName(tmp string) string {
	dir, name := filepath.Split(tmp)
	if at := strings.LastIndex(name, ".tmp"); at >= 0 {
		return dir + name[:at] + ".bak" + name[at+4:]
	}
	return tmp + ".bak"
}

func replaceFileW(tmp, target, backup string) error {
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
	kept, kerr := syscall.UTF16PtrFromString(backup)
	if kerr != nil {
		return kerr
	}
	r, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(replaced)), uintptr(unsafe.Pointer(replacement)),
		uintptr(unsafe.Pointer(kept)), uintptr(replaceFileWriteThrough), 0, 0)
	if r == 0 {
		return callErr
	}
	return nil
}
