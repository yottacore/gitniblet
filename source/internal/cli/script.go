// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

package cli

import (
	"fmt"
	"os"

	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/script"
)

// Script is the --script front-end: standard client bring-up (build, bootstrap,
// replay, seed), then run the script against the CRUD API through the scripting
// host. The host is an enterprise feature, so the open-source build (no host
// registered) rejects the flag before doing any work.
func Script(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: ngdb --script <script.lua> <ddl> <sqlite> <logdir>")
	}
	if !script.Available() {
		return fmt.Errorf("--script requires the enterprise build; the open-source build has no scripting host")
	}
	// This form spells the paths out rather than naming a registered database,
	// so name them here instead of indexing the arg list at every use.
	scriptFile, paths := args[0], dbPaths{DDL: args[1], SQLite: args[2], Log: args[3]}
	keyFile, pref := config.ResolveEncryption(paths.DDL, nil)
	c, err := schema.OpenClientWith(schema.OpenOpts{
		DDLPath: paths.DDL, DBPath: paths.SQLite, LogDir: paths.Log, KeyFile: keyFile, EncryptPref: pref,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	builtins, err := schema.Builtins()
	if err != nil {
		return err
	}
	attachWarns, err := script.Attach(c.API, paths.DDL, paths.Log, c.Schema, builtins)
	if err != nil {
		return err
	}
	for _, w := range append(c.Warnings, attachWarns...) {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return script.RunFile(c.API, scriptFile)
}
