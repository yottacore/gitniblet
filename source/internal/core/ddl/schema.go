// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package ddl

import (
	"embed"
	"sync"

	shcl "github.com/jim-collier/shcl/source/go"
)

// schema.shcl states the DDL's vocabulary for shcl's validator. Embedding it
// keeps the one static binary intact and keeps the vocabulary reviewable as a
// plain file rather than buried in Go string literals.
//
// The two shapes that used to be generated here - layout blocks nesting
// recursively, and every database:/ui: path repeated without its wrapper - are
// both schema fragments now, so the file is the whole vocabulary.
//
//go:embed schema.shcl
var schemaFS embed.FS

var (
	schemaOnce sync.Once
	schemaDocV *shcl.Document
)

// schemaDoc returns the validation schema. Parsed once - it is immutable, and
// Validate only reads it, so every DDL load shares the one document.
func schemaDoc() *shcl.Document {
	schemaOnce.Do(func() {
		text, err := schemaFS.ReadFile("schema.shcl")
		if err != nil {
			// Embedded at build time, so this cannot fail in a shipped binary;
			// an empty schema degrades to "validate nothing" rather than
			// refusing to open a database over a build problem.
			schemaDocV = shcl.New()
			return
		}
		schemaDocV = shcl.Parse(string(text))
	})
	return schemaDocV
}
