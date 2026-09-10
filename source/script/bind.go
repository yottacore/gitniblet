// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package script

import (
	"fmt"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// apiDB adapts the core CRUD API to the DB interface the host sees. Only
// AttachmentsFor and SetTrigger need translation; the rest pass straight
// through.
type apiDB struct{ api *crud.API }

func (d apiDB) Create(table string, fields map[string]string) (string, error) {
	return d.api.Create(table, fields)
}
func (d apiDB) Get(table, id string) (map[string]string, bool, error) { return d.api.Get(table, id) }
func (d apiDB) Update(table, id string, fields map[string]string) error {
	return d.api.Update(table, id, fields)
}
func (d apiDB) SetField(table, id, field, value string) error {
	return d.api.SetField(table, id, field, value)
}
func (d apiDB) MarkDelete(table, id string) error { return d.api.MarkDelete(table, id) }
func (d apiDB) Delete(table, id string) error     { return d.api.Delete(table, id) }
func (d apiDB) Query(query string, args ...any) ([]map[string]string, error) {
	return d.api.Query(query, args...)
}
func (d apiDB) CommentAdd(table, id, text string) (string, error) {
	return d.api.CommentAdd(table, id, text)
}
func (d apiDB) CommentsFor(table, id string) ([]map[string]string, error) {
	return d.api.CommentsFor(table, id)
}
func (d apiDB) AttachURI(table, id, uri, desc string) (string, error) {
	return d.api.AttachURI(table, id, uri, desc)
}
func (d apiDB) AttachmentsFor(table, id string) ([]Attachment, error) {
	atts, err := d.api.AttachmentsFor(table, id)
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, len(atts))
	for i, att := range atts {
		out[i] = Attachment{ID: att.ID, Kind: att.Kind, Label: att.Label, Description: att.Description}
	}
	return out, nil
}

// SetTrigger installs the host's trigger on the API. Trigger's method set
// matches the core's, so it is assignable here.
func (d apiDB) SetTrigger(t Trigger) { d.api.Trigger = t }

// collect pulls the code: bindings out of the schemas, first-wins on conflicts,
// the same order the trigger wiring used to walk.
func collect(schemas ...*ddl.Schema) CodeSet {
	cs := CodeSet{
		App:   map[string]string{},
		Table: map[string]map[string]string{},
		Field: map[string]map[string]map[string]string{},
	}
	for _, sch := range schemas {
		for k, fn := range sch.AppCode {
			if cs.App[k] == "" {
				cs.App[k] = fn
			}
		}
		for _, tb := range sch.Tables {
			if cs.Table[tb.Name] == nil && len(tb.Code) > 0 {
				m := map[string]string{}
				for k, fn := range tb.Code {
					m[k] = fn
				}
				cs.Table[tb.Name] = m
			}
			for _, f := range tb.Fields {
				if len(f.Code) == 0 {
					continue
				}
				if cs.Field[tb.Name] == nil {
					cs.Field[tb.Name] = map[string]map[string]string{}
				}
				if cs.Field[tb.Name][f.Name] == nil {
					m := map[string]string{}
					for k, fn := range f.Code {
						m[k] = fn
					}
					cs.Field[tb.Name][f.Name] = m
				}
			}
		}
	}
	return cs
}

// Attach wires the DDL's code: hooks into the API through the registered host.
// With no host (open-source build) there are no triggers; it warns if the DDL
// defined hooks that will now be inert.
func Attach(api *crud.API, ddlPath, logDir string, schemas ...*ddl.Schema) ([]string, error) {
	code := collect(schemas...)
	if registered == nil {
		if !code.Empty() {
			return []string{"scripting is an enterprise feature; this build ignores the DDL code: hooks"}, nil
		}
		return nil, nil
	}
	return registered.Attach(apiDB{api}, ddl.ScriptPath(ddlPath), ddlPath, logDir, code)
}

// RunFile executes a script file against the API through the registered host.
func RunFile(api *crud.API, path string) error {
	if registered == nil {
		return fmt.Errorf("--script requires the enterprise build; the open-source build has no scripting host")
	}
	return registered.RunFile(apiDB{api}, path)
}
