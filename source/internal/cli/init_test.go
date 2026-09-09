// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jim-collier/nano-git-db/internal/core/config"
)

// initEnv isolates the registry base and switches to a fresh working directory
// holding one DDL, returning that dir. It restores cwd and the override after.
func initEnv(t *testing.T) (workDir, base string) {
	t.Helper()
	base = t.TempDir()
	config.SetBaseOverride(base)
	t.Cleanup(func() { config.SetBaseOverride("") })

	workDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "todo.ddl"), []byte(personDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	return workDir, base
}

// --init with an explicit dir registers a record naming the DDL, that dir, and
// materializes the view.
func TestInitExplicitDir(t *testing.T) {
	work, base := initEnv(t)
	logDir := t.TempDir()

	if err := Init([]string{logDir}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(base, "todo"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "todo" {
		t.Fatalf("name = %q", cfg.Name)
	}
	if cfg.DDLPath != filepath.Join(work, "todo.ddl") {
		t.Fatalf("ddl_path = %q", cfg.DDLPath)
	}
	if cfg.LogDir != logDir {
		t.Fatalf("log_dir = %q, want %q", cfg.LogDir, logDir)
	}
	// The view was materialized (build + seed), so the sqlite exists.
	if _, err := os.Stat(cfg.SQLitePath); err != nil {
		t.Fatalf("sqlite not built: %v", err)
	}
}

// --init with no arg in a non-git directory uses the current directory as the
// tx-log location.
func TestInitNoArgUsesPWD(t *testing.T) {
	work, base := initEnv(t)
	if err := Init(nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(base, "todo"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogDir != work {
		t.Fatalf("log_dir = %q, want the working dir %q", cfg.LogDir, work)
	}
}

func TestInitRefusesDuplicate(t *testing.T) {
	_, _ = initEnv(t)
	if err := Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := Init(nil); err == nil {
		t.Fatal("second --init should refuse the already-registered name")
	}
}

func TestInitNeedsDDL(t *testing.T) {
	base := t.TempDir()
	config.SetBaseOverride(base)
	t.Cleanup(func() { config.SetBaseOverride("") })
	empty := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	if err := Init(nil); err == nil {
		t.Fatal("--init with no .ddl in $PWD should error")
	}
}
