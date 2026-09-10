// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package ddl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	shcl "github.com/jim-collier/shcl/source/go"
)

func TestValueScalars(t *testing.T) {
	if !IsNull("NULL") || IsNull("null") {
		t.Fatal("IsNull")
	}
	for _, s := range []string{"true", "1", "Y", "yes", "enabled", "T"} {
		if b, ok := AsBool(s); !ok || !b {
			t.Fatalf("AsBool(%q) want true", s)
		}
	}
	for _, s := range []string{"false", "0", "n", "no", "disabled", "F"} {
		if b, ok := AsBool(s); !ok || b {
			t.Fatalf("AsBool(%q) want false", s)
		}
	}
	if _, ok := AsBool("bool"); ok {
		t.Fatal("AsBool(bool) should not be a boolean")
	}
	nums := map[string]float64{"1,000": 1000, ".1": 0.1, "1.": 1, "0.1": 0.1, "3.005": 3.005, "-2.5": -2.5}
	for s, want := range nums {
		if f, ok := AsFloat(s); !ok || f != want {
			t.Fatalf("AsFloat(%q) = %v,%v want %v", s, f, ok, want)
		}
	}
	if _, ok := AsInt("3.005"); ok {
		t.Fatal("AsInt(3.005) should fail")
	}
	if i, ok := AsInt("1,000"); !ok || i != 1000 {
		t.Fatalf("AsInt(1,000) = %v,%v", i, ok)
	}
	if name, _, ok := AsFunc("fMyFunction()"); !ok || name != "fMyFunction" {
		t.Fatalf("AsFunc = %q,%v", name, ok)
	}
	if sql, ok := AsSQL("`SELECT * FROM t`"); !ok || sql != "SELECT * FROM t" {
		t.Fatalf("AsSQL = %q,%v", sql, ok)
	}
	if s, q := Unquote(`"o'l"`); !q || s != "o'l" {
		t.Fatalf("Unquote = %q,%v", s, q)
	}
}

func TestSplitListRespectsQuotesAndCommas(t *testing.T) {
	got := SplitList(`table7, "old_table79", 'a, b'`)
	want := []string{"table7", "old_table79", "a, b"}
	if len(got) != len(want) {
		t.Fatalf("SplitList len = %v (%q)", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SplitList[%d] = %q want %q", i, got[i], want[i])
		}
	}
}

func TestNestingAndKeyGroups(t *testing.T) {
	src := "tables:\n\ttable: t1\n\t\tfields:\n\t\t\tfield: a\n\t\t\tfield: b\n\t\tuniques:\n\t\t\tunique: a, b\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 || s.Tables[0].Name != "t1" {
		t.Fatalf("tables = %+v", s.Tables)
	}
	tbl := s.Tables[0]
	if len(tbl.Fields) != 2 || tbl.Fields[0].Name != "a" || tbl.Fields[1].Name != "b" {
		t.Fatalf("fields = %+v", tbl.Fields)
	}
	if len(tbl.Uniques) != 1 || len(tbl.Uniques[0]) != 2 || tbl.Uniques[0][1] != "b" {
		t.Fatalf("uniques = %v", tbl.Uniques)
	}
	if s.HasErrors() {
		t.Fatalf("clean DDL reported errors: %v", s.Warnings)
	}
}

// Mixed indentation used to fail the whole load. SHCL recovers instead: the
// offending line is reported and skipped so the rest of the file still opens.
// The report is what matters - a silently dropped line would be the bug.
func TestInconsistentIndentIsReportedNotFatal(t *testing.T) {
	src := "tables:\n\ttable: t\n        fields:\n" // tab, then spaces
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("load should recover, got %v", err)
	}
	if !s.HasErrors() {
		t.Fatal("mixed indentation should be reported")
	}
}

func TestCommentStrippingIgnoresQuotedHash(t *testing.T) {
	src := "tables:\n\ttable: t\n\t\tfields:\n\t\t\tfield: a\n\t\t\t\tspecial: \"a#b\"  ## real comment\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tables[0].Fields[0].Special; got != "a#b" {
		t.Fatalf("special = %q want a#b", got)
	}
}

// Parses the real project/example.shcl to guard the full grammar. Path is found
// by climbing so it survives directory moves.
func TestParseExampleDDL(t *testing.T) {
	path := findExample(t)
	s, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Three tables: the worked example, plus the two built-in lookup tables the
	// file documents (lookups, lookup_values).
	if len(s.Tables) != 3 {
		t.Fatalf("tables = %d want 3 (%v)", len(s.Tables), tableNames(s))
	}
	if got := tableNames(s); got[0] != "table_name1" || got[1] != "lookups" || got[2] != "lookup_values" {
		t.Fatalf("table names = %v", got)
	}
	tbl := s.Tables[0]
	if len(tbl.Aliases) != 2 || tbl.Aliases[1] != "old_table79" {
		t.Fatalf("aliases = %v", tbl.Aliases)
	}
	if len(tbl.Fields) != 2 {
		t.Fatalf("fields = %d want 2", len(tbl.Fields))
	}
	f1 := tbl.Fields[0]
	if f1.Name != "field_name1" || f1.Type != "string" {
		t.Fatalf("field1 = %+v", f1)
	}
	// The example's default is the @null sentinel, which must resolve to NULL
	// rather than reaching SQLite as the literal text "@null".
	if !IsSentinel(f1.Default) || !IsNull(f1.Default) {
		t.Fatalf("field1 default = %q, want the @null sentinel", f1.Default)
	}
	if f1.NullOK == nil || !*f1.NullOK {
		t.Fatal("field1 null_ok want true")
	}
	if f1.UI.Order == nil || *f1.UI.Order != 3.005 {
		t.Fatalf("field1 ui.order = %v", f1.UI.Order)
	}
	if len(tbl.Uniques) != 2 || len(tbl.Uniques[0]) != 2 || tbl.Uniques[1][0] != "field_name2" {
		t.Fatalf("uniques = %v", tbl.Uniques)
	}
	if len(tbl.Indexes) != 1 || tbl.Indexes[0][0] != "field_name1" {
		t.Fatalf("indexes = %v", tbl.Indexes)
	}
	if tbl.Features.Comments || tbl.Features.AuditTrail {
		t.Fatalf("features should default off: %+v", tbl.Features)
	}

	// A lookup-backed field's list_source is SQL, so it has to survive verbatim -
	// a plain string read would split it on its comma and re-quote the pieces.
	lookupValues := s.table("lookup_values")
	if lookupValues == nil {
		t.Fatal("no lookup_values table")
	}
	groupField := lookupValues.field("lookups_idx")
	if groupField == nil {
		t.Fatal("no lookups_idx field")
	}
	if !strings.Contains(groupField.UI.ListSource, "SELECT idx, title FROM lookups") {
		t.Fatalf("list_source did not survive intact: %q", groupField.UI.ListSource)
	}
	if groupField.Default != SentinelPrevious {
		t.Fatalf("lookups_idx default = %q, want %s", groupField.Default, SentinelPrevious)
	}

	if len(s.Relations) != 2 || s.Relations[0].Type != "1:m" || s.Relations[1].Type != "m:m" {
		t.Fatalf("relations = %+v", s.Relations)
	}
	if !s.Relations[0].CascadeDelete {
		t.Fatal("relation[0] cascade_delete want true")
	}

	if len(s.Views) != 1 || s.Views[0].Name != "people" {
		t.Fatalf("views = %+v", s.Views)
	}
	if s.DefaultView != "people" {
		t.Fatalf("default view = %q want people", s.DefaultView)
	}
	layout := s.Views[0].Layout
	if len(layout) != 2 || layout[0].Name != "top" || len(layout[0].Children) != 2 {
		t.Fatalf("view layout = %+v", layout)
	}
	if layout[1].Name != "3" || len(layout[1].Location) != 3 || layout[1].Location[0] != "top" {
		t.Fatalf("block 3 = %+v", layout[1])
	}

	// The shipped example must be a clean parse: it is what users copy.
	if s.HasErrors() {
		t.Fatalf("example.shcl should parse without errors, got %v", s.Warnings)
	}
}

func tableNames(s *Schema) []string {
	var out []string
	for _, t := range s.Tables {
		out = append(out, t.Name)
	}
	return out
}

func findExample(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "project", "example.shcl")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("project/example.shcl not found; skipping integration parse")
	return ""
}

// The hierarchy does not have to be unique: duplicate wrappers merge into the
// first occurrence, and "a/b:" is shorthand for nesting - so long DDLs can
// restate a section instead of finding the right indent far above.
func TestHierarchyMergeAndShorthand(t *testing.T) {
	src := "database:\n" +
		"\ttables:\n" +
		"\t\ttable: t1\n" +
		"\t\t\tfields:\n" +
		"\t\t\t\tfield: a\n" +
		"database:\n" + // duplicate wrapper merges
		"\ttables:\n" +
		"\t\ttable: t2\n" +
		"\t\t\tfields:\n" +
		"\t\t\t\tfield: b\n" +
		"database.tables:\n" + // path shorthand
		"\ttable: t3\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: c\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 3 || s.Tables[0].Name != "t1" || s.Tables[2].Name != "t3" {
		t.Fatalf("tables = %+v", s.Tables)
	}
	for _, w := range s.Warnings {
		if strings.Contains(w, "duplicate") {
			t.Errorf("container merging must not warn: %s", w)
		}
	}
}

// Old flat DDLs (no database:/ui: wrappers, `methods:` not `code:`) must keep
// parsing unchanged.
func TestFlatLayoutAndMethodsKeyStillParse(t *testing.T) {
	src := "methods:\n" +
		"\tbefore_open: fBoot()\n" +
		"tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\tmethods:\n" +
		"\t\t\tbefore_update: fCheck()\n" +
		"views:\n" +
		"\tview: v\n" +
		"\t\tlayout:\n" +
		"\t\t\tblock: b\n" +
		"\t\t\t\ttable: t\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.AppCode["before_open"] != "fBoot()" {
		t.Fatalf("app code = %v", s.AppCode)
	}
	if len(s.Tables) != 1 || s.Tables[0].Code["before_update"] != "fCheck()" {
		t.Fatalf("table code = %+v", s.Tables)
	}
	if len(s.Views) != 1 || s.DefaultView != "" {
		t.Fatalf("views = %+v default = %q", s.Views, s.DefaultView)
	}
}

func TestUIWrapperAndDefaultView(t *testing.T) {
	src := "code:\n" +
		"\tafter_open: fReady()\n" +
		"ui:\n" +
		"\tviews:\n" +
		"\t\tview: alpha\n" +
		"\t\t\tlayout:\n" +
		"\t\t\t\tblock: b\n" +
		"\t\t\t\t\ttable: t\n" +
		"\t\tview: beta\n" +
		"\t\t\tlayout:\n" +
		"\t\t\t\tblock: b\n" +
		"\t\t\t\t\ttable: t\n" +
		"\tdefault_view: \"beta\"\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.AppCode["after_open"] != "fReady()" {
		t.Fatalf("app code = %v", s.AppCode)
	}
	if len(s.Views) != 2 || s.DefaultView != "beta" {
		t.Fatalf("views = %+v default = %q", s.Views, s.DefaultView)
	}
}

// Parse-time diagnostics: this DDL targets users without SQL knowledge, so
// mistakes must surface as warnings, not silence or opaque SQLite errors.
func TestParserWarnings(t *testing.T) {
	src := "tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\t\t\ttype: string\n" +
		"\t\t\t\ttype: int\n" + // duplicate key
		"\t\t\tfield: b\n" +
		"\t\t\t\ttitle: John's list # swallowed comment\n" + // unclosed quote hides '#'
		"\t\tuniques:\n" +
		"\t\t\ta, nope\n" // unknown field
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tables[0].Fields[0].Type; got != "string" {
		t.Errorf("conflicting duplicate scalar: first value must win, got %q", got)
	}
	for _, want := range []string{"repeats as a bare leaf", "unknown field"} {
		found := false
		for _, w := range s.Warnings {
			if strings.Contains(w, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no %q warning in %v", want, s.Warnings)
		}
	}
	// Repeating keys (field, table) must NOT warn.
	for _, w := range s.Warnings {
		if strings.Contains(w, `"field"`) || strings.Contains(w, `"table"`) {
			t.Errorf("false duplicate warning: %s", w)
		}
	}
}

// Load-time validation: what can be safely assumed around is fixed with a
// warning (nameless fields dropped, system-column collisions dropped, unknown
// types stored as text); nothing here may hard-error.
//
// Restating an entity MERGES it rather than dropping the repeat: two `table: t`
// sections are one node by SHCL's (name, value) merge rule, so the second one's
// fields join the first. That is the same rule that lets a long DDL add to a
// table further down the file, and it is why there is no "already defined"
// warning for a table - a repeat is indistinguishable from a deliberate addition.
func TestLoadValidationSoftFixes(t *testing.T) {
	src := "tables:\n" +
		"\ttable: t\n" +
		"\t\tfields:\n" +
		"\t\t\tfield: a\n" +
		"\t\t\t\ttype: strng\n" + // typo'd type
		"\t\t\tfield:\n" + // no name
		"\t\t\tfield: id\n" + // system column: ui: only, so type: is ignored
		"\t\t\t\ttype: string\n" +
		"\ttable: t\n" + // restated: merges into the first
		"\t\tfields:\n" +
		"\t\t\tfield: other\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tables) != 1 {
		t.Fatalf("restated table should merge into one, got %d", len(s.Tables))
	}
	names := []string{}
	for _, f := range s.Tables[0].Fields {
		names = append(names, f.Name)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "other" {
		t.Fatalf("fields = %v, want [a other]", names)
	}
	if s.Tables[0].Fields[0].Type != "strng" {
		t.Fatalf("unknown type should store as written, got %q", s.Tables[0].Fields[0].Type)
	}
	warns := strings.Join(s.Warnings, "\n")
	for _, want := range []string{
		"field with no name", "is a system field", "value not allowed",
	} {
		if !strings.Contains(warns, want) {
			t.Errorf("warnings missing %q:\n%s", want, warns)
		}
	}
}

func TestIrregularIndentWarns(t *testing.T) {
	src := "tables:\n" +
		"  table: t\n" +
		"    fields:\n" +
		"   field: a\n" // 3 spaces on a 2-space file
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range s.Warnings {
		if strings.Contains(w, "matches no open level") {
			found = true
		}
	}
	if !found {
		t.Errorf("no indent warning in %v", s.Warnings)
	}
}

func TestTunables(t *testing.T) {
	src := "tunables:\n" +
		"\tgit_sync_frequency: 30\n" +
		"\tgc_age_days: 45\n" +
		"\tmystery: 9\n" +
		"tunables:\n" + // duplicate section merges; first value wins
		"\tgit_sync_frequency: 99\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.TunableInt("git_sync_frequency", 60); got != 30 {
		t.Fatalf("git_sync_frequency = %d", got)
	}
	if got := s.TunableInt("gc_age_days", 90); got != 45 {
		t.Fatalf("gc_age_days = %d", got)
	}
	if got := s.TunableInt("absent", 7); got != 7 {
		t.Fatalf("absent tunable = %d", got)
	}
	joined := strings.Join(s.Warnings, "\n")
	if !strings.Contains(joined, "tunables.mystery") {
		t.Fatalf("an unknown tunable should be reported: %v", s.Warnings)
	}
	// Reported, but still stored: a tunable a newer client wrote has to survive
	// a round trip through an older one that does not know it.
	if s.Tunables["mystery"] != "9" {
		t.Fatalf("unknown tunable should still store, got %q", s.Tunables["mystery"])
	}
}

// The default-value sentinels are app-computed, so neither may ever reach SQLite
// as a literal. @previous is the one that used to slip through: only @null was
// recognized, so on any column SQLite accepts a string for, the column came out
// with DEFAULT '@previous' baked in.
func TestSentinelDefaults(t *testing.T) {
	for _, sentinel := range []string{SentinelNull, SentinelPrevious} {
		if !IsSentinel(sentinel) {
			t.Errorf("%s should be a sentinel", sentinel)
		}
	}
	// A bare NULL predates the @ spelling and still means the same thing.
	if !IsNull("NULL") || !IsNull(SentinelNull) {
		t.Error("NULL and @null should both resolve to null")
	}
	// @previous is a sentinel but not a null: it inherits a real value at entry.
	if IsNull(SentinelPrevious) {
		t.Error("@previous is not a null default")
	}
	// An ordinary value is neither, or real defaults would stop being emitted.
	for _, literal := range []string{"general", "0", "fMyFunction()"} {
		if IsSentinel(literal) || IsNull(literal) {
			t.Errorf("%q should be an ordinary default", literal)
		}
	}
	// Quoting escapes a sentinel, which is what the DDL has always documented.
	for _, quoted := range []string{`"@null"`, `'@null'`, `"@previous"`} {
		if IsSentinel(quoted) || IsNull(quoted) {
			t.Errorf("%s is a quoted literal, not a sentinel", quoted)
		}
	}
}

// The escape only works if a default reaches IsSentinel as its source text, so
// this walks the real path: parse a DDL, and check what each spelling became.
func TestQuotedSentinelSurvivesParse(t *testing.T) {
	src := "tables:\n\ttable: t\n\t\tfields:\n" +
		"\t\t\tfield: bare\n\t\t\t\ttype: string\n\t\t\t\tdefaultval: @null\n" +
		"\t\t\tfield: quoted\n\t\t\t\ttype: string\n\t\t\t\tdefaultval: \"@null\"\n" +
		"\t\t\tfield: plain\n\t\t\t\ttype: string\n\t\t\t\tdefaultval: general\n"
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"bare": true, "quoted": false, "plain": false}
	table := s.Tables[0]
	for _, f := range table.Fields {
		got := IsSentinel(f.Default)
		if got != want[f.Name] {
			t.Errorf("field %s: default %q sentinel=%v, want %v", f.Name, f.Default, got, want[f.Name])
		}
	}
	// The quoted one has to end up as the literal four characters, not with its
	// quotes baked into the value.
	for _, f := range table.Fields {
		if f.Name != "quoted" {
			continue
		}
		if unquoted, was := Unquote(f.Default); !was || unquoted != "@null" {
			t.Errorf("quoted default %q should unquote to @null", f.Default)
		}
	}
}

// shcl hints on a repeated bare leaf ("did you mean an array?"). For the keys
// whose whole design is to recur, the schema declares a repeat bound and shcl's
// own filter drops the hint - so a correct DDL stays quiet and the hint keeps
// working where a repeat really is the mistake.
func TestDeclaredRepeatsStaySilent(t *testing.T) {
	quiet := "tables:\n\ttable: t\n\t\tfields:\n\t\t\tfield: a\n\t\t\t\ttype: string\n" +
		"\t\t\tfield: b\n\t\t\t\ttype: string\n" +
		"\t\tuniques:\n\t\t\tunique: a\n\t\t\tunique: b\n" +
		"\t\tindexes:\n\t\t\tindex: a\n\t\t\tindex: b\n"
	s, _ := Parse([]byte(quiet))
	for _, w := range s.Warnings {
		t.Errorf("repeated unique/index should not warn: %s", w)
	}

	// aliases is not declared repeating, so doubling it is still flagged.
	noisy := "tables:\n\ttable: t\n\t\taliases: old1\n\t\taliases: old2\n"
	s, _ = Parse([]byte(noisy))
	if len(s.Warnings) == 0 {
		t.Error("a repeated aliases: should still be hinted")
	}
}

// Restating a section combines it with the earlier one. That is legal and often
// what was meant, but it is also how two tables of the same name silently become
// one, so it has to be reported either way - the whole point of the hint.
func TestSectionMergeIsReported(t *testing.T) {
	// Same table named twice under one section: the entity itself is named.
	sameSection := "tables:\n\ttable: dup\n\t\tfields:\n\t\t\tfield: a\n\t\t\t\ttype: string\n" +
		"\ttable: other\n\ttable: dup\n"
	s, _ := Parse([]byte(sameSection))
	if !hasWarning(s, "merged with 'table'") {
		t.Errorf("a restated table should be reported, got %v", s.Warnings)
	}

	// Across two sections shcl reports the outer wrapper only, and the table
	// underneath merges silently - which is exactly why the wrapper report is
	// kept rather than filtered as noise.
	across := "tables:\n\ttable: dup\n\t\tfields:\n\t\t\tfield: a\n\t\t\t\ttype: string\n" +
		"\ntunables:\n\tgc_age_days: 30\n" +
		"\ntables:\n\ttable: dup\n\t\tfields:\n\t\t\tfield: b\n\t\t\t\ttype: string\n"
	s, _ = Parse([]byte(across))
	if len(s.Tables) != 1 {
		t.Fatalf("the two dup tables should have merged into one, got %d", len(s.Tables))
	}
	if !hasWarning(s, "merged with 'tables'") {
		t.Errorf("the merge should be reported, got %v", s.Warnings)
	}

	// A schema written in one pass says nothing, which is what the shipped
	// example and the built-in schema rely on.
	clean := "tables:\n\ttable: a\n\t\tfields:\n\t\t\tfield: x\n\t\t\t\ttype: string\n\ttable: b\n"
	s, _ = Parse([]byte(clean))
	for _, w := range s.Warnings {
		t.Errorf("a single-pass schema should be quiet: %s", w)
	}
}

// The unknown-field sweep only runs against a fault-free schema, so a typo in
// schema.shcl would quietly stop unknown keys ever being reported and a broken
// vocabulary would look exactly like a clean one. Probing with a key no DDL
// could ever have proves the sweep is live, which is only true when the
// embedded schema itself parses clean.
func TestSchemaSelfCheck(t *testing.T) {
	s, _ := Parse([]byte("no_such_top_level_key: 1\n"))
	if !hasWarning(s, "no_such_top_level_key") {
		t.Errorf("unknown top-level key went unreported, so schema.shcl has a fault: %v", s.Warnings)
	}
}

// A fault in the vocabulary used to switch validation off wholesale, so a
// broken schema.shcl and a clean one produced identical silence. It no longer
// does: the surviving constraints still check, and the fault is reported
// against the vocabulary rather than dressed up as a line of the user's DDL.
func TestSchemaFaultLeavesValidationOn(t *testing.T) {
	broken := shcl.Parse("field: a\n\ttype: no_such_type\nfield: b\n\ttype: int\n")
	doc := shcl.Parse("a: whatever\nb: not_a_number\n")

	var fault, survived bool
	for _, d := range doc.Validate(broken) {
		if isSchemaFault(d) {
			fault = true
			continue
		}
		if strings.Contains(d.Message, "wrong type") {
			survived = true
		}
	}
	if !fault {
		t.Error("the schema fault should still be reported")
	}
	if !survived {
		t.Error("constraints that parsed cleanly should still check the document")
	}
}

// Layout blocks nest by mounting their own shape, so there is no depth past
// which keys quietly stop being checked. 40 is far beyond anything a real
// layout reaches - the point is that nothing caps it.
func TestLayoutNestsToAnyDepth(t *testing.T) {
	build := func(depth int, deepKey string) []byte {
		var b strings.Builder
		b.WriteString("views:\n\tview: v\n\t\tlayout:\n")
		indent := "\t\t\t"
		for i := 0; i < depth; i++ {
			fmt.Fprintf(&b, "%sblock: %d\n", indent, i)
			indent += "\t"
			fmt.Fprintf(&b, "%stype: grid\n", indent)
			if i == depth-1 && deepKey != "" {
				fmt.Fprintf(&b, "%s%s: x\n", indent, deepKey)
			}
			b.WriteString(indent + "block:\n")
			indent += "\t"
		}
		return []byte(b.String())
	}

	for _, depth := range []int{3, 12, 40} {
		s, _ := Parse([]byte(build(depth, "")))
		for _, w := range s.Warnings {
			t.Errorf("depth %d: a legal layout should be quiet: %s", depth, w)
		}
		s, _ = Parse(build(depth, "bogus_block_key"))
		if !hasWarning(s, "bogus_block_key") {
			t.Errorf("depth %d: a bad key stopped being checked: %v", depth, s.Warnings)
		}
	}
}

func hasWarning(s *Schema, substr string) bool {
	for _, w := range s.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
