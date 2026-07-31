// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Opt-in per-table feature behavior (design "Automatic features that any
// table can opt-in to"): audit capture, comments, URI + copied-file
// attachments, and row-access grants. schema.Bootstrap creates the backing
// tables; this file is what they DO. Every write here rides the same
// log-first path as ordinary data, so features replicate like everything
// else. Row-level access ENFORCEMENT is the separate access-model item;
// grants are just data until then.
package crud

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/guid"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

// parentArg renders a row id for binding against a parent_id column. Those
// columns are ref-typed, so they hold the raw bytes; binding the text form
// would match nothing and report no error.
func parentArg(id string) ([]byte, error) {
	raw, err := guid.Decode(id)
	if err != nil {
		return nil, fmt.Errorf("crud: %w", err)
	}
	return raw, nil
}

// EnableFeatures tells the API which tables opted in to what, and carries
// the tables' access rules along (access.go enforces them). Pass the user
// schema first, then the built-ins - the first definition of a table wins,
// matching schema.Catalog.
func (a *API) EnableFeatures(schemas ...*ddl.Schema) {
	a.features = map[string]ddl.Features{}
	a.access = map[string]ddl.Access{}
	a.fieldAccess = map[string]map[string]ddl.Access{}
	for _, schema := range schemas {
		for _, table := range schema.Tables {
			if _, ok := a.features[table.Name]; ok {
				continue
			}
			a.features[table.Name] = table.Features
			a.access[table.Name] = table.Access
			fieldAcc := map[string]ddl.Access{}
			for _, field := range table.Fields {
				fieldAcc[field.Name] = field.Access
			}
			a.fieldAccess[table.Name] = fieldAcc
		}
	}
}

// FeatureSet reports a table's opt-ins (zero value when none or unknown).
func (a *API) FeatureSet(table string) ddl.Features { return a.features[table] }

func (a *API) requireFeature(table string, on bool, feat string) error {
	if !on {
		return fmt.Errorf("crud: table %q has not opted in to %s", table, feat)
	}
	return nil
}

// --- audit trail ---

// audit builds the audit_trail entries that ride the same commit as a data
// change, so the trail and the change land (and replicate) together. Old
// values come from the view BEFORE the change applies. A failed read degrades
// to a record without old values - the trail must never block the write.
func (a *API) audit(table, id, action string, changed map[string]string) []txlog.Entry {
	if a.features == nil || !a.features[table].AuditTrail {
		return nil
	}
	now := a.now()
	var vals string
	switch action {
	case "update":
		if old, ok, _ := a.Get(table, id); ok {
			oldVals := map[string]string{}
			for field, newVal := range changed {
				// design: only fields that HAD a value which changed or was cleared
				if oldVal := old[field]; oldVal != "" && oldVal != newVal {
					oldVals[field] = oldVal
				}
			}
			vals = encodeAuditValues(oldVals)
		}
		// design: one record accumulates a user's updates while the row stays
		// active - merge into the newest record when it is ours and an update.
		// The FIRST recorded old value per field stays (it is the oldest).
		if last := a.lastAudit(table, id); last != nil &&
			last["action"] == "update" && last["user_id"] == a.UserID {
			merged := mergeAuditValues(last["values"], vals)
			entries := []txlog.Entry{a.entry("audit_trail", last["id"], "date", "update", now)}
			if merged != last["values"] {
				entries = append(entries, a.entry("audit_trail", last["id"], "values", "update", merged))
			}
			return entries
		}
	case "delete":
		// the row is about to vanish from the view; snapshot what it held
		if old, ok, _ := a.Get(table, id); ok {
			snap := map[string]string{}
			for field, value := range old {
				switch field {
				case "id", "is_active", "is_deleted", "date_created":
				default:
					if value != "" {
						snap[field] = value
					}
				}
			}
			vals = encodeAuditValues(snap)
		}
	}
	audID := newID()
	entries := []txlog.Entry{
		a.entry("audit_trail", audID, "", "create", ""),
		a.entry("audit_trail", audID, "table_name", "update", table),
		a.entry("audit_trail", audID, "parent_id", "update", id),
		a.entry("audit_trail", audID, "user_id", "update", a.UserID),
		a.entry("audit_trail", audID, "date", "update", now),
		a.entry("audit_trail", audID, "action", "update", action),
	}
	if vals != "" {
		entries = append(entries, a.entry("audit_trail", audID, "values", "update", vals))
	}
	return entries
}

// lastAudit fetches the newest audit record for a row; nil when none/unreadable.
func (a *API) lastAudit(table, id string) map[string]string {
	parent, err := parentArg(id)
	if err != nil {
		return nil
	}
	rows, err := a.Query(`SELECT "id","action","user_id","values" FROM "audit_trail"
		WHERE "table_name"=? AND "parent_id"=? ORDER BY "date" DESC, "id" DESC LIMIT 1`, table, parent)
	if err != nil || len(rows) == 0 {
		return nil
	}
	return rows[0]
}

// encodeAuditValues renders the design's values format: field:"old value"
// pairs, space-separated, sorted, with any double quote doubled (CSV-style).
func encodeAuditValues(values map[string]string) string {
	parts := make([]string, 0, len(values))
	for _, field := range sortedKeys(values) {
		parts = append(parts, field+`:"`+strings.ReplaceAll(values[field], `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}

// parseAuditValues is the inverse; unparseable tails are dropped, not fatal.
func parseAuditValues(encoded string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(encoded); {
		colon := strings.Index(encoded[i:], `:"`)
		if colon < 0 {
			break
		}
		field := encoded[i : i+colon]
		i += colon + 2
		var val strings.Builder
		for {
			quote := strings.IndexByte(encoded[i:], '"')
			if quote < 0 {
				return out // torn value: keep what parsed so far
			}
			val.WriteString(encoded[i : i+quote])
			i += quote + 1
			if i < len(encoded) && encoded[i] == '"' { // doubled quote = literal
				val.WriteByte('"')
				i++
				continue
			}
			break
		}
		out[strings.TrimSpace(field)] = val.String()
		for i < len(encoded) && encoded[i] == ' ' {
			i++
		}
	}
	return out
}

// mergeAuditValues folds new old-values into an existing record, keeping the
// already-recorded (older) value when a field appears in both.
func mergeAuditValues(have, add string) string {
	haveVals, addVals := parseAuditValues(have), parseAuditValues(add)
	for field, value := range addVals {
		if _, ok := haveVals[field]; !ok {
			haveVals[field] = value
		}
	}
	return encodeAuditValues(haveVals)
}

// --- m:m links ---

// Link records a live many2many link (idempotent) and returns the link row id.
// Convention: the host row is side 1, the feature row side 2.
func (a *API) Link(table1, id1, table2, id2 string) (string, error) {
	parent1, err := parentArg(id1)
	if err != nil {
		return "", err
	}
	parent2, err := parentArg(id2)
	if err != nil {
		return "", err
	}
	rows, err := a.Query(`SELECT "id" FROM "many2many"
		WHERE "table_name_1"=? AND "parent_id_1"=? AND "table_name_2"=? AND "parent_id_2"=? AND "is_deleted"=0`,
		table1, parent1, table2, parent2)
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		return rows[0]["id"], nil
	}
	return a.Create("many2many", map[string]string{
		"table_name_1": table1, "parent_id_1": id1,
		"table_name_2": table2, "parent_id_2": id2,
	})
}

// Links returns the side-2 row ids linked to (table1, id1).
func (a *API) Links(table1, id1, table2 string) ([]string, error) {
	parent1, err := parentArg(id1)
	if err != nil {
		return nil, err
	}
	rows, err := a.Query(`SELECT "parent_id_2" FROM "many2many"
		WHERE "table_name_1"=? AND "parent_id_1"=? AND "table_name_2"=? AND "is_deleted"=0`,
		table1, parent1, table2)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row["parent_id_2"]
	}
	return ids, nil
}

// --- comments ---

// CommentAdd appends a comment to a row of an opted-in table.
func (a *API) CommentAdd(table, id, text string) (string, error) {
	if err := a.requireFeature(table, a.features[table].Comments, "comments"); err != nil {
		return "", err
	}
	return a.Create("comments", map[string]string{
		"table_name": table, "parent_id": id, "comment": text,
	})
}

// CommentsFor lists a row's live comments, oldest first. Reads are not gated.
func (a *API) CommentsFor(table, id string) ([]map[string]string, error) {
	parent, err := parentArg(id)
	if err != nil {
		return nil, err
	}
	return a.Query(`SELECT * FROM "comments"
		WHERE "table_name"=? AND "parent_id"=? AND "is_deleted"=0 ORDER BY "date_created"`, table, parent)
}

// --- attachments ---

// Attachment is one attached item of either kind, UI-ready.
type Attachment struct {
	ID          string
	Kind        string // "uri" or "copy"
	Label       string // the uri, or name.extension
	Description string
}

// AttachURI attaches a link-in-place URI (or local path) to a row.
func (a *API) AttachURI(table, id, uri, desc string) (string, error) {
	if err := a.requireFeature(table, a.features[table].URIAttachments, "uri_attachments"); err != nil {
		return "", err
	}
	attID, err := a.Create("attachments_uri", map[string]string{"uri": uri, "description": desc})
	if err != nil {
		return "", err
	}
	if _, err := a.Link(table, id, "attachments_uri", attID); err != nil {
		return "", err
	}
	return attID, nil
}

// AttachFile copies a file into the log's attachments/ subfolder (stored as
// <row id>, no extension, per the design) and attaches it to a row.
func (a *API) AttachFile(table, id, src, desc string) (string, error) {
	if err := a.requireFeature(table, a.features[table].LocalAttachments, "local_attachments"); err != nil {
		return "", err
	}
	base := filepath.Base(src)
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	attID, err := a.Create("attachments_copy", map[string]string{
		"name":        strings.TrimSuffix(base, filepath.Ext(base)),
		"og_name":     base,
		"extension":   ext,
		"description": desc,
	})
	if err != nil {
		return "", err
	}
	if err := copyFile(src, filepath.Join(a.AttachmentsDir(), attID)); err != nil {
		_ = a.Delete("attachments_copy", attID) // no orphan row; the log keeps the history
		return "", err
	}
	if _, err := a.Link(table, id, "attachments_copy", attID); err != nil {
		return "", err
	}
	return attID, nil
}

// AttachmentsFor lists a row's attachments of both kinds, link order.
func (a *API) AttachmentsFor(table, id string) ([]Attachment, error) {
	var out []Attachment
	for _, kind := range []struct{ table, kind string }{
		{"attachments_uri", "uri"}, {"attachments_copy", "copy"},
	} {
		ids, err := a.Links(table, id, kind.table)
		if err != nil {
			return nil, err
		}
		for _, attID := range ids {
			row, ok, err := a.Get(kind.table, attID)
			if err != nil {
				return nil, err
			}
			if !ok || row["is_deleted"] == "1" {
				continue
			}
			att := Attachment{ID: attID, Kind: kind.kind, Description: row["description"]}
			if kind.kind == "uri" {
				att.Label = row["uri"]
			} else {
				att.Label = row["name"]
				if row["extension"] != "" {
					att.Label += "." + row["extension"]
				}
			}
			out = append(out, att)
		}
	}
	return out, nil
}

// AttachmentsDir is where copied attachment files live: <logdir>/attachments,
// so they ride the same git sync as the tx-log.
func (a *API) AttachmentsDir() string { return filepath.Join(a.log.Dir(), "attachments") }

// AttachmentWorkingCopy copies a stored attachment to the user tmp dir as
// name_<copy date, local time>.extension (per the design) and returns the path.
func (a *API) AttachmentWorkingCopy(attID string) (string, error) {
	row, ok, err := a.Get("attachments_copy", attID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("crud: no attachment %q", attID)
	}
	name := row["name"] + "_" + time.Now().Format("20060102-150405")
	if row["extension"] != "" {
		name += "." + row["extension"]
	}
	dst := filepath.Join(os.TempDir(), name)
	if err := copyFile(filepath.Join(a.AttachmentsDir(), attID), dst); err != nil {
		return "", err
	}
	return dst, nil
}

// copyFile clones src to dst, creating dst's directory. Plain io.Copy already
// behaves like `cp --reflink=auto` here: on Linux the os package copies via
// copy_file_range(2), which reflinks on filesystems that can (btrfs, XFS) and
// falls back to a byte copy everywhere else.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// --- row access grants (enforcement = the access-model backlog item) ---

// GrantRowAccess links a group to a row's access_rows entry, creating the
// entry on first grant.
func (a *API) GrantRowAccess(table, id, groupID string) error {
	if err := a.requireFeature(table, a.features[table].RowLevelAccess, "row_level_access"); err != nil {
		return err
	}
	parent, err := parentArg(id)
	if err != nil {
		return err
	}
	rows, err := a.Query(`SELECT "id" FROM "access_rows"
		WHERE "table_name"=? AND "parent_id"=? AND "is_deleted"=0`, table, parent)
	if err != nil {
		return err
	}
	var accID string
	if len(rows) > 0 {
		accID = rows[0]["id"]
	} else if accID, err = a.Create("access_rows", map[string]string{
		"table_name": table, "parent_id": id,
	}); err != nil {
		return err
	}
	_, err = a.Link("access_rows", accID, "groups", groupID)
	return err
}

// RowAccessGroups returns the group ids granted to a row (empty = no grants).
func (a *API) RowAccessGroups(table, id string) ([]string, error) {
	parent, err := parentArg(id)
	if err != nil {
		return nil, err
	}
	rows, err := a.Query(`SELECT "id" FROM "access_rows"
		WHERE "table_name"=? AND "parent_id"=? AND "is_deleted"=0`, table, parent)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return a.Links("access_rows", rows[0]["id"], "groups")
}
