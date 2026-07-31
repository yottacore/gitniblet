// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// SQL helpers for hand-written queries. Ids and row references are BLOBs in
// the view but text everywhere a person sees them, and SQLite compares a BLOB
// against a text literal as simply unequal - no error, just an empty result.
// These two functions are how a raw query crosses that line on purpose:
//
//	SELECT * FROM comments WHERE parent_id = id('AZeR9kQ_c0-Bq2VvKQpXhw')
//	SELECT idtext(id), title FROM task
//
// A malformed id raises an error rather than matching nothing, so a typo in a
// query says so instead of looking like "no rows".
package store

import (
	"database/sql/driver"
	"fmt"

	sqlite "modernc.org/sqlite"

	"github.com/jim-collier/nano-git-db/internal/core/guid"
)

// Registration is driver-global and only reaches connections opened later, so
// it happens at package init - every store.Open then has both functions.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("id", 1, idFunc)
	sqlite.MustRegisterDeterministicScalarFunction("idtext", 1, idTextFunc)
}

// idFunc turns an id's text form into the raw bytes a key or ref column holds.
func idFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	switch arg := args[0].(type) {
	case nil:
		return nil, nil
	case []byte:
		return arg, nil // already raw; makes id() safe to apply twice
	case string:
		return guid.Decode(arg)
	default:
		return nil, fmt.Errorf("id(): want an id string, got %T", arg)
	}
}

// idTextFunc is the inverse, for comparing or displaying an id as text.
func idTextFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	switch arg := args[0].(type) {
	case nil:
		return nil, nil
	case []byte:
		return guid.Encode(arg), nil
	case string:
		return arg, nil
	default:
		return nil, fmt.Errorf("idtext(): want an id, got %T", arg)
	}
}
