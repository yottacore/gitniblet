// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"

	shcl "github.com/jim-collier/shcl/source/go"
)

// Shared load/save plumbing for the three registry files. They differ only in
// how strict they are about a garbled file, which mirrors what each one costs
// if it is silently half-read: a database record must be reported broken so the
// picker can gray it out, while settings and logins fall back to defaults.

// loadStrict reads an SHCL file and refuses anything with an error diagnostic.
// Used for the per-database record, where a corrupt file has to surface as
// unopenable rather than quietly losing a path.
func loadStrict(path string) (*shcl.Document, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return shcl.ParseWith(string(text), shcl.Strict)
}

// loadLenient reads an SHCL file at the default strictness, yielding an empty
// document when it is missing or unreadable. Callers get defaults on a first
// run and never have to handle an error.
func loadLenient(path string) *shcl.Document {
	text, err := os.ReadFile(path)
	if err != nil {
		return shcl.New()
	}
	return shcl.Parse(string(text))
}

// save writes a document's canonical form, creating nothing but the file - the
// caller makes the directory, since only it knows the right mode.
func save(path string, doc *shcl.Document, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.WriteString(doc.ToCanonical())
	if cerr := f.Close(); err == nil {
		err = cerr // a close error here means a truncated record on disk
	}
	return err
}

// setIfSet writes a string leaf only when it carries something, so an unset
// optional field is absent from the file rather than present-but-empty.
func setIfSet(doc *shcl.Document, path, val string) {
	if val != "" {
		doc.SetString(path, val)
	}
}
