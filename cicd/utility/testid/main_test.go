// SPDX-License-Identifier: MIT
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ID: dedqq
func TestEncode(t *testing.T) {
	m := minuteOf(time.Date(2026, 10, 24, 22, 0, 0, 0, time.UTC))
	if got := encode(m); got != "decp8" {
		t.Fatalf("2026-10-24 22:00 UTC gave %q, want decp8", got)
	}
	if back, ok := decode("decp8"); !ok || back != m {
		t.Fatalf("decode gave %d %v, want %d", back, ok, m)
	}
	if _, ok := decode("decpu"); ok {
		t.Fatal("u isn't Crockford, but decoded")
	}
	for _, name := range []string{"Test", "TestX", "Fuzz_x", "Test2"} {
		if !isTestName(name) {
			t.Errorf("%s should be a test", name)
		}
	}
	for _, name := range []string{"Testx", "Fuzzy", "Example", "testX"} {
		if isTestName(name) {
			t.Errorf("%s isn't a test", name)
		}
	}
}

// ID: dedqr
func TestNextFreeSkipsTaken(t *testing.T) {
	now := time.Date(2026, 10, 24, 22, 0, 30, 0, time.UTC)
	got := nextFree(map[string]bool{"decp8": true, "decpa": true}, now, 3)
	if strings.Join(got, " ") != "decp9 decpb decpc" {
		t.Fatalf("got %v", got)
	}
}

func fakeRepo(t *testing.T, goSrc, bashSrc string) string {
	t.Helper()
	repo := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("source/go.mod", "module example.com/m\n\ngo 1.25.0\n")
	write("source/a/a_test.go", goSrc)
	write("cicd/utility/x-test.bash", bashSrc)
	write("cicd/utility/testid/go.mod", "module gitniblet-testid\n")
	return repo
}

// ID: dedqs
func TestCheck(t *testing.T) {
	src := "package a\n\nimport \"testing\"\n\n// Doc line.\n// ID: decp8\nfunc TestOne(t *testing.T) {}\n\n" +
		"func TestNoID(t *testing.T) {}\n\n// ID: dECP9\nfunc FuzzBad(f *testing.F) {}\n\nfunc TestMain(m *testing.M) {}\n\nfunc helper(t *testing.T) {}\n"
	var out strings.Builder
	if check(fakeRepo(t, src, "fCheck decpa \"x\" true\n"), &out) {
		t.Fatal("passed with a missing and a bad ID")
	}
	for _, want := range []string{"TestNoID has no ID", `FuzzBad has a bad ID "dECP9"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no %q in:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "TestMain") || strings.Contains(out.String(), "helper") {
		t.Errorf("TestMain or a helper was taken for a test:\n%s", out.String())
	}

	out.Reset()
	src = "package a\n\nimport \"testing\"\n\n// ID: decp8\nfunc TestOne(t *testing.T) {}\n"
	if check(fakeRepo(t, src, "fCase decp8 \"x\"\necho \"PASS decp8 x\"\n"), &out) {
		t.Fatal("passed with one ID in Go and bash")
	}
	if !strings.Contains(out.String(), "ID decp8 is used 2 times") {
		t.Errorf("clash not named:\n%s", out.String())
	}

	out.Reset()
	if !check(fakeRepo(t, src, "fCheck decpa \"x\" true\n"), &out) {
		t.Fatalf("failed a clean tree:\n%s", out.String())
	}
}

// ID: dedqt
func TestReport(t *testing.T) {
	src := "package a\n\nimport \"testing\"\n\n// ID: decp8\nfunc TestOk(t *testing.T) {}\n\n// ID: decp9\nfunc TestBad(t *testing.T) {}\n\n// ID: decpa\nfunc TestSkip(t *testing.T) {}\n"
	repo := fakeRepo(t, src, "")
	jsonIn := strings.Join([]string{
		`{"Action":"run","Package":"example.com/m/a","Test":"TestOk"}`,
		`{"Action":"pass","Package":"example.com/m/a","Test":"TestOk/sub"}`,
		`{"Action":"pass","Package":"example.com/m/a","Test":"TestOk"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestBad/sub","Output":"    a_test.go:9: boom\n"}`,
		`{"Action":"fail","Package":"example.com/m/a","Test":"TestBad"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestSkip","Output":"    a_test.go:12: needs b29w\n"}`,
		`{"Action":"output","Package":"example.com/m/a","Test":"TestSkip","Output":"--- SKIP: TestSkip (0.00s)\n"}`,
		`{"Action":"skip","Package":"example.com/m/a","Test":"TestSkip"}`,
		`{"Action":"pass","Package":"example.com/m/a","Test":"TestNew"}`,
		`{"Action":"fail","Package":"example.com/m/a"}`,
		`{"ImportPath":"example.com/m/b","Action":"build-output","Output":"b.go:1: syntax error\n"}`,
	}, "\n")
	var out strings.Builder
	ok, err := report(repo, strings.NewReader(jsonIn), &out)
	if err != nil || ok {
		t.Fatalf("ok %v, err %v", ok, err)
	}
	want := "PASS decp8 a TestOk\nFAIL decp9 a TestBad\n\t    a_test.go:9: boom\nSKIP decpa a TestSkip, needs b29w\n" +
		"PASS ----- a TestNew\nFAIL ----- a\nb.go:1: syntax error\n2 passed, 1 failed, 1 skipped\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	ok, err = report(repo, strings.NewReader(jsonIn[:strings.Index(jsonIn, `{"Action":"output","Package":"example.com/m/a","Test":"TestBad/sub"`)]), &out)
	if err != nil || !ok {
		t.Fatalf("an all-pass run failed: %v\n%s", err, out.String())
	}
}

// ID: dedr3
func TestRemote(t *testing.T) {
	src := "package a\n\nimport \"testing\"\n\n// ID: decp8\nfunc TestOk(t *testing.T) {}\n\n// ID: decp9\nfunc TestBad(t *testing.T) {}\n\n// ID: decpa\nfunc TestSkip(t *testing.T) {}\n\n// ID: decpb\nfunc TestPanic(t *testing.T) {}\n"
	repo := fakeRepo(t, src, "")
	const v = "\x16"
	run := func(name string, lines ...string) []string {
		return append([]string{v + "=== RUN   " + name}, lines...)
	}
	var in []string
	in = append(in, "FreeBSD amd64 on box: 4 cores, load 0.1, 2 jobs", beginMark+"example.com/m/a")
	in = append(in, run("TestOk", v+"--- PASS: TestOk (0.00s)")...)
	in = append(in, run("TestBad", "    a_test.go:9: boom", v+"--- FAIL: TestBad (0.00s)")...)
	in = append(in, run("TestSkip", "    a_test.go:12: needs b29w", v+"--- SKIP: TestSkip (0.00s)")...)
	in = append(in, v+"FAIL\r", endMark+"1")
	// A panic leaves its test with no result, and the package with none.
	in = append(in, beginMark+"example.com/m/a")
	in = append(in, run("TestOk", v+"--- PASS: TestOk (0.00s)")...)
	in = append(in, run("TestPanic", "panic: kaput")...)
	in = append(in, "no newline here"+endMark+"2", "PASS smoke-cli")
	in = append(in, beginMark+"example.com/m/a")
	in = append(in, run("TestOk", v+"--- PASS: TestOk (0.00s)")...)
	in = append(in, v+"PASS", endMark+"0")

	var out strings.Builder
	ok, err := remote(repo, strings.NewReader(strings.Join(in, "\n")+"\n"), &out)
	if err != nil || ok {
		t.Fatalf("ok %v, err %v\n%s", ok, err, out.String())
	}
	want := "FreeBSD amd64 on box: 4 cores, load 0.1, 2 jobs\n" +
		"PASS decp8 a TestOk\nFAIL decp9 a TestBad\n\t=== RUN   TestBad\n\t    a_test.go:9: boom\n\t--- FAIL: TestBad (0.00s)\n" +
		"SKIP decpa a TestSkip, needs b29w\nFAIL ----- a\n\tFAIL\n" +
		"PASS decp8 a TestOk\nFAIL decpb a TestPanic\n\t=== RUN   TestPanic\n\tpanic: kaput\n\tno newline here\nFAIL ----- a\n\texit code 2\n" +
		"PASS smoke-cli\n" +
		"PASS decp8 a TestOk\n" +
		"3 passed, 2 failed, 1 skipped\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	// The box went away mid-binary: what came is kept, and the package fails.
	out.Reset()
	cut := strings.Join(append([]string{beginMark + "example.com/m/a"}, run("TestOk", v+"--- PASS: TestOk (0.00s)")...), "\n") + "\n" + v + "=== RUN   TestBad\n"
	ok, err = remote(repo, strings.NewReader(cut), &out)
	if err != nil || ok {
		t.Fatalf("a cut-off run passed: %v\n%s", err, out.String())
	}
	for _, s := range []string{"PASS decp8 a TestOk", "FAIL decp9 a TestBad", "FAIL ----- a\n\tno end line from the box"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("no %q in:\n%s", s, out.String())
		}
	}

	out.Reset()
	allPass := strings.Join(append(append([]string{beginMark + "example.com/m/a"}, run("TestOk", v+"--- PASS: TestOk (0.00s)")...), v+"PASS", endMark+"0"), "\n")
	if ok, err = remote(repo, strings.NewReader(allPass), &out); err != nil || !ok {
		t.Fatalf("an all-pass run failed: %v\n%s", err, out.String())
	}
}
