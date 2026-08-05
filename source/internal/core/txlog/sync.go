// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package txlog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Syncing shells out to the `git` binary (no libgit2) to keep a log dir's repo
// in step with its remote. The log is append-only and set to merge as a union,
// and replay sorts by (date, tx_id), so concurrent clients converge.

// SyncResult summarizes one sync pass.
type SyncResult struct {
	Skipped   bool // dir is not a git repo - sync is a no-op, local logging still works
	Committed bool
	Pulled    bool
	Pushed    bool
	NoRemote  bool // repo has no remote to push to
	Changed   bool // the pull brought new entries (HEAD moved)
}

// Syncer reconciles one log dir's git repo with its remote.
type Syncer struct {
	dir      string
	rel      string
	interval time.Duration

	// OnChange (optional) runs after a pull that brought new entries. The safe
	// reconciliation is a full schema-build + replay of the whole log:
	// incremental apply is order-sensitive (a pulled entry may sort before
	// already-applied local ones), so callers rebuild rather than patch.
	OnChange func() error
}

// NewSyncer targets the git working dir holding lg. interval is used by Run.
func NewSyncer(lg *Log, interval time.Duration) *Syncer {
	return &Syncer{dir: filepath.Dir(lg.path), rel: filepath.Base(lg.path), interval: interval}
}

// InRepo reports whether dir is inside a git work tree.
func InRepo(dir string) bool {
	out, err := gitOut(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// RepoRoot returns the work-tree top level containing dir, or ("", false) when
// dir is not inside a git repo. Used by --init to auto-place the tx-log under a
// repo's own subfolder.
func RepoRoot(dir string) (string, bool) {
	out, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(out)
	return root, root != ""
}

// RepoAccount returns the git account name configured for dir - user.name, or
// user.email when the name is unset - or "" when dir is not a repo or nothing is
// configured. The web UI uses it to identify the local user without a password.
func RepoAccount(dir string) string {
	if !InRepo(dir) {
		return ""
	}
	if out, err := gitOut(dir, "config", "user.name"); err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return name
		}
	}
	if out, err := gitOut(dir, "config", "user.email"); err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}

// Sync runs one pass: commit local log changes, then reconcile with the remote.
// A non-git log dir is a no-op. Union merge auto-resolves append conflicts.
func (s *Syncer) Sync() (SyncResult, error) {
	var res SyncResult
	if !InRepo(s.dir) {
		res.Skipped = true
		return res, nil
	}
	recoverMerge(s.dir)
	if err := ensureUnionAttr(s.dir, s.rel); err != nil {
		return res, err
	}
	if err := ensureIgnores(s.dir); err != nil {
		return res, err
	}
	// Stage the whole dir: the live log, retired GC segments (and their
	// deletions), and the attachments/ subfolder all ride the same sync.
	if _, err := gitOut(s.dir, "add", "-A", "--", "."); err != nil {
		return res, err
	}
	if stagedAny(s.dir) {
		msg := "txlog sync " + time.Now().UTC().Format(time.RFC3339)
		if _, err := gitOut(s.dir, "commit", "-m", msg); err != nil {
			return res, err
		}
		res.Committed = true
	}

	if !hasRemote(s.dir) {
		res.NoRemote = true
		return res, nil
	}
	if hasUpstream(s.dir) {
		// Head before and after the pull, to tell whether it brought anything
		// in. A failed read just leaves Changed false, which only costs a
		// redundant replay.
		before, _ := gitOut(s.dir, "rev-parse", "HEAD")
		if _, err := gitOut(s.dir, "pull", "--no-rebase", "--no-edit"); err != nil {
			// A genuine conflict (some non-union file) would otherwise leave
			// the repo mid-merge and fail every future pass.
			recoverMerge(s.dir)
			return res, fmt.Errorf("pull: %w", err)
		}
		res.Pulled = true
		after, _ := gitOut(s.dir, "rev-parse", "HEAD")
		res.Changed = strings.TrimSpace(before) != strings.TrimSpace(after)
		if _, err := gitOut(s.dir, "push"); err != nil {
			return res, fmt.Errorf("push: %w", err)
		}
		res.Pushed = true
	} else {
		// Remote exists but this branch has no upstream yet (fresh repo).
		if _, err := gitOut(s.dir, "push", "-u", "origin", "HEAD"); err != nil {
			return res, fmt.Errorf("push -u: %w", err)
		}
		res.Pushed = true
	}
	return res, nil
}

// Run syncs every interval until ctx is canceled. onErr (optional) receives
// per-pass errors so the loop keeps running.
func (s *Syncer) Run(ctx context.Context, onErr func(error)) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			res, err := s.Sync()
			if err != nil {
				if onErr != nil {
					onErr(err)
				}
				continue
			}
			if res.Changed && s.OnChange != nil {
				if err := s.OnChange(); err != nil && onErr != nil {
					onErr(err)
				}
			}
		}
	}
}

// ensureUnionAttr sets the log file to merge as a union, so two clients' appends
// combine instead of conflicting.
func ensureUnionAttr(dir, rel string) error {
	path := filepath.Join(dir, ".gitattributes")
	// txlog-*.csv: GC segments are write-once, but two clients collecting
	// concurrently both delete the old files - union keeps that automerging.
	return appendLinesOnce(path, rel+" merge=union", "txlog-*.csv merge=union")
}

// ensureIgnores keeps derived and secret files out of the synced log directory.
// It matters when the schema sits in the repo and the log directory is the same
// folder: the SQLite view would then be committed as a binary blob no merge
// driver can reconcile, and a key file placed beside the schema would be pushed
// next to the data it decrypts.
func ensureIgnores(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	return appendLinesOnce(path, "*.sqlite", "*.sqlite-journal", "*.sqlite-wal", "*.key")
}

// appendLinesOnce adds each line to a file if it is not already there.
func appendLinesOnce(path string, lines ...string) error {
	for _, line := range lines {
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), line) {
			continue
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(file, line)
		file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// recoverMerge aborts a merge a previous pass left unresolved; git refuses to
// commit or pull during one, so without this a single conflict wedges the sync
// loop permanently.
func recoverMerge(dir string) {
	if _, err := gitOut(dir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		_, _ = gitOut(dir, "merge", "--abort")
	}
}

// staged reports whether the log or attributes file has staged changes.
func stagedAny(dir string) bool {
	// git diff --cached --quiet exits non-zero when there are staged changes.
	return exec.Command("git", "-C", dir, "diff", "--cached", "--quiet").Run() != nil
}

func hasRemote(dir string) bool {
	out, err := gitOut(dir, "remote")
	return err == nil && strings.TrimSpace(out) != ""
}

func hasUpstream(dir string) bool {
	_, err := gitOut(dir, "rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
