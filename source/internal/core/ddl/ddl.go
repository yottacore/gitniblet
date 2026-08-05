// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package ddl parses the nano-git-db DDL into an in-memory schema model.
//
// The DDL is an SHCL document (github.com/jim-collier/shcl), so the language
// itself - indentation, quoting, comments, merging of repeated sections - is
// shcl's job, not this package's. What is left here is two things shcl cannot
// know: the schema file in schema.shcl, which states the DDL's vocabulary and
// value domains for shcl's validator, and the mappers below, which walk a
// validated document into the typed Schema.
//
// Warnings come from two sources. Anything about a single line - an unknown key,
// a value outside its allowed set, a bad number - is an shcl diagnostic. Anything
// cross-cutting - a table defined twice, a unique naming a field that does not
// exist - is checked here. Both cite a line, so they read alike.
//
// Most attributes are optional, so scalars that carry an unset/default
// distinction use pointers (nil = "use the default").
package ddl

import (
	"fmt"
	"os"
	"strings"

	shcl "github.com/jim-collier/shcl/source/go"
)

// Schema is a parsed DDL file.
type Schema struct {
	AppCode     map[string]string // before_open, after_open, ... (DDL `code:`)
	Tunables    map[string]string // tunables: section, raw values (see TunableInt)
	Tables      []Table
	Relations   []Relationship
	Views       []View
	DefaultView string   // ui: default_view; empty means "first view defined"
	Encryption  string   // database-level encryption: always|never|auto ("" = unset)
	Warnings    []string // soft issues; parsing still succeeds
	// Errors counts the error-severity diagnostics behind Warnings: lines that
	// were skipped or repaired, and failed validation. Parsing still produced a
	// schema, but something in the file did not survive intact - discovery uses
	// this to flag a record rather than opening a silently incomplete schema.
	Errors int
}

// HasErrors reports whether the load lost or repaired anything.
func (s *Schema) HasErrors() bool { return s.Errors > 0 }

// AccessRule is one allow/deny pair. Empty whitelist means "all".
type AccessRule struct {
	Whitelist []string
	Blacklist []string
}

// Access is table/field-level access split by operation.
type Access struct {
	Read   AccessRule
	Write  AccessRule
	Delete AccessRule
}

// Features are the opt-in per-table capabilities.
type Features struct {
	LocalAttachments bool
	URIAttachments   bool
	Comments         bool
	AuditTrail       bool
	RowLevelAccess   bool
}

// Table is one DDL table.
type Table struct {
	Name       string
	Aliases    []string
	Access     Access
	Fields     []Field
	Code       map[string]string // before_update, after_update
	Uniques    [][]string        // each group of field names (auto-named, indexed)
	Indexes    [][]string        // each index
	Features   Features
	Encryption string // table-level encryption: always|never|auto ("" = unset)
	// SystemUI holds presentation hints for the auto-added columns. A DDL entry
	// naming a system field is legal but presentation-only: the column itself is
	// managed, so only its ui: block is kept (anything else warns and is
	// ignored). Keeping these out of Fields is what stops the view builder from
	// emitting a second column for a name it already manages.
	SystemUI map[string]FieldUI
	// NoSystemFields (DDL `system_fields: no`) drops the auto-added
	// is_active/date_created/is_deleted columns; `id` is always managed.
	// Used by the built-in audit_trail table, which defines exactly its own
	// fields per the design.
	NoSystemFields bool
}

// Validation holds a field's validation rules.
type Validation struct {
	Required *bool
	MinLen   *int
	MaxLen   *int
	MinVal   *float64
	MaxVal   *float64
	Regex    string
	Method   string // script function name, raw
}

// FieldUI holds a field's presentation hints.
type FieldUI struct {
	VisibleForm  *bool
	VisibleList  *bool
	Title        string
	Desc         string
	Order        *float64
	Readonly     *bool
	Width        *int
	Widget       string
	ListType     string
	ListSource   string // raw: literals, backtick SQL, or a lookups idx
	Format       string
	LookupInList string // symbol|title|both
	LookupInForm string // symbol|title|both
}

// Field is one column.
type Field struct {
	Name       string
	Aliases    []string
	Access     Access
	Type       string
	Encryption string // field-level encryption: always|never|auto ("" = unset)
	Special    string
	IsActive   *bool
	Default    string // raw: a sentinel, a func() ref, or a literal
	NullOK     *bool
	EmptyOK    *bool
	Validation Validation
	Code       map[string]string
	UI         FieldUI
}

// Relationship is a 1:m or m:m link.
type Relationship struct {
	Type             string // 1:m or m:m
	Parent           string
	Child            string
	ParentIDField    string
	CascadeDelete    bool
	EnableAuditTrail bool
}

// Block is one (possibly nested) view layout block.
type Block struct {
	Name        string
	Table       string
	Type        string // form|grid|tree_grid|comments
	ParentField string // tree_grid: same-table field naming the parent row
	Location    []string
	Readonly    *bool
	Children    []Block
}

// View is a UI view over tables.
type View struct {
	Name              string
	Aliases           []string
	StartupNamedQuery string
	Readonly          *bool
	Access            AccessRule // views use a flat rule, not read/write/delete
	Layout            []Block
}

// Parse turns DDL source into a Schema.
//
// The load is Loose on purpose, and not just as a shrug: SHCL's Loose bundle is
// almost exactly the value tolerance this DDL already documented - y/n and
// enable/disable as booleans, a leading currency symbol on a number, a
// fractional value where an integer is wanted. The DDL is aimed at users with no
// SQL background, so the parser does the work.
//
// Loose still reports everything it repairs, and (like Standard) never fails a
// load over one bad line: the rest of the file still opens, so a typo costs a
// line rather than the whole schema. Nothing passes silently.
func Parse(src []byte) (*Schema, error) {
	doc, _ := shcl.ParseWith(string(src), shcl.Loose) // only Strict can fail a load
	s := &Schema{AppCode: map[string]string{}, Tunables: map[string]string{}}
	s.addDiagnostics(doc.Diagnostics())
	s.addDiagnostics(doc.Validate(schemaDoc()))

	root := cursor{doc: doc}
	s.mapCode(root)
	s.mapTunables(root)
	s.mapEncryption(root)
	s.mapTables(root)
	s.mapRelationships(root)
	s.mapViews(root)
	return s, nil
}

// ParseFile reads and parses a DDL file.
func ParseFile(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// addDiagnostics folds shcl's line-numbered diagnostics into the schema's
// warnings. Hints are carried through too: a repeated bare leaf that looks like
// an array, or a section that silently combined with an earlier one, are
// exactly the near-misses this DDL's audience needs told about.
//
// SuppressDeclaredRepeats drops the repeated-leaf hint for keys the schema
// declares as repeating - `unique:`, `index:`, `row:` - where recurring IS the
// design and the hint would fire on every correct DDL.
// The merged-with-an-earlier-section hint is deliberately NOT filtered, even
// though restating `database:` is legal and merges cleanly. When a wrapper and
// an entity inside it both merge, only the outermost is reported - so silencing
// wrappers would also silence two far-apart `table: t` sections quietly becoming
// one table, which is the thing worth catching. A schema laid out in one pass
// never sees it; project/example.shcl is written that way for the same reason.
func (s *Schema) addDiagnostics(diags []shcl.Diagnostic) {
	for _, d := range shcl.SuppressDeclaredRepeats(schemaDoc(), diags) {
		if isSchemaFault(d) {
			// A fault's line is a line of the built-in vocabulary, not of the
			// DDL in front of the user, so it must not read like one. Only a
			// build that shipped a broken schema.shcl can produce these, and
			// the surviving constraints still check the document around them.
			s.Warnings = append(s.Warnings, "built-in schema fault: "+d.Message)
			s.Errors++
			continue
		}
		s.Warnings = append(s.Warnings, fmt.Sprintf("line %d: %s", d.Line, d.Message))
		if d.Severity == shcl.SeverityError {
			s.Errors++
		}
	}
}

// isSchemaFault tells a complaint about the validation schema (V09x) from one
// about the document being loaded. They share a diagnostics list but not a line
// numbering.
func isSchemaFault(d shcl.Diagnostic) bool { return strings.HasPrefix(d.Code, "V09") }

// warn records a cross-cutting problem: one about the relationship between
// lines rather than about a single line. It still cites the entity's own line
// where one is known, so it reads like every other diagnostic.
func (s *Schema) warn(format string, args ...any) {
	s.Warnings = append(s.Warnings, fmt.Sprintf(format, args...))
}

// errAt is warnAt for something the load could not repair, so callers that ask
// HasErrors() report the database unopenable rather than letting it fail later
// with a raw SQLite error.
func (s *Schema) errAt(c cursor, format string, args ...any) {
	s.warnAt(c, format, args...)
	s.Errors++
}

// warnAt is warn with a line number, for the cases holding the offending
// entity's cursor. Line 0 means shcl could not place it; the message still goes
// out, just without the prefix.
func (s *Schema) warnAt(c cursor, format string, args ...any) {
	if line := c.line(); line > 0 {
		s.warn("line %d: %s", line, fmt.Sprintf(format, args...))
		return
	}
	s.warn(format, args...)
}

// sectionPrefixes are the paths a logical section may sit at. `database:` and
// `ui:` are organizational wrappers, so a DDL may nest a section under one or
// leave it at the top level; both spell the same thing. shcl merges repeated
// wrappers itself, so each prefix resolves at most once.
func sectionPrefixes(wrapper string) []string { return []string{wrapper, ""} }

// mapCode reads the application-level `code:` hooks.
func (s *Schema) mapCode(root cursor) {
	for name, fn := range root.codeMap("code") {
		s.AppCode[name] = fn
	}
}

// mapEncryption reads the database-level encryption directive.
func (s *Schema) mapEncryption(root cursor) {
	for _, prefix := range sectionPrefixes("database") {
		at := cursor{doc: root.doc, path: prefix}
		if !at.exists("encryption") {
			continue
		}
		if value := at.lower("encryption"); encValues[value] {
			s.Encryption = value
			return
		}
	}
}

// mapTunables reads the tunables: section. The known keys are validated by the
// schema file; an unknown one still stores, so a tunable written by a newer
// client survives a round trip through an older one.
func (s *Schema) mapTunables(root cursor) {
	at := root.child("tunables")
	for _, key := range at.children("") {
		if _, dup := s.Tunables[key]; dup {
			continue // first wins, like every other merge
		}
		s.Tunables[key] = at.str(shcl.QuoteSegment(key))
	}
}

// mapTables reads every table, from either section prefix.
func (s *Schema) mapTables(root cursor) {
	for _, prefix := range sectionPrefixes("database") {
		at := cursor{doc: root.doc, path: prefix}.child("tables")
		names := at.instances("table")
		for i, name := range names {
			tc := at.instance("table", i)
			table := s.parseTable(tc, name)
			if table.Name == "" {
				s.warnAt(tc, "a table with no name was dropped")
				continue
			}
			if s.table(table.Name) != nil {
				s.warnAt(tc, "table %q is defined more than once; the first definition wins", table.Name)
				continue
			}
			s.Tables = append(s.Tables, table)
		}
	}
}

// mapRelationships reads every relationship, from either section prefix.
func (s *Schema) mapRelationships(root cursor) {
	for _, prefix := range sectionPrefixes("database") {
		at := cursor{doc: root.doc, path: prefix}.child("relationships")
		for i := 0; i < at.count("relationship"); i++ {
			s.Relations = append(s.Relations, parseRelationship(at.instance("relationship", i)))
		}
	}
}

// mapViews reads every view and the default-view pointer.
func (s *Schema) mapViews(root cursor) {
	for _, prefix := range sectionPrefixes("ui") {
		at := cursor{doc: root.doc, path: prefix}
		if s.DefaultView == "" {
			s.DefaultView = at.str("default_view")
		}
		views := at.child("views")
		for i, name := range views.instances("view") {
			s.Views = append(s.Views, parseView(views.instance("view", i), name))
		}
	}
}

// encValues is the closed encryption vocabulary. always and never lock lower
// levels; auto (the default) defers up or down. A value outside the set is
// reported by schema validation and read here as unset.
var encValues = map[string]bool{"": true, "always": true, "never": true, "auto": true}

// encOf reads an encryption: child, treating anything outside the set as unset.
func encOf(c cursor) string {
	if value := c.lower("encryption"); encValues[value] {
		return value
	}
	return ""
}

func (s *Schema) parseTable(c cursor, name string) Table {
	t := Table{Name: name, Code: c.codeMap("code")}
	t.Aliases = c.list("aliases")
	t.Encryption = encOf(c)
	// system_fields defaults on; only an explicit false opts out.
	t.NoSystemFields = !c.boolOr("system_fields", true)
	if c.exists("access") {
		t.Access = parseAccessRWD(c.child("access"))
	}

	fields := c.child("fields")
	for i, fieldName := range fields.instances("field") {
		fc := fields.instance("field", i)
		field := parseField(fc, fieldName)
		if field.Name == "" {
			s.warnAt(fc, "a field with no name in table %q was dropped", t.Name)
			continue
		}
		// A system field's column is managed, so an entry naming one contributes
		// presentation only - it never becomes a second column.
		if isSystemField(field.Name, t.NoSystemFields) {
			if t.SystemUI == nil {
				t.SystemUI = map[string]FieldUI{}
			}
			t.SystemUI[field.Name] = field.UI
			if definesColumn(field) {
				s.warnAt(fc, "field %q in table %q is a system field: its ui: applies, the rest is ignored", field.Name, t.Name)
			}
			continue
		}
		if t.hasField(field.Name) {
			s.warnAt(fc, "field %q in table %q is defined more than once; the first definition wins", field.Name, t.Name)
			continue
		}
		t.Fields = append(t.Fields, field)
	}

	t.Uniques = s.parseKeyGroups(c, &t, "uniques", "unique")
	t.Indexes = s.parseKeyGroups(c, &t, "indexes", "index")

	if c.exists("features") {
		features := c.child("features")
		t.Features = Features{
			LocalAttachments: features.boolOr("local_attachments", false),
			URIAttachments:   features.boolOr("uri_attachments", false),
			Comments:         features.boolOr("comments", false),
			AuditTrail:       features.boolOr("audit_trail", false),
			RowLevelAccess:   features.boolOr("row_level_access", false),
		}
	}
	return t
}

// parseKeyGroups reads a uniques:/indexes: section. Each group is one instance
// of the singular name carrying a comma list of field names, so a table can
// declare several groups (`unique: a, b` then `unique: c, d`).
func (s *Schema) parseKeyGroups(c cursor, t *Table, section, entry string) [][]string {
	if !c.exists(section) {
		return nil
	}
	at := c.child(section)
	var out [][]string
	for i := 0; i < at.count(entry); i++ {
		gc := at.instance(entry, i)
		group := gc.list("")
		if len(group) == 0 {
			continue
		}
		// A name matching no field would reach SQLite verbatim and take the
		// whole open down with a raw error, from a schema the picker had just
		// listed as fine. Counting it as an error is what greys the database
		// out with a reason instead.
		//
		// The group is dropped whole rather than one name at a time: pruning
		// the bad name from `unique: a, b` leaves `unique: a`, a stricter rule
		// the schema never asked for, which would then refuse rows that are
		// perfectly legal.
		bad := false
		for _, name := range group {
			if !t.hasField(name) {
				s.errAt(gc, "%s on table %q names unknown field %q; the group is dropped",
					section, t.Name, name)
				bad = true
			}
		}
		if bad {
			continue
		}
		out = append(out, group)
	}
	return out
}

// The closed set of field types lives in schema.shcl as the `allowed:` list on
// a field's type, so an unrecognized one is reported there with its line number.
// It still stores as written rather than failing the load.

func parseField(c cursor, name string) Field {
	f := Field{Name: name, Code: c.codeMap("code")}
	f.Aliases = c.list("aliases")
	if c.exists("access") {
		f.Access = parseAccessRWD(c.child("access"))
	}
	f.Type = c.lower("type")
	f.Encryption = encOf(c)
	f.Special = c.str("special")
	f.IsActive = c.boolPtr("is_active")
	f.Default = c.raw("defaultval")

	// null_ok/empty_ok belong under validation:, but the example DDL has carried
	// them at field level too; read either place, field level first.
	validation := c.child("validation")
	f.NullOK = firstBool(c, validation, "null_ok")
	f.EmptyOK = firstBool(c, validation, "empty_ok")
	if c.exists("validation") {
		f.Validation = Validation{
			Required: validation.boolPtr("required"),
			MinLen:   validation.intPtr("minlen"),
			MaxLen:   validation.intPtr("maxlen"),
			MinVal:   validation.floatPtr("minval"),
			MaxVal:   validation.floatPtr("maxval"),
			Regex:    validation.raw("regex"),
			Method:   validation.raw("method"),
		}
		// defaultval also appears under validation: in the example DDL.
		if f.Default == "" {
			f.Default = validation.raw("defaultval")
		}
	}
	if c.exists("ui") {
		ui := c.child("ui")
		f.UI = FieldUI{
			VisibleForm: ui.boolPtr("visible_form"),
			VisibleList: ui.boolPtr("visible_list"),
			// label: is the documented key; title: was the earlier spelling.
			Title:        ui.firstStr("label", "title"),
			Desc:         ui.str("description"),
			Order:        ui.floatPtr("order"),
			Readonly:     ui.boolPtr("readonly"),
			Width:        ui.intPtr("width"),
			Widget:       ui.str("widget"),
			ListType:     ui.str("list_type"),
			ListSource:   ui.raw("list_source"),
			Format:       ui.str("format"),
			LookupInList: ui.lower("lookup_in_lists"),
			LookupInForm: ui.lower("lookup_in_forms"),
		}
	}
	return f
}

// firstBool reads a bool from the first of two cursors that carries it, for keys
// accepted at more than one level.
func firstBool(first, second cursor, rel string) *bool {
	if val := first.boolPtr(rel); val != nil {
		return val
	}
	return second.boolPtr(rel)
}

func parseRelationship(c cursor) Relationship {
	return Relationship{
		Type:             c.lower("type"),
		Parent:           c.str("parent"),
		Child:            c.str("child"),
		ParentIDField:    c.str("parent_id_field"),
		CascadeDelete:    c.boolOr("cascade_delete", false),
		EnableAuditTrail: c.boolOr("enable_audit_trail", false),
	}
}

func parseView(c cursor, name string) View {
	v := View{Name: name}
	v.Aliases = c.list("aliases")
	// Renamed from default_named_query 2026-07; keep reading the old key so
	// existing DDLs parse. It must compare equal to a named query's name.
	v.StartupNamedQuery = c.str("startup_named_query")
	if v.StartupNamedQuery == "" {
		v.StartupNamedQuery = c.str("default_named_query")
	}
	v.Readonly = c.boolPtr("readonly")
	if c.exists("access") {
		v.Access = accessFlat(c.child("access"))
	}
	if c.exists("layout") {
		v.Layout = parseBlocks(c.child("layout"))
	}
	return v
}

// parseBlocks reads a layout's block instances, recursing into nested ones.
func parseBlocks(c cursor) []Block {
	var out []Block
	for i, name := range c.instances("block") {
		at := c.instance("block", i)
		out = append(out, Block{
			Name:        name,
			Table:       at.str("table"),
			Type:        at.lower("type"),
			ParentField: at.str("parent_field"),
			Location:    at.list("location"),
			Readonly:    at.boolPtr("readonly"),
			Children:    parseBlocks(at),
		})
	}
	return out
}

func parseAccessRWD(c cursor) Access {
	return Access{
		Read:   accessFlat(c.child("read")),
		Write:  accessFlat(c.child("write")),
		Delete: accessFlat(c.child("delete")),
	}
}

// accessFlat reads whitelist/blacklist from a cursor. Early DDLs (including the
// original example) misspelled it "blaclist"; keep accepting both so they parse.
func accessFlat(c cursor) AccessRule {
	return AccessRule{
		Whitelist: c.list("whitelist"),
		Blacklist: c.firstList("blacklist", "blaclist"),
	}
}

// TunableInt reads a tunable as an integer, falling back on absent or
// non-numeric values.
func (s *Schema) TunableInt(key string, def int) int {
	value, ok := s.Tunables[key]
	if !ok {
		return def
	}
	num, ok := AsInt(value)
	if !ok {
		return def
	}
	return num
}

// table finds an already-parsed table by name.
func (s *Schema) table(name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

func (t *Table) field(name string) *Field {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// HasAlwaysEncryption reports whether any level declares encryption: always -
// i.e. writing needs a key. Used to warn when a key is missing.
func (s *Schema) HasAlwaysEncryption() bool {
	if s.Encryption == "always" {
		return true
	}
	for _, t := range s.Tables {
		if t.Encryption == "always" {
			return true
		}
		for _, f := range t.Fields {
			if f.Encryption == "always" {
				return true
			}
		}
	}
	return false
}

// EncryptionPolicy resolves a field's encryption directive, scanning
// database -> table -> field outermost first. The first always|never locks the
// result - a lower level cannot override it (only a higher one can). If every
// level is auto or unset the result is "auto": no schema lock, so the caller's
// local preference and key presence decide. Unknown tables/fields inherit the
// database (then "auto") level.
func (s *Schema) EncryptionPolicy(table, field string) string {
	levels := []string{s.Encryption}
	if t := s.table(table); t != nil {
		levels = append(levels, t.Encryption)
		if f := t.field(field); f != nil {
			levels = append(levels, f.Encryption)
		}
	}
	for _, level := range levels {
		if level == "always" || level == "never" {
			return level
		}
	}
	return "auto"
}

// definesColumn reports whether a field entry says anything about the column
// itself, as opposed to only how it is presented. Used to tell a presentation-only
// system-field entry (silent, as designed) from one trying to redefine a managed
// column (warned and ignored).
func definesColumn(f Field) bool {
	return f.Type != "" || f.Default != "" || f.Special != "" || f.Encryption != "" ||
		f.NullOK != nil || f.EmptyOK != nil || len(f.Aliases) > 0 ||
		f.Validation != Validation{}
}

// isSystemField reports whether a name is one of the auto-added columns.
func isSystemField(name string, noSystemFields bool) bool {
	if name == "id" {
		return true // id is managed on every table, opt-out or not
	}
	switch name {
	case "is_active", "date_created", "is_deleted":
		return !noSystemFields
	}
	return false
}

func (t *Table) hasField(name string) bool {
	if isSystemField(name, t.NoSystemFields) {
		return true
	}
	for _, f := range t.Fields {
		if f.Name == name {
			return true
		}
	}
	return false
}
