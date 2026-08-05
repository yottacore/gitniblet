// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Trigger hook points (design "code:" DDL keys). The core stays
// script-agnostic: it fires these around every write and the scripting host
// plugs in the sidecar-backed implementation. Order per the design: field-level
// before_update runs first (it may replace the value), then table-level
// (it may veto); after hooks mirror that once the commit is written.
package crud

// Trigger receives write events. id is "" on create (not yet issued when the
// before hooks run).
type Trigger interface {
	// BeforeField validates and/or replaces one incoming value.
	BeforeField(table, field, value string) (string, error)
	// BeforeUpdate may veto the whole write. fields are the final values.
	BeforeUpdate(table, id string, fields map[string]string) error
	AfterField(table, id, field, value string)
	AfterUpdate(table, id string)
}

// runBefore fires the before hooks over a write's fields, returning the
// (possibly replaced) values. The caller's map is never mutated.
func (a *API) runBefore(table, id string, fields map[string]string) (map[string]string, error) {
	if a.Trigger == nil || len(fields) == 0 {
		return fields, nil
	}
	out := make(map[string]string, len(fields))
	for _, field := range sortedKeys(fields) {
		value, err := a.Trigger.BeforeField(table, field, fields[field])
		if err != nil {
			return nil, err
		}
		out[field] = value
	}
	if err := a.Trigger.BeforeUpdate(table, id, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) runAfter(table, id string, fields map[string]string) {
	if a.Trigger == nil {
		return
	}
	for _, field := range sortedKeys(fields) {
		a.Trigger.AfterField(table, id, field, fields[field])
	}
	a.Trigger.AfterUpdate(table, id)
}
