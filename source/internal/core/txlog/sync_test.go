// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestInRepoAndSkip(t *testing.T) {
	dir := t.TempDir()
	if InRepo(dir) {
		t.Fatal("temp dir should not be a git repo")
	}
	lg, _ := Open(dir)
	res, err := NewSyncer(lg, time.Minute).Sync()
	if err != nil || !res.Skipped {
		t.Fatalf("non-repo sync: %+v err=%v", res, err)
	}
}

func TestSyncCommitsLocally(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	lg, _ := Open(dir)
	if err := lg.Append(Entry{TxID: "a1", Date: "2026-07-01T00:00:00.000Z", Table: "person", RowID: "0a0a", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	res, err := NewSyncer(lg, time.Minute).Sync()
	if err != nil {
		t.Fatal(err)
	}
	if !res.Committed || !res.NoRemote {
		t.Fatalf("want committed+noremote, got %+v", res)
	}
	if out, _ := gitOut(dir, "log", "--oneline"); out == "" {
		t.Fatal("expected a commit in the log repo")
	}
}

// Two clients appending concurrently must converge to the same entry set after
// syncing through a shared remote (union merge + canonical replay order).
func TestTwoClientsConverge(t *testing.T) {
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	if _, err := gitOut(base, "-c", "init.defaultBranch=main", "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}

	// Seed the remote with the log file so clients append (not add/add).
	seed := clone(t, base, remote, "seed")
	seedLog, _ := Open(seed)
	if err := seedLog.Append(Entry{TxID: "s0", Date: "2026-07-01T00:00:00.000Z", Table: "person", RowID: "0000", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(seedLog, time.Minute).Sync(); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	a := clone(t, base, remote, "a")
	b := clone(t, base, remote, "b")
	la, _ := Open(a)
	lb, _ := Open(b)

	if err := la.Append(Entry{TxID: "a1", Date: "2026-07-01T00:00:01.000Z", Table: "person", RowID: "0a0a", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	if err := lb.Append(Entry{TxID: "b1", Date: "2026-07-01T00:00:02.000Z", Table: "person", RowID: "0b0b", Op: "create"}); err != nil {
		t.Fatal(err)
	}

	// A pushes first, then B (pull merges A), then A pulls B's merge.
	if _, err := NewSyncer(la, time.Minute).Sync(); err != nil {
		t.Fatalf("A sync 1: %v", err)
	}
	if _, err := NewSyncer(lb, time.Minute).Sync(); err != nil {
		t.Fatalf("B sync: %v", err)
	}
	if _, err := NewSyncer(la, time.Minute).Sync(); err != nil {
		t.Fatalf("A sync 2: %v", err)
	}

	want := []string{"a1", "b1", "s0"}
	if got := txIDs(t, la); !equal(got, want) {
		t.Fatalf("client A ids = %v, want %v", got, want)
	}
	if got := txIDs(t, lb); !equal(got, want) {
		t.Fatalf("client B ids = %v, want %v", got, want)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init")
	mustGit(t, dir, "config", "user.email", "test@example.invalid")
	mustGit(t, dir, "config", "user.name", "Test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
}

func clone(t *testing.T, base, remote, name string) string {
	t.Helper()
	mustGit(t, base, "clone", remote, name)
	dir := filepath.Join(base, name)
	mustGit(t, dir, "config", "user.email", "test@example.invalid")
	mustGit(t, dir, "config", "user.name", "Test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func txIDs(t *testing.T, lg *Log) []string {
	t.Helper()
	entries, _, err := lg.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, e := range entries {
		if !seen[e.TxID] {
			seen[e.TxID] = true
			ids = append(ids, e.TxID)
		}
	}
	sort.Strings(ids)
	return ids
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRepoAccount(t *testing.T) {
	dir := t.TempDir()
	if RepoAccount(dir) != "" {
		t.Fatal("a non-repo dir should have no account")
	}
	gitInit(t, dir) // sets user.name = Test
	if got := RepoAccount(dir); got != "Test" {
		t.Fatalf("RepoAccount = %q, want Test", got)
	}
	// With no name, it falls back to the email.
	mustGit(t, dir, "config", "--unset", "user.name")
	if got := RepoAccount(dir); got != "test@example.invalid" {
		t.Fatalf("RepoAccount email fallback = %q", got)
	}
}
