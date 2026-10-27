// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import (
	"os"

	"github.com/yottacore/gitniblet/internal/core/shclio"
)

// Shared load/save plumbing for the three registry files. They differ only in
// how strict they are about a garbled file, which mirrors what each one costs
// if it is silently half-read: a database record must be reported broken so the
// picker can gray it out, while settings and logins fall back to defaults.

// loadStrict reads an SHCL file and refuses anything with an error diagnostic.
// Used for the per-database record, where a corrupt file has to surface as
// unopenable rather than quietly losing a path.
func loadStrict(path string) (*shclio.Doc, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return shclio.Load(string(text), shclio.Strict)
}

// loadLenient reads an SHCL file at the default strictness, yielding an empty
// document when it is missing or unreadable. Callers get defaults on a first
// run and never have to handle an error.
func loadLenient(path string) *shclio.Doc {
	text, err := os.ReadFile(path)
	if err != nil {
		return shclio.New()
	}
	doc, _ := shclio.Load(string(text), shclio.Standard) // only Strict can fail
	return doc
}

// save writes a document, naming its format, creating nothing but the file - the
// caller makes the directory, since only it knows the right mode.
func save(path string, doc *shclio.Doc, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.WriteString(shclio.Text(doc))
	if cerr := f.Close(); err == nil {
		err = cerr // a close error here means a truncated record on disk
	}
	return err
}

// setIfSet writes a string leaf only when it carries something, so an unset
// optional field is absent from the file rather than present-but-empty.
func setIfSet(doc *shclio.Doc, path, val string) error {
	if val == "" {
		return nil
	}
	return doc.Put(path, val)
}
