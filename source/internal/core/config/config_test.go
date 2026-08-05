// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const miniDDL = "tables:\n" +
	"\ttable: task\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: title\n" +
	"\t\t\t\ttype: string\n"

// isolate points UserBase at a temp dir and clears the system-dir env so a test
// never sees the developer's real registry.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)        // Linux user base
	t.Setenv("AppData", home)                // Windows user base
	t.Setenv("XDG_CONFIG_DIRS", t.TempDir()) // empty system base
	t.Setenv("ProgramData", t.TempDir())
	base, err := UserBase()
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func writeDDL(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "schema.ddl")
	if err := os.WriteFile(p, []byte(miniDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateLoadRoundTrip(t *testing.T) {
	isolate(t)
	ddlPath := writeDDL(t, t.TempDir())
	logDir := t.TempDir()

	cfg, err := Create("todo", ddlPath, logDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DDLPath != ddlPath || cfg.LogDir != logDir {
		t.Fatalf("paths not stored: %+v", cfg)
	}
	// Defaults key off the record dir + name.
	if cfg.SQLitePath != filepath.Join(cfg.Dir(), "todo.sqlite") {
		t.Fatalf("sqlite default = %q", cfg.SQLitePath)
	}
	if cfg.KeyFile != filepath.Join(cfg.Dir(), "todo.key") {
		t.Fatalf("key default = %q", cfg.KeyFile)
	}

	got, err := Load(cfg.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "todo" || got.DDLPath != ddlPath || got.LogDir != logDir {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestFindByName(t *testing.T) {
	isolate(t)
	ddlPath := writeDDL(t, t.TempDir())
	if _, err := Create("issues", ddlPath, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	if cfg := FindByName("issues"); cfg == nil || cfg.DDLPath != ddlPath {
		t.Fatalf("exact name lookup failed: %+v", cfg)
	}
	// A file extension on the query is ignored.
	if cfg := FindByName("issues.ddl"); cfg == nil {
		t.Fatal("name with extension should resolve")
	}
	// Unknown names and the empty string yield nil, not a panic.
	if FindByName("nope") != nil || FindByName("") != nil {
		t.Fatal("unknown/empty name should be nil")
	}
}

func TestCreateRefusesDuplicate(t *testing.T) {
	isolate(t)
	ddlPath := writeDDL(t, t.TempDir())
	if _, err := Create("dup", ddlPath, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("dup", ddlPath, t.TempDir()); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
}

func TestTouchStampsLastOpened(t *testing.T) {
	isolate(t)
	ddlPath := writeDDL(t, t.TempDir())
	cfg, _ := Create("t", ddlPath, t.TempDir())
	if cfg.LastOpened != "" {
		t.Fatal("last_opened should start empty")
	}
	if err := cfg.Touch(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(cfg.Dir())
	if got.LastOpened == "" {
		t.Fatal("last_opened not persisted")
	}
}

func TestListValidatesRecords(t *testing.T) {
	isolate(t)
	ddlDir := t.TempDir()
	goodDDL := writeDDL(t, ddlDir)

	// good record
	if _, err := Create("good", goodDDL, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// record whose DDL was deleted
	missing, _ := Create("missing", filepath.Join(ddlDir, "gone.ddl"), t.TempDir())
	_ = missing
	// record with a corrupt DDL (mixed indentation is a hard parse error)
	badDDLDir := t.TempDir()
	badDDL := filepath.Join(badDDLDir, "bad.ddl")
	if err := os.WriteFile(badDDL, []byte("tables:\n\ttable: x\n        fields:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("corrupt", badDDL, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	byName := map[string]Listed{}
	for _, l := range List() {
		byName[l.Name] = l
	}
	if len(byName) != 3 {
		t.Fatalf("want 3 discovered, got %d: %v", len(byName), byName)
	}
	if byName["good"].Err != nil {
		t.Fatalf("good record should be openable: %v", byName["good"].Err)
	}
	if byName["missing"].Err == nil {
		t.Fatal("missing-DDL record should report an error")
	}
	if byName["corrupt"].Err == nil {
		t.Fatal("corrupt-DDL record should report an error")
	}
}

func TestBaseOverride(t *testing.T) {
	isolate(t)
	def, err := UserBase()
	if err != nil {
		t.Fatal(err)
	}
	alt := t.TempDir()
	SetBaseOverride(alt)
	t.Cleanup(func() { SetBaseOverride("") })
	got, err := UserBase()
	if err != nil {
		t.Fatal(err)
	}
	if got != alt {
		t.Fatalf("override ignored: got %q want %q", got, alt)
	}
	if got == def {
		t.Fatal("override should differ from the default base")
	}
	SetBaseOverride("")
	if back, _ := UserBase(); back != def {
		t.Fatalf("clearing override did not restore default: %q vs %q", back, def)
	}
}

func TestPWDDdl(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if _, ok := PWDDdl(); ok {
		t.Fatal("no ddl yet, want false")
	}
	writeDDL(t, dir)
	p, ok := PWDDdl()
	if !ok || filepath.Base(p) != "schema.ddl" {
		t.Fatalf("want schema.ddl, got %q ok=%v", p, ok)
	}
	// A second .ddl makes it ambiguous.
	if err := os.WriteFile(filepath.Join(dir, "other.ddl"), []byte(miniDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := PWDDdl(); ok {
		t.Fatal("two ddls is ambiguous, want false")
	}
}

// chdir switches to dir for the test and restores the old cwd after.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// A schema that lost something on the way in is not a database to open
// silently. This is the far end of the DDL error count: a unique group naming
// a field that does not exist used to list as fine and then fail to open with
// a raw SQLite error.
func TestListFlagsASchemaWithErrors(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.ddl")
	if err := os.WriteFile(broken, []byte(miniDDL+
		"\t\tuniques:\n\t\t\tunique: ttile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("broken", broken, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	listed := List()
	if len(listed) != 1 {
		t.Fatalf("listed %d databases, want 1", len(listed))
	}
	if listed[0].Err == nil {
		t.Fatal("a schema with errors should list as unopenable, with a reason")
	}
	if !strings.Contains(listed[0].Err.Error(), "ttile") {
		t.Errorf("the reason should name the problem: %v", listed[0].Err)
	}
}

// The control: a sound schema still lists clean.
func TestListAcceptsASoundSchema(t *testing.T) {
	isolate(t)
	if _, err := Create("fine", writeDDL(t, t.TempDir()), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	listed := List()
	if len(listed) != 1 || listed[0].Err != nil {
		t.Fatalf("sound schema did not list clean: %+v", listed)
	}
}
