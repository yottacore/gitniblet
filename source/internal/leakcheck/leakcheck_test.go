// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package leakcheck

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const envLeak = "GITNIBLET_LEAKCHECK_LEAK"

func TestMain(m *testing.M) { os.Exit(Run(m)) }

var neverClosed = make(chan struct{})

// Leaks on purpose, run only as the child of TestLeakFailsTheRun.
// ID: defqa
func TestLeaksAGoroutine(t *testing.T) {
	if os.Getenv(envLeak) != "1" {
		t.Skip("only as a child of TestLeakFailsTheRun")
	}
	go func() { <-neverClosed }()
}

// ID: defq9
func TestLeakFailsTheRun(t *testing.T) {
	cases := []struct {
		name, leak, check string
		fails             bool
	}{
		{"leak", "1", "1", true},
		{"no leak", "0", "1", false},
		{"leak, check off", "1", "0", false},
	}
	for _, c := range cases {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLeaksAGoroutine$", "-test.count=1")
		cmd.Env = append(os.Environ(), envLeak+"="+c.leak, EnvOff+"="+c.check)
		out, err := cmd.CombinedOutput()
		failed := err != nil
		if failed != c.fails || (c.fails && !strings.Contains(string(out), "goleak")) {
			t.Errorf("%s: failed=%v, want %v\n%s", c.name, failed, c.fails, out)
		}
	}
}
