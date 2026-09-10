// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// Package app is the shared entrypoint behind every build's main: it parses the
// global flag prefixes and dispatches to the right front-end. main stays a thin
// wrapper so an external build (e.g. a superset that links extra features) can
// reuse the exact same dispatch by calling app.Run instead of duplicating it.
package app

import (
	"fmt"
	"strings"

	"github.com/jim-collier/nano-git-db/donate"
	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/cli"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/tui"
	"github.com/jim-collier/nano-git-db/internal/web"
)

// Version is the authoritative release version, bumped here in source before
// cutting a release (the release tag and goreleaser both derive from it).
var Version = "1.0.0-beta.2"

// Build is extra provenance (short commit + dirty flag) stamped by cicd/build.bash
// via -ldflags -X; empty for a plain `go build`, so a release binary reads clean.
var Build = ""

// Run dispatches the given args (os.Args[1:]) to a front-end and returns its
// error, if any.
func Run(args []string) error {
	// --version: print the build version and stop. A concise line the CI/CD
	// pipeline can run to confirm the binary executes, without opening the TUI.
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-v") {
		line := "ngdb " + Version
		if Build != "" {
			line += " (" + Build + ")"
		}
		fmt.Println(line)
		return nil
	}
	// --help: the usage block on stdout, exit 0. Anything unrecognized still
	// prints it, but on stderr and with a failing status.
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return cli.Help()
	}
	// --config[=path]: a global prefix that redirects the database registry;
	// strip it, apply the override, then dispatch the remaining args normally.
	if len(args) > 0 {
		if val, ok := splitFlag(args[0], "--config"); ok {
			rest := args[1:]
			if val == "" { // "--config <path>" form
				if len(rest) == 0 {
					return fmt.Errorf("--config needs a directory path")
				}
				val, rest = rest[0], rest[1:]
			}
			config.SetBaseOverride(val)
			return Run(rest)
		}
	}
	// --encrypt[=on|off|auto]: a global prefix setting the local encryption
	// preference for this run (bare --encrypt means on). Encryption is an
	// enterprise feature, so the open-source build (no provider registered)
	// rejects the flag outright rather than accepting a preference it cannot act
	// on.
	if len(args) > 0 {
		if val, ok := splitFlag(args[0], "--encrypt"); ok {
			if !enc.Available() {
				return fmt.Errorf("--encrypt requires the enterprise build; the open-source build cannot encrypt")
			}
			if val == "" {
				val = "on"
			}
			pref, valid := config.NormEncrypt(val)
			if !valid {
				return fmt.Errorf("--encrypt: %q is not on|off|auto", val)
			}
			config.SetEncryptPref(pref)
			return Run(args[1:])
		}
	}

	mode := ""
	if len(args) > 0 {
		mode = args[0]
	}
	// --init[=path]: register a database from a $PWD .shcl (its own mode, not a
	// data verb).
	if val, ok := splitFlag(mode, "--init"); ok {
		rest := args[1:]
		if val != "" {
			rest = append([]string{val}, rest...)
		}
		return cli.Init(rest)
	}
	switch mode {
	case "--donate":
		// Donate is an open-source-only feature; the enterprise build disables it
		// (donate.Enabled = false), so there the flag just falls through to usage.
		if donate.Enabled {
			return cli.Donate()
		}
		return cli.Run(args)
	case "--tui":
		return tui.Run(args[1:])
	case "--serve":
		return web.Run(args[1:])
	case "--script":
		return cli.Script(args[1:])
	case "":
		// No args at all: the interactive database picker (or a $PWD DDL). The
		// picker is a TUI screen, so this routes there rather than to the CLI.
		return tui.Run(nil)
	default:
		return cli.Run(args)
	}
}

// splitFlag matches a flag given either as "name" or "name=value".
func splitFlag(arg, name string) (value string, ok bool) {
	if arg == name {
		return "", true
	}
	if strings.HasPrefix(arg, name+"=") {
		return arg[len(name)+1:], true
	}
	return "", false
}
