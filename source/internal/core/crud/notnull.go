// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package crud

import (
	"fmt"
	"sort"
	"strings"
)

// null_ok: no is checked here rather than as NOT NULL in the view, which made
// replay drop the row (backlog 2026102407331876). Replay never re-checks.

// requireOnCreate refuses a create that would leave a null_ok: no field null.
// A static default counts, since the view fills it in.
func (a *API) requireOnCreate(table string, fields map[string]string) error {
	var missing []string
	for field, hasDefault := range a.notNull[table] {
		if _, given := fields[field]; !given && !hasDefault {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("crud: %s: %s can't be null (null_ok: no)", table, strings.Join(missing, ", "))
}

// requireNullable refuses an explicit null on a null_ok: no field, default or not.
func (a *API) requireNullable(table, field string) error {
	if _, strict := a.notNull[table][field]; strict {
		return fmt.Errorf("crud: %s: %s can't be null (null_ok: no)", table, field)
	}
	return nil
}
