// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package schema

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/txlog"
)

const syncDDL = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n"

func gitrun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Auto-sync end to end: client A writes and syncs; client B's background
// loop pulls the entry and full-replays it into B's view.
func TestAutoSyncPullsAndReplays(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	gitrun(t, base, "init", "--bare", remote)
	logA, logB := filepath.Join(base, "a"), filepath.Join(base, "b")
	gitrun(t, base, "clone", remote, logA)
	// Sync's own `git commit` runs with the ambient env (no identity injected),
	// so give the clone a repo-local one - a bare CI runner has no global identity.
	gitrun(t, logA, "config", "user.email", "t@t")
	gitrun(t, logA, "config", "user.name", "t")
	ddlPath := filepath.Join(base, "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(syncDDL), 0o644); err != nil {
		t.Fatal(err)
	}

	ca, err := OpenClient(ddlPath, filepath.Join(base, "a.sqlite"), logA, "ua")
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Close()
	id, err := ca.API.Create("person", map[string]string{"name": "Ann"})
	if err != nil {
		t.Fatal(err)
	}
	lgA, err := txlog.Open(logA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txlog.NewSyncer(lgA, time.Minute).Sync(); err != nil {
		t.Fatal(err)
	}

	// B joins the usual way: cloning the already-populated log repo
	gitrun(t, base, "clone", remote, logB)
	gitrun(t, logB, "config", "user.email", "t@t")
	gitrun(t, logB, "config", "user.name", "t")
	cb, err := OpenClient(ddlPath, filepath.Join(base, "b.sqlite"), logB, "ub")
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()
	if _, ok, _ := cb.API.Get("person", id); !ok {
		t.Fatal("open-time replay should already show Ann")
	}
	stop := cb.StartAutoSync(1, nil) // 1s ticks
	defer stop()

	// a row written AFTER B opened can only arrive through the loop
	id2, err := ca.API.Create("person", map[string]string{"name": "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txlog.NewSyncer(lgA, time.Minute).Sync(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if row, ok, _ := cb.API.Get("person", id2); ok && row["name"] == "Bob" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("client B never saw client A's second row via auto-sync")
}

func TestAutoSyncNoopOutsideRepo(t *testing.T) {
	ddlPath := filepath.Join(t.TempDir(), "s.ddl")
	if err := os.WriteFile(ddlPath, []byte(syncDDL), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	c, err := OpenClient(ddlPath, filepath.Join(dir, "v.sqlite"), filepath.Join(dir, "log"), "u")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	stop := c.StartAutoSync(60, nil) // plain dir: must not start anything
	stop()
	stop = c.StartAutoSync(0, nil) // freq 0 disables
	stop()
}
