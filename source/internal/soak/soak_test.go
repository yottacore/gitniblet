// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package soak

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yottacore/gitniblet/internal/core/schema"
	"github.com/yottacore/gitniblet/internal/core/txlog"
	"github.com/yottacore/gitniblet/internal/leakcheck"
)

const (
	envTime = "GITNIBLET_SOAK"      // how long to run, as a Go duration; unset skips
	envLeak = "GITNIBLET_SOAK_LEAK" // goroutine or heap, only in TestSoakCatchesLeaks' children

	warmup       = 5  // cycles before the first sample, while caches and pools fill
	minCycles    = 30 // sampled cycles, however short the time
	perWorkspace = 40 // cycles on one pair of clones, so each replay stays short
	heapSlack    = 1 << 20
)

const ddlText = "tables:\n" +
	"\ttable: person\n" +
	"\t\tfields:\n" +
	"\t\t\tfield: name\n" +
	"\t\t\t\ttype: string\n" +
	"\t\t\tfield: note\n" +
	"\t\t\t\ttype: string\n"

func TestMain(m *testing.M) { os.Exit(leakcheck.Run(m)) }

// Held across cycles by the heap leak put in on purpose.
var kept [][]byte

// Two clones of one remote, written to in turn, as two users would.
type workspace struct {
	dir, ddl string
	logs     [2]string
}

// ID: dehwn
func TestSoak(t *testing.T) {
	budget := soakTime(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Sync commits with whatever git config it finds, so keep the box's out.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	base := t.TempDir()

	var ws workspace
	baseG := runtime.NumGoroutine()
	var heap, gor []int64
	start := time.Now()
	for n := 0; n < warmup+minCycles || time.Since(start) < budget; n++ {
		if n%perWorkspace == 0 {
			if ws.dir != "" {
				if err := os.RemoveAll(ws.dir); err != nil {
					t.Fatal(err)
				}
			}
			ws = newWorkspace(t, filepath.Join(base, fmt.Sprint(n)))
		}
		cycle(t, ws, n)
		if n < warmup {
			continue
		}
		h, g := measure(baseG)
		heap, gor = append(heap, h), append(gor, g)
	}

	hLo, gLo := lows(heap), lows(gor)
	t.Logf("%d cycles in %s. Low points by thirds: heap in use %d, %d, %d KiB; goroutines %d, %d, %d",
		len(heap)+warmup, time.Since(start).Round(time.Second), hLo[0]>>10, hLo[1]>>10, hLo[2]>>10, gLo[0], gLo[1], gLo[2])
	if climbing(heap, max(heapSlack, hLo[0]/10)) {
		t.Errorf("heap in use kept climbing: %d, %d, %d KiB", hLo[0]>>10, hLo[1]>>10, hLo[2]>>10)
	}
	if climbing(gor, 0) {
		t.Errorf("goroutines kept climbing: %d, %d, %d", gLo[0], gLo[1], gLo[2])
	}
}

// Watches TestSoak fail with a leak put in on purpose. It runs only in the
// Soak test step, since each child takes a while.
// ID: dehwp
func TestSoakCatchesLeaks(t *testing.T) {
	soakTime(t)
	if os.Getenv(envLeak) != "" {
		t.Skip("a child runs only TestSoak")
	}
	for leak, want := range map[string]string{"goroutine": "goroutines kept climbing", "heap": "heap in use kept climbing"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSoak$", "-test.count=1")
		cmd.Env = append(os.Environ(), envTime+"=1ns", envLeak+"="+leak, leakcheck.EnvOff+"=0")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), want) {
			t.Errorf("%s leak: want TestSoak to fail on it, got err=%v\n%s", leak, err, out)
		}
	}
}

// ID: dehwq
func TestClimbing(t *testing.T) {
	flat := []int64{10, 10, 10, 10, 10, 10, 10, 10, 10}
	cases := []struct {
		name  string
		vals  []int64
		slack int64
		want  bool
	}{
		{"flat", flat, 0, false},
		{"noisy but flat", []int64{10, 14, 10, 12, 10, 15, 11, 10, 13}, 0, false},
		{"one step up, then flat", []int64{10, 10, 10, 20, 20, 20, 20, 20, 20}, 0, false},
		{"steady climb", []int64{10, 11, 12, 13, 14, 15, 16, 17, 18}, 0, true},
		{"noisy climb", []int64{10, 30, 11, 14, 40, 13, 17, 16, 50}, 0, true},
		{"climb within slack", []int64{10, 11, 12, 13, 14, 15, 16, 17, 18}, 6, false},
		{"climb past slack", []int64{10, 11, 12, 13, 14, 15, 16, 17, 18}, 5, true},
		{"too few", []int64{1, 2}, 0, false},
		{"falling", []int64{18, 17, 16, 15, 14, 13, 12, 11, 10}, 0, false},
	}
	for _, c := range cases {
		if got := climbing(c.vals, c.slack); got != c.want {
			t.Errorf("%s: climbing(%v, %d) = %v, want %v", c.name, c.vals, c.slack, got, c.want)
		}
	}
}

func soakTime(t *testing.T) time.Duration {
	t.Helper()
	s := os.Getenv(envTime)
	if s == "" {
		t.Skip("set " + envTime + " to a time, such as 1m, to run it")
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("%s: %v", envTime, err)
	}
	return d
}

func newWorkspace(t *testing.T, dir string) workspace {
	t.Helper()
	ws := workspace{dir: dir, ddl: filepath.Join(dir, "s.ddl")}
	ws.logs = [2]string{filepath.Join(dir, "a"), filepath.Join(dir, "b")}
	remote := filepath.Join(dir, "remote.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ws.ddl, []byte(ddlText), 0o644); err != nil {
		t.Fatal(err)
	}
	// One commit on the remote first, so both clones have an upstream.
	gitrun(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	gitrun(t, dir, "init", "-q", "-b", "main", ws.logs[0])
	gitrun(t, ws.logs[0], "remote", "add", "origin", remote)
	identity(t, ws.logs[0])
	gitrun(t, ws.logs[0], "commit", "-q", "--allow-empty", "-m", "start")
	gitrun(t, ws.logs[0], "push", "-q", "-u", "origin", "main")
	gitrun(t, dir, "clone", "-q", remote, ws.logs[1])
	identity(t, ws.logs[1])
	return ws
}

func identity(t *testing.T, dir string) {
	t.Helper()
	gitrun(t, dir, "config", "user.email", "t@t")
	gitrun(t, dir, "config", "user.name", "t")
}

func gitrun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// One user's session: open, which replays the log, then write, sync, replay
// what the sync pulled, and close.
func cycle(t *testing.T, ws workspace, n int) {
	t.Helper()
	side := n % 2
	logDir := ws.logs[side]
	c, err := schema.OpenClient(ws.ddl, filepath.Join(ws.dir, fmt.Sprintf("%d.sqlite", side)), logDir, fmt.Sprint("u", side))
	if err != nil {
		t.Fatal(err)
	}
	stop := c.StartAutoSync(3600, nil)
	note := strings.Repeat("x", 200)
	ids := make([]string, 0, 3)
	for i := range 3 {
		id, err := c.API.Create("person", map[string]string{"name": fmt.Sprint("p", n, "-", i), "note": note})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := c.API.Update("person", ids[0], map[string]string{"name": fmt.Sprint("q", n)}); err != nil {
		t.Fatal(err)
	}
	if err := c.API.Delete("person", ids[1]); err != nil {
		t.Fatal(err)
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := txlog.NewSyncer(lg, time.Minute).Sync()
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		entries, _, err := lg.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.API.Replay(entries); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	switch os.Getenv(envLeak) {
	case "goroutine":
		return // the store's pool keeps a goroutine until Close
	case "heap":
		kept = append(kept, make([]byte, 256<<10))
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// Heap in use and the goroutine count after a cycle. Goroutines a Close
// stopped get a moment to exit, so only ones left running count.
func measure(baseG int) (heap, goroutines int64) {
	for deadline := time.Now().Add(200 * time.Millisecond); runtime.NumGoroutine() > baseG && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int64(ms.HeapInuse), int64(runtime.NumGoroutine())
}

// The low point of each third of vals. Low points, since a GC that came late
// or a pool that was full only lifts a sample.
func lows(vals []int64) [3]int64 {
	var lo [3]int64
	n := len(vals) / 3
	if n == 0 {
		return lo
	}
	for i := range lo {
		lo[i] = vals[i*n]
		for _, v := range vals[i*n : (i+1)*n] {
			lo[i] = min(lo[i], v)
		}
	}
	return lo
}

// Whether vals keep climbing: each third's low point is over the one before,
// and the last is more than slack over the first. A cache that fills once
// steps up and stays, which isn't a climb.
func climbing(vals []int64, slack int64) bool {
	if len(vals) < 3 {
		return false
	}
	lo := lows(vals)
	return lo[0] < lo[1] && lo[1] < lo[2] && lo[2]-lo[0] > slack
}
