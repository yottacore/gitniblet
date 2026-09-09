// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package schema owns the built-in tables: users, groups, and the opt-in
// feature tables (many2many, comments, audit trail, row access, attachments).
// They are written in the same DDL end-users write, embedded in this one file
// so developers can tweak them but end-users cannot - it ships compiled in.
//
// Bootstrap runs at startup after the user schema is built: it creates the
// always-on tables plus whichever feature tables any table's features: block
// asks for (CREATE IF NOT EXISTS + additive migration, so re-running every
// startup is the verification the design calls for). SeedDefaults then fills
// the default groups rows, log-first, once the tx-log has been applied.
package schema

import (
	"fmt"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/store"
)

const builtinsDDL = `
tables:
	table: users
		fields:
			field: screen_name  ## defaults to the github name at the app layer
				type: string
			field: github_name
				type: string
			field: email
				type: string
			field: phone
				type: string
			field: avatar
				type: binary
		features:
			local_attachments: y
			uri_attachments: y
			comments: y
			audit_trail: y
	table: groups
		fields:
			field: name
				type: string
			field: description
				type: string
		uniques:  ## unique-on-live-name is what makes concurrent seeding converge
			unique: name
		features:
			local_attachments: y
			uri_attachments: y
			comments: y
			audit_trail: y
	table: many2many  ## one table stores every m:m link
		fields:
			field: table_name_1
				type: string
			field: table_name_2
				type: string
			field: parent_id_1  ## row id in table_name_1
				type: ref
			field: parent_id_2  ## row id in table_name_2
				type: ref
		uniques:  ## partial (live rows only), so is_deleted needs no folding in
			unique: table_name_1, table_name_2, parent_id_1, parent_id_2
	table: comments
		fields:
			field: table_name
				type: string
			field: parent_id  ## row id in table_name
				type: ref
			field: comment
				type: string
		indexes:
			index: table_name, parent_id
	table: audit_trail  ## design: gets ONLY these fields, no system columns
		system_fields: no
		fields:
			field: table_name
				type: string
			field: parent_id  ## row id in table_name
				type: ref
			field: user_id
				type: string
			field: date  ## stored GMT, displayed local
				type: datetime_utc
			field: action  ## create, update, mark_delete, delete
				type: string
			field: values  ## old values of only the fields that changed
				type: string
		indexes:
			index: table_name, parent_id
	table: access_rows
		fields:
			field: table_name
				type: string
			field: parent_id  ## row id in table_name
				type: ref
		indexes:
			index: table_name, parent_id
	table: attachments_uri
		fields:
			field: uri
				type: string
			field: description
				type: string
	table: attachments_copy
		fields:
			field: name  ## doc name, defaults to the original base name
				type: string
			field: description
				type: string
			field: og_name  ## full original filename
				type: string
			field: extension  ## original extension; stored file is named <id>, no extension
				type: string

relationships:
	relationship: users_groups  ## implied by design: every user belongs to groups
		type: m:m
		parent: users
		child: groups
		cascade_delete: y
		enable_audit_trail: y
	relationship: access_rows_groups  ## implied by design: row grants are held by groups
		type: m:m
		parent: access_rows
		child: groups
		cascade_delete: y
		enable_audit_trail: y
`

// alwaysOn are created unconditionally: users/groups per the design, and
// many2many because the implied users<->groups m:m lives there.
var alwaysOn = []string{"users", "groups", "many2many"}

// Builtins parses the embedded DDL. It is small, so no caching.
func Builtins() (*ddl.Schema, error) {
	s, err := ddl.Parse([]byte(builtinsDDL))
	if err != nil {
		return nil, fmt.Errorf("builtins DDL: %w", err)
	}
	return s, nil
}

// featureTables marks the feature tables one table's features: block pulls in.
// Attachments and row access ride on m:m links, so they drag many2many along.
func featureTables(features ddl.Features, need map[string]bool) {
	if features.Comments {
		need["comments"] = true
	}
	if features.AuditTrail {
		need["audit_trail"] = true
	}
	if features.LocalAttachments {
		need["attachments_copy"] = true
		need["many2many"] = true
	}
	if features.URIAttachments {
		need["attachments_uri"] = true
		need["many2many"] = true
	}
	if features.RowLevelAccess {
		need["access_rows"] = true
		need["many2many"] = true
	}
}

// Bootstrap ensures the built-in tables exist in the view: the always-on set,
// plus every feature table something opts in to (the built-ins' own features
// count, so comments/audit/attachments always exist in practice; access_rows
// only appears when a table asks for row_level_access). Idempotent - safe to
// run every startup. user may be nil. Returned warnings are non-fatal.
func Bootstrap(st *store.Store, user *ddl.Schema) ([]string, error) {
	builtins, err := Builtins()
	if err != nil {
		return nil, err
	}
	need := map[string]bool{}
	for _, name := range alwaysOn {
		need[name] = true
	}
	for _, table := range builtins.Tables {
		if table.Name == "users" || table.Name == "groups" {
			featureTables(table.Features, need)
		}
	}
	userNames := map[string]bool{}
	if user != nil {
		for _, table := range user.Tables {
			userNames[table.Name] = true
			featureTables(table.Features, need)
		}
	}
	var warns []string
	toBuild := &ddl.Schema{}
	for _, table := range builtins.Tables {
		if !need[table.Name] {
			continue
		}
		if userNames[table.Name] {
			// The user schema already built its own shape; additively merging
			// two definitions of one table would leave neither intact.
			warns = append(warns, fmt.Sprintf(
				"table %q is a built-in name; keeping the user DDL definition, skipping the built-in one", table.Name))
			continue
		}
		toBuild.Tables = append(toBuild.Tables, table)
	}
	return warns, st.Build(toBuild)
}

// defaultGroups are the rows populated on db creation, per the design.
var defaultGroups = []struct{ Name, Desc string }{
	{"owners", "Full control. The first user is a member."},
	{"admins", "Can administer users and data. The first user is a member."},
	{"users", "Regular users."},
	{"guests", "Heavily restricted, mostly read-only."},
}

// SeedDefaults creates the default groups rows when missing. It goes through
// CRUD so the seed goes into the tx-log truth and replicates; call it after the
// log has been applied to the view, or an already-seeded log seeds twice.
// Two clients seeding concurrently still converge: the union-merged log replays
// both creates and the partial unique on name skips the loser.
// First-user membership in owners/admins is deferred until users exist.
func SeedDefaults(api *crud.API) error {
	for _, group := range defaultGroups {
		rows, err := api.Query(`SELECT "id" FROM "groups" WHERE "name"=? AND "is_deleted"=0`, group.Name)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			continue
		}
		if _, err := api.Create("groups", map[string]string{
			"name":        group.Name,
			"description": group.Desc,
		}); err != nil {
			return err
		}
	}
	return nil
}
