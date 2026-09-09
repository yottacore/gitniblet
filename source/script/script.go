// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package script is the scripting seam between the open-source core and a
// scripting host. The host (Lua) is an enterprise feature, so the open-source
// build registers no provider: --script is rejected and DDL code: hooks do not
// run. The enterprise build registers a provider and both light up. The core
// never imports the host, and the host never imports the core's internal
// packages - everything crosses through the interfaces here.
package script

// DB is the write surface a script or trigger runs against. The core's CRUD API
// satisfies it through an adapter, so a host binds to these methods without
// importing the core.
type DB interface {
	Create(table string, fields map[string]string) (string, error)
	Get(table, id string) (map[string]string, bool, error)
	Update(table, id string, fields map[string]string) error
	SetField(table, id, field, value string) error
	MarkDelete(table, id string) error
	Delete(table, id string) error
	Query(query string, args ...any) ([]map[string]string, error)
	CommentAdd(table, id, text string) (string, error)
	CommentsFor(table, id string) ([]map[string]string, error)
	AttachURI(table, id, uri, desc string) (string, error)
	AttachmentsFor(table, id string) ([]Attachment, error)
	SetTrigger(Trigger)
}

// Attachment mirrors the core's attachment record without pulling in its type.
type Attachment struct {
	ID          string
	Kind        string // "uri" or "copy"
	Label       string // the uri, or name.extension
	Description string
}

// Trigger receives write events. Its method set matches the core's trigger
// hook, so the adapter can install a value of this type on the API. id is "" on
// create.
type Trigger interface {
	BeforeField(table, field, value string) (string, error)
	BeforeUpdate(table, id string, fields map[string]string) error
	AfterField(table, id, field, value string)
	AfterUpdate(table, id string)
}

// CodeSet is the DDL's code: bindings, already collected from the schema: which
// function runs for each app, table, and field hook. The host validates these
// names against the loaded sidecar itself.
type CodeSet struct {
	App   map[string]string                       // hook -> function
	Table map[string]map[string]string            // table -> hook -> function
	Field map[string]map[string]map[string]string // table -> field -> hook -> function
}

// Empty reports whether the DDL defines no code: hooks at all.
func (c CodeSet) Empty() bool {
	return len(c.App) == 0 && len(c.Table) == 0 && len(c.Field) == 0
}

// Provider is a scripting host. The enterprise build registers one.
type Provider interface {
	// RunFile executes a script file against the database.
	RunFile(db DB, path string) error
	// Attach loads the sidecar at scriptPath and installs the DDL's code: hooks
	// as a Trigger on db. ddlPath and logDir are passed through to the app-level
	// open hooks. Returned strings are non-fatal warnings.
	Attach(db DB, scriptPath, ddlPath, logDir string, code CodeSet) ([]string, error)
}

var registered Provider

// Register installs the scripting provider. The enterprise main calls this
// before dispatch; the open-source main does not.
func Register(p Provider) { registered = p }

// Available reports whether a scripting provider is registered.
func Available() bool { return registered != nil }
