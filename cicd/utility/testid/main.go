// SPDX-License-Identifier: MIT
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// testid keeps every CI test's ID: whole minutes since 2000-01-01 UTC in
// lowercase Crockford base 32. A Go test carries its ID as a "// ID: xxxxx"
// line right above its func. A bash test has it as the first argument of
// fCheck or fCase, or in its own "PASS xxxxx" line.
//
//	testid check <repo>    every Go test has an ID, and no ID is used twice
//	testid next [n]        the next n free IDs from now
//	testid ids <repo>      "<package> <test> <id>" for every Go test
//	testid report <repo>   go test -json on stdin -> one PASS/FAIL/SKIP line per test
//	testid remote <repo>   the same, from a test box's framed -test.v=test2json output
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const usage = "usage: testid check <repo> | next [n] | ids <repo> | report <repo> | remote <repo>"

const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

var (
	epoch    = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	idRe     = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{5,}$`)
	idLineRe = regexp.MustCompile(`^// ID: (\S+)\s*$`)
	bashIDRe = regexp.MustCompile(`\b(?:fCheck|fCase|PASS) ([0-9a-hjkmnp-tv-z]{5}) `)
	// Folders holding Go tests, relative to the repo.
	goRoots = []string{"source", "cicd/utility/testid"}
)

func encode(minutes int64) string {
	if minutes <= 0 {
		return "0"
	}
	var b []byte
	for ; minutes > 0; minutes /= 32 {
		b = append([]byte{alphabet[minutes%32]}, b...)
	}
	return string(b)
}

func decode(id string) (int64, bool) {
	var n int64
	for _, c := range id {
		i := strings.IndexRune(alphabet, c)
		if i < 0 {
			return 0, false
		}
		n = n*32 + int64(i)
	}
	return n, id != ""
}

func minuteOf(t time.Time) int64 { return int64(t.Sub(epoch) / time.Minute) }

type goTest struct {
	pkg, name, id, pos string
}

// isTestName follows the testing package: TestXxx or FuzzXxx, where Xxx
// doesn't start with a lowercase letter.
func isTestName(name string) bool {
	for _, prefix := range []string{"Test", "Fuzz"} {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			if rest == "" {
				return true
			}
			r, _ := utf8.DecodeRuneInString(rest)
			return !unicode.IsLower(r)
		}
	}
	return false
}

func modulePath(dir string) (string, string, error) {
	for d := dir; ; d = filepath.Dir(d) {
		raw, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for _, ln := range strings.Split(string(raw), "\n") {
				if mod, ok := strings.CutPrefix(strings.TrimSpace(ln), "module "); ok {
					return strings.TrimSpace(mod), d, nil
				}
			}
			return "", "", fmt.Errorf("%s/go.mod: no module line", d)
		}
		if d == filepath.Dir(d) {
			return "", "", fmt.Errorf("no go.mod above %s", dir)
		}
	}
}

func relTo(repo, path string) string {
	if rel, err := filepath.Rel(repo, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}

func scanGo(repo string) ([]goTest, error) {
	var tests []goTest
	fset := token.NewFileSet()
	for _, root := range goRoots {
		err := filepath.WalkDir(filepath.Join(repo, root), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "vendor" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			mod, modDir, err := modulePath(filepath.Dir(path))
			if err != nil {
				return err
			}
			pkg := mod
			if rel, _ := filepath.Rel(modDir, filepath.Dir(path)); rel != "." {
				pkg += "/" + filepath.ToSlash(rel)
			}
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" || !isTestName(fn.Name.Name) || fn.Type.Params.NumFields() != 1 {
					continue
				}
				at := fset.Position(fn.Pos())
				t := goTest{pkg: pkg, name: fn.Name.Name, pos: fmt.Sprintf("%s:%d:%d", relTo(repo, path), at.Line, at.Column)}
				if fn.Doc != nil {
					last := fn.Doc.List[len(fn.Doc.List)-1].Text
					if m := idLineRe.FindStringSubmatch(last); m != nil {
						t.id = m[1]
					}
				}
				tests = append(tests, t)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return tests, nil
}

// scanBash returns each bash test ID with the files that use it, once per file.
func scanBash(repo string) (map[string][]string, error) {
	ids := map[string][]string{}
	err := filepath.WalkDir(filepath.Join(repo, "cicd"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".bash") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, m := range bashIDRe.FindAllStringSubmatch(string(raw), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				ids[m[1]] = append(ids[m[1]], relTo(repo, path))
			}
		}
		return nil
	})
	return ids, err
}

// nextFree gives n IDs from now on that nothing holds yet.
func nextFree(taken map[string]bool, now time.Time, n int) []string {
	used := map[int64]bool{}
	for id := range taken {
		if m, ok := decode(id); ok {
			used[m] = true
		}
	}
	var out []string
	for m := minuteOf(now); len(out) < n; m++ {
		if !used[m] {
			out = append(out, encode(m))
		}
	}
	return out
}

func allIDs(repo string) ([]goTest, map[string][]string, error) {
	tests, err := scanGo(repo)
	if err != nil {
		return nil, nil, err
	}
	where, err := scanBash(repo)
	if err != nil {
		return nil, nil, err
	}
	for _, t := range tests {
		if t.id != "" {
			where[t.id] = append(where[t.id], t.pos)
		}
	}
	return tests, where, nil
}

func check(repo string, out io.Writer) bool {
	tests, where, err := allIDs(repo)
	if err != nil {
		fmt.Fprintln(out, "testid:", err)
		return false
	}
	good := true
	var missing []goTest
	for _, t := range tests {
		switch {
		case t.id == "":
			missing = append(missing, t)
		case !idRe.MatchString(t.id):
			fmt.Fprintf(out, "%s: %s has a bad ID %q\n", t.pos, t.name, t.id)
			good = false
		}
	}
	ids := make([]string, 0, len(where))
	for id := range where {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if len(where[id]) > 1 {
			fmt.Fprintf(out, "ID %s is used %d times: %s\n", id, len(where[id]), strings.Join(where[id], ", "))
			good = false
		}
	}
	if len(missing) > 0 {
		taken := map[string]bool{}
		for id := range where {
			taken[id] = true
		}
		free := nextFree(taken, time.Now(), len(missing))
		for i, t := range missing {
			fmt.Fprintf(out, "%s: %s has no ID. Add \"// ID: %s\" right above it.\n", t.pos, t.name, free[i])
		}
		good = false
	}
	if good {
		fmt.Fprintf(out, "testid: %d Go tests and %d IDs in all, each unique\n", len(tests), len(where))
	}
	return good
}

type event struct {
	Action     string
	Package    string
	ImportPath string
	Test       string
	Output     string
}

func shortPkg(pkg, mainMod string) string {
	if rest, ok := strings.CutPrefix(pkg, mainMod+"/"); ok && mainMod != "" {
		return rest
	}
	return pkg
}

type tally struct{ pass, fail, skip int }

var skipWhy = regexp.MustCompile(`^\s+\S+\.go:\d+: (.+)$`)

// reporter prints one line per top-level test from go test -json events. A
// failed test's output follows its line, and a failed package's own output
// follows it, so nothing go test would have shown is lost.
type reporter struct {
	out       io.Writer
	ids       map[string]string
	mainMod   string
	n         tally
	pkgFailed bool
	logs      map[string][]string
}

func newReporter(repo string, out io.Writer) (*reporter, error) {
	tests, err := scanGo(repo)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]string, len(tests))
	for _, t := range tests {
		ids[t.pkg+" "+t.name] = t.id
	}
	mainMod, _, _ := modulePath(filepath.Join(repo, "source"))
	return &reporter{out: out, ids: ids, mainMod: mainMod, logs: map[string][]string{}}, nil
}

// line takes one line of go test -json. One that isn't JSON is printed as is.
func (r *reporter) line(b []byte) {
	var ev event
	if json.Unmarshal(b, &ev) != nil {
		fmt.Fprintln(r.out, string(b))
		return
	}
	r.event(ev)
}

func (r *reporter) event(ev event) {
	out := r.out
	top, _, _ := strings.Cut(ev.Test, "/")
	key := ev.Package + " " + top
	switch {
	case ev.Action == "build-output":
		fmt.Fprint(out, ev.Output)
	case ev.Action == "output":
		r.logs[key] = append(r.logs[key], ev.Output)
	case ev.Test == "" && ev.Action == "fail":
		r.pkgFailed = true
		fmt.Fprintf(out, "FAIL ----- %s\n", shortPkg(ev.Package, r.mainMod))
		for _, ln := range r.logs[key] {
			fmt.Fprint(out, "\t", ln)
		}
		delete(r.logs, key)
	case ev.Test == "" && (ev.Action == "pass" || ev.Action == "skip"):
		delete(r.logs, key)
	case ev.Test != top || (ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip"):
	default:
		id := r.ids[key]
		if id == "" {
			id = "-----"
		}
		name := shortPkg(ev.Package, r.mainMod) + " " + top
		switch ev.Action {
		case "pass":
			r.n.pass++
			fmt.Fprintf(out, "PASS %s %s\n", id, name)
		case "skip":
			r.n.skip++
			why := ""
			for _, ln := range r.logs[key] {
				if m := skipWhy.FindStringSubmatch(strings.TrimRight(ln, "\n")); m != nil {
					why = ", " + m[1]
				}
			}
			fmt.Fprintf(out, "SKIP %s %s%s\n", id, name, why)
		case "fail":
			r.n.fail++
			fmt.Fprintf(out, "FAIL %s %s\n", id, name)
			for _, ln := range r.logs[key] {
				fmt.Fprint(out, "\t", ln)
			}
		}
		delete(r.logs, key)
	}
}

// done prints the count, and says whether everything passed.
func (r *reporter) done() bool {
	fmt.Fprintf(r.out, "%d passed, %d failed, %d skipped\n", r.n.pass, r.n.fail, r.n.skip)
	return r.n.fail == 0 && !r.pkgFailed
}

func newScanner(in io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return sc
}

// report reads go test -json.
func report(repo string, in io.Reader, out io.Writer) (bool, error) {
	r, err := newReporter(repo, out)
	if err != nil {
		return false, err
	}
	sc := newScanner(in)
	for sc.Scan() {
		r.line(sc.Bytes())
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	return r.done(), nil
}

// A test box has no Go, so it runs each test binary with -test.v=test2json and
// frames its output with these lines. The end line may follow output that had
// no newline of its own.
const (
	beginMark = "@@ngdb-test begin " // + import path
	endMark   = "@@ngdb-test end "   // + the binary's exit code
)

// test2json is go tool test2json -p pkg, over one binary's raw output.
func test2json(pkg string, raw []byte) ([]byte, error) {
	cmd := exec.Command("go", "tool", "test2json", "-p", pkg)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// remote reads a test box's output: plain lines, printed as they are, and
// each test binary's framed output, which becomes report's lines. The exit
// code settles what the output can't: a test still running when the binary
// died fails, and so does its package.
func remote(repo string, in io.Reader, out io.Writer) (bool, error) {
	r, err := newReporter(repo, out)
	if err != nil {
		return false, err
	}
	pkg, open := "", false
	var raw bytes.Buffer
	settle := func(rc string) error {
		js, err := test2json(pkg, raw.Bytes())
		if err != nil {
			return fmt.Errorf("test2json for %s: %w", pkg, err)
		}
		running := map[string]bool{}
		var order []string
		pkgDone := false
		sc := newScanner(bytes.NewReader(js))
		for sc.Scan() {
			var ev event
			if json.Unmarshal(sc.Bytes(), &ev) != nil {
				continue
			}
			switch {
			case ev.Test == "" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip"):
				pkgDone = true
			case ev.Test != "" && !strings.Contains(ev.Test, "/") && ev.Action == "run":
				running[ev.Test] = true
				order = append(order, ev.Test)
			case ev.Test != "" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip"):
				delete(running, ev.Test)
			}
			if rc != "0" && ev.Test == "" && ev.Action == "pass" {
				ev.Action = "fail"
			}
			r.event(ev)
		}
		if rc == "0" {
			if !pkgDone {
				r.event(event{Action: "pass", Package: pkg})
			}
			return sc.Err()
		}
		for _, name := range order {
			if running[name] {
				r.event(event{Action: "fail", Package: pkg, Test: name})
			}
		}
		if !pkgDone {
			if rc == "" {
				r.event(event{Action: "output", Package: pkg, Output: "no end line from the box; its run stopped here\n"})
			} else {
				r.event(event{Action: "output", Package: pkg, Output: "exit code " + rc + "\n"})
			}
			r.event(event{Action: "fail", Package: pkg})
		}
		return sc.Err()
	}
	sc := newScanner(in)
	for sc.Scan() {
		ln := strings.TrimSuffix(sc.Text(), "\r")
		if open {
			if i := strings.Index(ln, endMark); i >= 0 {
				if i > 0 {
					raw.WriteString(ln[:i])
					raw.WriteByte('\n')
				}
				open = false
				if err := settle(strings.TrimSpace(ln[i+len(endMark):])); err != nil {
					return false, err
				}
				continue
			}
			if p, ok := strings.CutPrefix(ln, beginMark); ok {
				if err := settle(""); err != nil {
					return false, err
				}
				pkg = strings.TrimSpace(p)
				raw.Reset()
				continue
			}
			raw.WriteString(ln)
			raw.WriteByte('\n')
			continue
		}
		if p, ok := strings.CutPrefix(ln, beginMark); ok {
			pkg, open = strings.TrimSpace(p), true
			raw.Reset()
			continue
		}
		fmt.Fprintln(out, ln)
	}
	if err := sc.Err(); err != nil {
		return false, err
	}
	if open {
		if err := settle(""); err != nil {
			return false, err
		}
	}
	return r.done(), nil
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	switch {
	case args[0] == "check" && len(args) == 2:
		if !check(args[1], os.Stdout) {
			os.Exit(1)
		}
	case args[0] == "next" && len(args) <= 2:
		count := 1
		if len(args) == 2 {
			if _, err := fmt.Sscan(args[1], &count); err != nil || count < 1 {
				fmt.Fprintln(os.Stderr, "testid: next takes a count of 1 or more")
				os.Exit(2)
			}
		}
		repo, _ := os.Getwd()
		for d := repo; d != filepath.Dir(d); d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "cicd", "test.bash")); err == nil {
				repo = d
				break
			}
		}
		_, where, err := allIDs(repo)
		if err != nil {
			fmt.Fprintln(os.Stderr, "testid:", err)
			os.Exit(1)
		}
		taken := map[string]bool{}
		for id := range where {
			taken[id] = true
		}
		for _, id := range nextFree(taken, time.Now(), count) {
			fmt.Println(id)
		}
	case args[0] == "ids" && len(args) == 2:
		tests, err := scanGo(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "testid:", err)
			os.Exit(1)
		}
		for _, t := range tests {
			fmt.Println(t.pkg, t.name, t.id)
		}
	case (args[0] == "report" || args[0] == "remote") && len(args) == 2:
		read := report
		if args[0] == "remote" {
			read = remote
		}
		ok, err := read(args[1], os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "testid:", err)
			os.Exit(1)
		}
		if !ok {
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}
