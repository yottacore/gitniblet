// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Deregister drops the registry record but leaves the tx-log and DDL alone.
func TestDeregisterKeepsFiles(t *testing.T) {
	isolate(t)
	ddl := writeDDL(t, t.TempDir())
	logDir := t.TempDir()
	cfg, err := Create("todo", ddl, logDir)
	if err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(cfg.Dir(), recordFile)
	if _, err := os.Stat(rec); err != nil {
		t.Fatalf("record not written: %v", err)
	}
	if err := cfg.Deregister(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rec); !os.IsNotExist(err) {
		t.Fatalf("record still present after deregister: %v", err)
	}
	if _, err := os.Stat(logDir); err != nil {
		t.Fatalf("tx-log wrongly removed: %v", err)
	}
	if _, err := os.Stat(ddl); err != nil {
		t.Fatalf("ddl wrongly removed: %v", err)
	}
	// Deregister on an already-gone record is a no-op, not an error.
	if err := cfg.Deregister(); err != nil {
		t.Fatalf("second deregister errored: %v", err)
	}
}

// DeleteFiles removes the tx-log and record dir but never the DDL.
func TestDeleteFilesKeepsDDL(t *testing.T) {
	isolate(t)
	ddl := writeDDL(t, t.TempDir())
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "txlog.csv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Create("todo", ddl, logDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasFiles() {
		t.Fatal("HasFiles = false, want true (tx-log exists)")
	}
	if err := cfg.DeleteFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logDir); !os.IsNotExist(err) {
		t.Fatalf("tx-log dir kept: %v", err)
	}
	if _, err := os.Stat(cfg.Dir()); !os.IsNotExist(err) {
		t.Fatalf("record dir kept: %v", err)
	}
	if _, err := os.Stat(ddl); err != nil {
		t.Fatalf("ddl deleted, should be kept: %v", err)
	}
}
