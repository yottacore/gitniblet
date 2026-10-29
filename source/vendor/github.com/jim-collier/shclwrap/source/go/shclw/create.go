// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
// SPDX-License-Identifier: MIT

package shclw

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"unicode/utf8"
)

var tempSeq atomic.Uint64

// linkFile is os.Link, swapped by a test for a filesystem with no links.
var linkFile = os.Link

// CheckMode refuses a WriteOpts.Mode with more than permission bits in it.
func CheckMode(path string, mode fs.FileMode) error {
	if mode&^fs.ModePerm != 0 {
		return &Error{Kind: ErrBadValue, Path: path, Msg: fmt.Sprintf("mode %#o is more than permission bits", uint32(mode))}
	}
	return nil
}

// Exists is the error for a create-only Write where the name is taken.
func Exists(path string) error {
	return &Error{Kind: ErrExists, Path: path, Msg: "already there; nothing written"}
}

// CreateNew writes data to path only when nothing is there, and is ErrExists
// otherwise. The text is synced under a temp name and hard-linked into place,
// so the name never holds part of it, and of 2 creates at once exactly one
// wins. A filesystem with no hard links gets an exclusive create of path
// itself (design.md, Loading and saving). mode 0 is a plain create's.
func CreateNew(path, data string, mode fs.FileMode) error {
	if err := CheckMode(path, mode); err != nil {
		return err
	}
	tmp, err := writeTemp(path, data, mode)
	if err != nil {
		return createErr(path, err)
	}
	err = linkFile(tmp, path)
	os.Remove(tmp)
	switch {
	case err == nil:
		syncDir(filepath.Dir(path))
		return nil
	case errors.Is(err, fs.ErrExist):
		return Exists(path)
	case noLinks(err):
		return createInPlace(path, data, mode)
	}
	return createErr(path, err)
}

// errInvalidFunction is Windows' ERROR_INVALID_FUNCTION.
const errInvalidFunction = syscall.Errno(1)

// noLinks is a link error that says the filesystem has no hard links. Linux
// gives EPERM, and Windows on FAT gives ERROR_INVALID_FUNCTION, which Go
// doesn't count as unsupported.
func noLinks(err error) bool {
	if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, fs.ErrPermission) {
		return true
	}
	var en syscall.Errno
	return runtime.GOOS == "windows" && errors.As(err, &en) && en == errInvalidFunction
}

// writeTemp writes data to a new file beside path and syncs it.
func writeTemp(path, data string, mode fs.FileMode) (string, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	// shcl's temp names cut the base the same way, to stay under 255 bytes.
	if len(base) > 64 {
		cut := 64
		for cut > 0 && !utf8.RuneStart(base[cut]) {
			cut--
		}
		base = base[:cut]
	}
	var last error
	for attempt := 0; attempt < 8; attempt++ {
		tmp := filepath.Join(dir, "."+base+".new"+strconv.Itoa(os.Getpid())+"."+strconv.FormatUint(tempSeq.Add(1), 10))
		f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, bornMode(mode))
		if errors.Is(err, fs.ErrExist) {
			last = err
			continue
		}
		if err != nil {
			return "", err
		}
		if err := fill(f, data, mode); err != nil {
			os.Remove(tmp)
			return "", err
		}
		return tmp, nil
	}
	return "", last
}

// createInPlace is CreateNew without a temp, for a filesystem with no hard
// links. A crash part way leaves part of the file.
func createInPlace(path, data string, mode fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, bornMode(mode))
	if errors.Is(err, fs.ErrExist) {
		return Exists(path)
	}
	if err != nil {
		return createErr(path, err)
	}
	if err := fill(f, data, mode); err != nil {
		os.Remove(path) // ours, made just above
		return createErr(path, err)
	}
	syncDir(filepath.Dir(path))
	return nil
}

// bornMode is what a file is opened with. One with a mode is born private,
// and gets its mode exactly by chmod, past the umask.
func bornMode(mode fs.FileMode) fs.FileMode {
	if mode == 0 {
		return 0o666
	}
	return 0o600
}

func fill(f *os.File, data string, mode fs.FileMode) error {
	_, err := f.WriteString(data)
	if err == nil && mode != 0 && runtime.GOOS != "windows" {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync() // some filesystems can't; the file itself is synced
		d.Close()
	}
}

func createErr(path string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return &Error{Kind: ErrNotFound, Path: path, Err: err}
	}
	return &Error{Kind: ErrIO, Path: path, Err: err}
}
