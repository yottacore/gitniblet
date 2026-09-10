// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package config

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// resolve canonicalizes a path so comparisons survive symlinked temp dirs (git
// reports a resolved repo root).
func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// A plain (non-repo) folder is used verbatim - no ngdb/<name> subfolder.
func TestLogDirForPlainFolder(t *testing.T) {
	dir := t.TempDir()
	got, err := LogDirFor(dir, "todo")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("plain folder: got %q, want verbatim %q", got, dir)
	}
}

// Pointing at a git repo's top level derives <root>/ngdb/<name>.
func TestLogDirForRepoSubfolder(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v (%s)", err, out)
	}
	got, err := LogDirFor(repo, "todo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolve(repo), "ngdb", "todo")
	if resolve(got) != want {
		t.Fatalf("repo subfolder: got %q, want %q", resolve(got), want)
	}
}

// An empty location falls back to the current directory (here, a non-repo temp).
func TestLogDirForEmptyUsesCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := LogDirFor("", "todo")
	if err != nil {
		t.Fatal(err)
	}
	if resolve(got) != resolve(dir) {
		t.Fatalf("empty location: got %q, want cwd %q", resolve(got), resolve(dir))
	}
}
