// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Access-rule evaluation (design "Optional granular access model"). The
// grammar owns the semantics of its own lists; callers supply the current
// user's group-name set. Everything is open until a DDL populates a list.
package ddl

// Allows evaluates one rule: membership in any blacklisted group denies
// (deny wins), an empty whitelist means everyone, otherwise membership in
// any whitelisted group is required.
func (r AccessRule) Allows(groups map[string]bool) bool {
	for _, group := range r.Blacklist {
		if groups[group] {
			return false
		}
	}
	if len(r.Whitelist) == 0 {
		return true
	}
	for _, group := range r.Whitelist {
		if groups[group] {
			return true
		}
	}
	return false
}

// AllowedFor evaluates an operation ("read", "write", "delete") with the
// design's built-in overrides: owners always pass, admins pass read/write.
func (ac Access) AllowedFor(op string, groups map[string]bool) bool {
	if groups["owners"] {
		return true
	}
	if groups["admins"] && op != "delete" {
		return true
	}
	switch op {
	case "read":
		return ac.Read.Allows(groups)
	case "write":
		return ac.Write.Allows(groups)
	default:
		return ac.Delete.Allows(groups)
	}
}
