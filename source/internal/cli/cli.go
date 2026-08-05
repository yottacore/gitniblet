// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package cli is the arg-based CRUD front-end. A data verb names one registered
// database (the name its config record was registered under, extension
// optional) and never spells out the ddl/sqlite/log paths - those live in the
// registry. The name is the first positional, or a --db/-d flag. The low-level
// schema/log verbs (build/replay/sync/gc) also take a name, and keep an
// explicit-path form for use before a database is registered. Writes stamp the
// user from NANOGITDB_USER, else the OS username.
package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jim-collier/nano-git-db/donate"
	"github.com/jim-collier/nano-git-db/enc"
	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/internal/core/store"
	"github.com/jim-collier/nano-git-db/internal/core/txlog"
	"github.com/jim-collier/nano-git-db/script"
)

// Run is the CLI entry point.
func Run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "ddl":
		if len(rest) >= 1 {
			return dumpDDL(rest[0])
		}
	case "build":
		return doBuild(rest)
	case "replay":
		return doReplay(rest)
	case "sync":
		return doSync(rest)
	case "gc":
		return doGC(rest)
	case "create", "get", "update", "setnull", "markdelete", "delete", "query",
		"comment", "comments", "attachuri", "attachfile", "attachments":
		return doData(verb, rest)
	case "--rename-table":
		return doRenameTable(rest)
	case "--rename-field":
		return doRenameField(rest)
	case "webuser":
		if len(rest) >= 1 {
			return webUser(rest[0])
		}
	}
	return usage()
}

// crudSelect pulls the leading database and (optional) table selectors off a
// data verb's args. Each can be given two ways: the database positionally
// (the first bare word) or as --db/-d, the table positionally (the next bare
// word) or as --table/-t. The explicit flags accept both "--db name" and
// "--db=name" and may lead in any order. Data verbs are name-only by design -
// the raw ddl/sqlite/log paths are never spelled out for CRUD.
//
// It returns the database's open triple, the table (empty when the verb reads
// it positionally, filled when a --table flag supplied it), and the remaining
// positional args (id, assignments, sql...).
func crudSelect(args []string) (paths []string, table string, rest []string, err error) {
	var name string
	// consume any leading --db/--table flags, in any order
selectors:
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--db" || arg == "-d":
			if len(args) < 2 {
				return nil, "", nil, fmt.Errorf("%s needs a database name", arg)
			}
			name, args = args[1], args[2:]
		case arg == "--table" || arg == "-t":
			if len(args) < 2 {
				return nil, "", nil, fmt.Errorf("%s needs a table name", arg)
			}
			table, args = args[1], args[2:]
		default:
			if val, ok := cutFlag(arg, "--db", "-d"); ok {
				name, args = val, args[1:]
			} else if val, ok := cutFlag(arg, "--table", "-t"); ok {
				table, args = val, args[1:]
			} else if name == "" && !strings.Contains(arg, "=") {
				name, args = arg, args[1:] // first bare word is the database name
			} else {
				break selectors // the rest are the verb's positionals
			}
		}
	}
	if name == "" {
		return nil, "", nil, fmt.Errorf("missing database name")
	}
	cfg := config.FindByName(name)
	if cfg == nil {
		return nil, "", nil, config.UnknownDatabase(name)
	}
	_ = cfg.Touch() // best-effort last-opened stamp
	return []string{cfg.DDLPath, cfg.SQLitePath, cfg.LogDir}, table, args, nil
}

// cutFlag matches "--db=value" / "-d=value" style selectors.
func cutFlag(arg string, names ...string) (value string, ok bool) {
	for _, name := range names {
		if v, found := strings.CutPrefix(arg, name+"="); found {
			return v, true
		}
	}
	return "", false
}

// nameTriple resolves a registered database name to its (ddl, sqlite, logdir)
// paths; ok is false when no such name is registered. The low-level verbs use
// it to accept a name yet still fall back to their explicit-path form.
func nameTriple(name string) (ddlPath, sqlitePath, logDir string, ok bool) {
	if cfg := config.FindByName(name); cfg != nil {
		return cfg.DDLPath, cfg.SQLitePath, cfg.LogDir, true
	}
	return "", "", "", false
}

// doData resolves the named database and dispatches one CRUD verb over it. The
// query verb takes SQL rather than a table; every other verb needs a table,
// taken from a --table flag or the next positional.
func doData(verb string, args []string) error {
	paths, table, rest, err := crudSelect(args)
	if err != nil {
		return err
	}
	if verb == "query" {
		if len(rest) < 1 {
			return fmt.Errorf("usage: ngdb query <db> <sql>")
		}
		return crudQuery(paths, rest[0])
	}
	// every other verb needs a table: the --table flag, else the next positional
	if table == "" {
		if len(rest) < 1 {
			return fmt.Errorf("usage: ngdb %s <db> <table> <args>", verb)
		}
		table, rest = rest[0], rest[1:]
	}
	need := func(n int, form string) error {
		if len(rest) < n {
			return fmt.Errorf("usage: ngdb %s <db> <table> %s", verb, form)
		}
		return nil
	}
	switch verb {
	case "create":
		return crudCreate(paths, table, rest)
	case "get":
		if err := need(1, "<id>"); err != nil {
			return err
		}
		return crudGet(paths, table, rest[0])
	case "update":
		if err := need(2, "<id> f=v [f=v...]"); err != nil {
			return err
		}
		return crudUpdate(paths, table, rest[0], rest[1:])
	case "setnull":
		if err := need(2, "<id> <field>"); err != nil {
			return err
		}
		return crudSetNull(paths, table, rest[0], rest[1])
	case "markdelete", "delete":
		if err := need(1, "<id>"); err != nil {
			return err
		}
		return crudDelete(verb, paths, table, rest[0])
	case "comment":
		if err := need(2, "<id> <text>"); err != nil {
			return err
		}
		return crudComment(paths, table, rest[0], strings.Join(rest[1:], " "))
	case "comments":
		if err := need(1, "<id>"); err != nil {
			return err
		}
		return crudComments(paths, table, rest[0])
	case "attachuri":
		if err := need(2, "<id> <uri> [desc]"); err != nil {
			return err
		}
		return crudAttach(paths, table, rest[0], rest[1], strings.Join(rest[2:], " "), false)
	case "attachfile":
		if err := need(2, "<id> <path> [desc]"); err != nil {
			return err
		}
		return crudAttach(paths, table, rest[0], rest[1], strings.Join(rest[2:], " "), true)
	case "attachments":
		if err := need(1, "<id>"); err != nil {
			return err
		}
		return crudAttachments(paths, table, rest[0])
	}
	return usage()
}

// doBuild builds/migrates a view: build <db>, or the explicit build <ddl> <sqlite>.
func doBuild(args []string) error {
	if len(args) == 1 {
		if ddlPath, sqlitePath, _, ok := nameTriple(args[0]); ok {
			return buildDB(ddlPath, sqlitePath)
		}
		return fmt.Errorf("unknown database %q (or use build <ddl> <sqlite>)", args[0])
	}
	if len(args) >= 2 {
		return buildDB(args[0], args[1])
	}
	return usage()
}

// doReplay rebuilds a view from its log: replay <db>, or replay <ddl> <sqlite> <dir>.
func doReplay(args []string) error {
	if len(args) == 1 {
		if ddlPath, sqlitePath, logDir, ok := nameTriple(args[0]); ok {
			return replay(ddlPath, sqlitePath, logDir)
		}
		return fmt.Errorf("unknown database %q (or use replay <ddl> <sqlite> <logdir>)", args[0])
	}
	if len(args) >= 3 {
		return replay(args[0], args[1], args[2])
	}
	return usage()
}

// doSync syncs the log via git: sync <db> also migrates + replays; sync <logdir>
// only reconciles the log; sync <ddl> <sqlite> <dir> is the explicit full cycle.
func doSync(args []string) error {
	if len(args) == 1 {
		if ddlPath, sqlitePath, logDir, ok := nameTriple(args[0]); ok {
			return syncAndReplay(ddlPath, sqlitePath, logDir)
		}
		return syncLog(args[0]) // not a name: treat it as a log directory
	}
	if len(args) >= 3 {
		return syncAndReplay(args[0], args[1], args[2])
	}
	return usage()
}

// doGC collects long-deleted entries: gc <db>, or gc <ddl> <logdir>.
func doGC(args []string) error {
	if len(args) == 1 {
		if ddlPath, _, logDir, ok := nameTriple(args[0]); ok {
			return gcLog(ddlPath, logDir)
		}
		return fmt.Errorf("unknown database %q (or use gc <ddl> <logdir>)", args[0])
	}
	if len(args) >= 2 {
		return gcLog(args[0], args[1])
	}
	return usage()
}

// doRenameTable: --rename-table <db> <old> <new>, or the explicit
// --rename-table <ddl> <sqlite> <old> <new>.
func doRenameTable(args []string) error {
	if len(args) == 3 {
		if ddlPath, sqlitePath, _, ok := nameTriple(args[0]); ok {
			return renameTable(ddlPath, sqlitePath, args[1], args[2])
		}
		return fmt.Errorf("unknown database %q (or use --rename-table <ddl> <sqlite> <old> <new>)", args[0])
	}
	if len(args) >= 4 {
		return renameTable(args[0], args[1], args[2], args[3])
	}
	return usage()
}

// doRenameField: --rename-field <db> <table> <old> <new>, or the explicit
// --rename-field <ddl> <sqlite> <table> <old> <new>.
func doRenameField(args []string) error {
	if len(args) == 4 {
		if ddlPath, sqlitePath, _, ok := nameTriple(args[0]); ok {
			return renameField(ddlPath, sqlitePath, args[1], args[2], args[3])
		}
		return fmt.Errorf("unknown database %q (or use --rename-field <ddl> <sqlite> <table> <old> <new>)", args[0])
	}
	if len(args) >= 5 {
		return renameField(args[0], args[1], args[2], args[3], args[4])
	}
	return usage()
}

// Help prints the usage block on stdout and succeeds - what an explicit --help
// asks for.
func Help() error {
	printUsage(os.Stdout)
	return nil
}

// usage reports a command that could not be run. The block goes to stderr so a
// script's stdout stays clean, and the error makes the exit status non-zero.
func usage() error {
	printUsage(os.Stderr)
	return fmt.Errorf("unknown or incomplete command")
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: ngdb <verb> ...")
	fmt.Fprintln(w, "  setup and run modes:")
	fmt.Fprintln(w, "    --init [repo-or-dir]           register the $PWD .shcl as a database")
	fmt.Fprintln(w, "    --config <dir> ...             use an alternate registry dir, then run")
	if enc.Available() { // enterprise build only
		fmt.Fprintln(w, "    --encrypt[=on|off|auto] ...    set the local encryption preference,")
		fmt.Fprintln(w, "                                   then run (--init --encrypt=on mints a key)")
	}
	fmt.Fprintln(w, "    --tui [<db>]                   terminal UI (no name: pick/create a db)")
	fmt.Fprintln(w, "    --serve [<db>]                 local web UI on 127.0.0.1:8765")
	fmt.Fprintln(w, "    webuser <username>             set a proxied-mode web login")
	fmt.Fprintln(w, "                                   (password from NGDB_WEB_PASSWORD or prompt)")
	fmt.Fprintln(w, "    --script <f.lua> <ddl> <sqlite> <dir>   run a Lua script")
	if donate.Enabled { // open-source-only feature
		fmt.Fprintln(w, "    --donate                       ways to support the project")
	}
	fmt.Fprintln(w, "    --version, -v                  print the version and exit")
	fmt.Fprintln(w, "  <db> is a registered database name (extension optional). Register one")
	fmt.Fprintln(w, "  with --init; run ngdb with no arguments to see the ones you have.")
	fmt.Fprintln(w, "  schema and log:")
	fmt.Fprintln(w, "    ddl <file>                     parse a DDL file and print a summary")
	fmt.Fprintln(w, "    build <db>                     build/migrate the SQLite view")
	fmt.Fprintln(w, "    replay <db>                    rebuild the view from the tx-log")
	fmt.Fprintln(w, "    sync <db>                      sync the log, then migrate and replay")
	fmt.Fprintln(w, "    gc <db>                         collect entries of long-deleted rows")
	fmt.Fprintln(w, "                                   (gc_age_days tunable, default 90)")
	fmt.Fprintln(w, "  data (each takes a <db>, then a <table>; e.g. create issues task f=v):")
	fmt.Fprintln(w, "    create <db> <table> f=v [f=v...]     insert a row; prints its id")
	fmt.Fprintln(w, "    get <db> <table> <id>                print one row")
	fmt.Fprintln(w, "    update <db> <table> <id> f=v [...]   set fields")
	fmt.Fprintln(w, "    setnull <db> <table> <id> <field>    set a field to SQL NULL")
	fmt.Fprintln(w, "    markdelete <db> <table> <id>         soft-delete")
	fmt.Fprintln(w, "    delete <db> <table> <id>             hard-delete")
	fmt.Fprintln(w, "    query <db> <sql>                     read-only SQL against the view")
	fmt.Fprintln(w, "    the db and table may also be given explicitly as flags in any order:")
	fmt.Fprintln(w, "      --db=<name> / -d <name>,  --table=<name> / -t <name>")
	fmt.Fprintln(w, "  schema ops (rewrite the DDL file and the SQLite view; the old name")
	fmt.Fprintln(w, "  becomes an alias so existing tx-log entries still replay):")
	fmt.Fprintln(w, "    --rename-table <db> <old> <new>")
	fmt.Fprintln(w, "    --rename-field <db> <table> <old> <new>")
	fmt.Fprintln(w, "  opt-in features (table must enable them in its DDL features: block):")
	fmt.Fprintln(w, "    comment <db> <table> <id> <text>           add a comment to a row")
	fmt.Fprintln(w, "    comments <db> <table> <id>                 list a row's comments")
	fmt.Fprintln(w, "    attachuri <db> <table> <id> <uri> [desc]   attach a link-in-place URI")
	fmt.Fprintln(w, "    attachfile <db> <table> <id> <path> [desc] copy a file in and attach it")
	fmt.Fprintln(w, "    attachments <db> <table> <id>              list a row's attachments")
	fmt.Fprintln(w, "  writes are stamped with NANOGITDB_USER (default: the OS username)")

}

// dumpDDL parses a schema file and prints a short summary of what it declares.
func dumpDDL(path string) error {
	sch, err := ddl.ParseFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("tables=%d relationships=%d views=%d\n", len(sch.Tables), len(sch.Relations), len(sch.Views))
	for _, t := range sch.Tables {
		fmt.Printf("  table %q: fields=%d uniques=%d indexes=%d\n",
			t.Name, len(t.Fields), len(t.Uniques), len(t.Indexes))
	}
	for _, w := range sch.Warnings {
		fmt.Println("  warning:", w)
	}
	return nil
}

// buildDB parses a DDL and builds (or updates) the SQLite view at dbPath.
func buildDB(ddlPath, dbPath string) error {
	sch, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Build(sch); err != nil {
		return err
	}
	bootWarns, err := schema.Bootstrap(st, sch)
	if err != nil {
		return err
	}
	fmt.Printf("built %d table(s) plus built-ins into %s\n", len(sch.Tables), dbPath)
	for _, w := range append(sch.Warnings, bootWarns...) {
		fmt.Println("  warning:", w)
	}
	fmt.Println("  note: default groups get seeded by replay/sync (they are log-first)")
	return nil
}

// replay rebuilds the SQLite view from a tx-log dir: build schema, then apply
// every logged entry. Demonstrates the core thesis - the log is the truth, the
// SQLite file is a derived, rebuildable view.
func replay(ddlPath, dbPath, logDir string) error {
	sch, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Build(sch); err != nil {
		return err
	}
	bootWarns, err := schema.Bootstrap(st, sch)
	if err != nil {
		return err
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		return err
	}
	entries, readWarns, err := lg.ReadAll()
	if err != nil {
		return err
	}
	builtins, err := schema.Builtins()
	if err != nil {
		return err
	}
	schema.ApplyAliases(entries, sch, builtins) // pre-rename entries -> current names
	// Decrypt field values before applying, using this DDL's registered key
	// (else a key beside the DDL); unreadable ones stay empty in the view.
	keyFile, _ := config.ResolveEncryptionForDDL(ddlPath)
	cipher, _, err := enc.LoadCipher(keyFile)
	if err != nil {
		return err
	}
	if n := crud.DecryptEntries(entries, cipher); n > 0 {
		fmt.Printf("  warning: %d encrypted value(s) unreadable (no/wrong key); shown empty\n", n)
	}
	warns, err := txlog.Apply(st, entries)
	if err != nil {
		return err
	}
	warns = append(readWarns, warns...)
	// Seed AFTER the replay: an already-seeded log has just filled groups, so
	// this is a no-op everywhere but on a brand-new database.
	api := crud.New(st, lg)
	api.UserID = "system"
	if err := schema.SeedDefaults(api); err != nil {
		return err
	}
	fmt.Printf("applied %d log entr%s into %s\n", len(entries), plural(len(entries)), dbPath)
	for _, w := range append(bootWarns, warns...) {
		fmt.Println("  warning:", w)
	}
	for _, t := range sch.Tables {
		var live int
		if err := st.DB().QueryRow(
			`SELECT COUNT(*) FROM ` + quoteIdent(t.Name) + ` WHERE "is_deleted"=0`).Scan(&live); err == nil {
			fmt.Printf("  %s: %d live row(s)\n", t.Name, live)
		}
	}
	return nil
}

func quoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// syncLog runs one git sync pass over a tx-log directory.
func syncLog(dir string) error {
	lg, err := txlog.Open(dir)
	if err != nil {
		return err
	}
	res, err := txlog.NewSyncer(lg, time.Minute).Sync()
	if err != nil {
		return err
	}
	switch {
	case res.Skipped:
		fmt.Println("not a git repo; nothing to sync (local logging still works)")
	case res.NoRemote && res.Committed:
		fmt.Println("committed locally; no remote configured")
	case res.NoRemote:
		fmt.Println("up to date; no remote configured")
	default:
		fmt.Printf("synced: committed=%v pulled=%v pushed=%v new=%v\n",
			res.Committed, res.Pulled, res.Pushed, res.Changed)
	}
	return nil
}

// syncAndReplay is the full client cycle: reconcile the log with the remote
// first (the DDL may live in the same repo and change too), then migrate the
// view schema additively, then rebuild the view from the merged log.
func syncAndReplay(ddlPath, dbPath, logDir string) error {
	if err := syncLog(logDir); err != nil {
		return err
	}
	return replay(ddlPath, dbPath, logDir)
}

// openAPI wires up the CRUD API for one data verb: parse the DDL, open and
// migrate the view (build + built-ins are idempotent, so the schema is always
// current), open the log. It does NOT replay - keeping the view current across
// clients is what replay/sync are for. Callers must Close the store.
func openAPI(paths []string) (*store.Store, *crud.API, error) {
	sch, err := ddl.ParseFile(paths[0])
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(paths[1])
	if err != nil {
		return nil, nil, err
	}
	if err := st.Build(sch); err != nil {
		st.Close()
		return nil, nil, err
	}
	if _, err := schema.Bootstrap(st, sch); err != nil {
		st.Close()
		return nil, nil, err
	}
	lg, err := txlog.Open(paths[2])
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	api := crud.New(st, lg)
	api.UserID = crud.DefaultUserID()
	builtins, err := schema.Builtins()
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	// Encryption for writes: find this DDL's registry record (its key lives in
	// the unsynced config dir), else fall back to a key beside the DDL.
	keyFile, pref := config.ResolveEncryptionForDDL(paths[0])
	cipher, _, err := enc.LoadCipher(keyFile)
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	api.EnableEncryption(cipher, pref, sch)
	api.EnableFeatures(sch, builtins)
	attachWarns, err := script.Attach(api, paths[0], paths[2], sch, builtins)
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	for _, w := range attachWarns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	applyGate(api, os.Stderr)
	return st, api, nil
}

// parseAssigns turns f=v args into a field map. An empty value (f=) is the
// empty string; SQL NULL has its own verb, so no value is magic here.
func parseAssigns(args []string) (map[string]string, error) {
	fields := make(map[string]string, len(args))
	for _, arg := range args {
		k, v, ok := strings.Cut(arg, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("expected field=value, got %q", arg)
		}
		fields[k] = v
	}
	return fields, nil
}

// gcLog collects tx-log entries of rows hard-deleted more than gc_age_days
// ago, via segment rotation (never editing a log file in place). The next
// sync commits the rotation.
func gcLog(ddlPath, logDir string) error {
	sch, err := ddl.ParseFile(ddlPath)
	if err != nil {
		return err
	}
	lg, err := txlog.Open(logDir)
	if err != nil {
		return err
	}
	entries, warns, err := lg.ReadAll()
	if err != nil {
		return err
	}
	for _, w := range warns {
		fmt.Println("warning:", w)
	}
	days := sch.TunableInt("gc_age_days", 90)
	keep, collected := txlog.GC(entries, txlog.CutoffDays(days))
	if collected == 0 {
		fmt.Printf("nothing to collect (threshold: deleted > %d days ago)\n", days)
		return nil
	}
	seg, err := lg.Rotate(keep)
	if err != nil {
		return err
	}
	fmt.Printf("collected %d entr%s (deleted > %d days ago); %d kept in %s\n",
		collected, plural(collected), days, len(keep), seg)
	if txlog.InRepo(logDir) {
		fmt.Println("run sync to publish the rotation")
	}
	return nil
}

// renameTable renames in the SQLite view first (data preserved; the log is
// the truth anyway, so a half-done rename is always recoverable by replay),
// then rewrites the DDL with the old name kept as an alias.
func renameTable(ddlPath, dbPath, oldName, newName string) error {
	src, err := os.ReadFile(ddlPath)
	if err != nil {
		return err
	}
	out, found := ddl.RenameTable(src, oldName, newName)
	if !found {
		return fmt.Errorf("no table %q in %s", oldName, ddlPath)
	}
	if err := alterView(dbPath, oldName,
		`ALTER TABLE `+quoteIdent(oldName)+` RENAME TO `+quoteIdent(newName)); err != nil {
		return err
	}
	if err := os.WriteFile(ddlPath, out, 0o644); err != nil {
		return fmt.Errorf("view renamed but DDL rewrite failed (rerun to finish): %w", err)
	}
	fmt.Printf("renamed table %s -> %s (old name kept as an alias)\n", oldName, newName)
	return nil
}

func renameField(ddlPath, dbPath, table, oldName, newName string) error {
	src, err := os.ReadFile(ddlPath)
	if err != nil {
		return err
	}
	out, found := ddl.RenameField(src, table, oldName, newName)
	if !found {
		return fmt.Errorf("no field %q in table %q in %s", oldName, table, ddlPath)
	}
	if err := alterView(dbPath, table,
		`ALTER TABLE `+quoteIdent(table)+` RENAME COLUMN `+quoteIdent(oldName)+` TO `+quoteIdent(newName)); err != nil {
		return err
	}
	if err := os.WriteFile(ddlPath, out, 0o644); err != nil {
		return fmt.Errorf("view renamed but DDL rewrite failed (rerun to finish): %w", err)
	}
	fmt.Printf("renamed field %s.%s -> %s (old name kept as an alias)\n", table, oldName, newName)
	return nil
}

// alterView runs one ALTER against the view when the table exists there; a
// view that was never built has nothing to rename.
func alterView(dbPath, table, stmt string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		fmt.Printf("note: %s not present in %s yet; only the DDL changes\n", table, dbPath)
		return nil
	}
	_, err = st.DB().Exec(stmt)
	return err
}

func crudComment(paths []string, table, id, text string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	cid, err := api.CommentAdd(table, id, text)
	if err != nil {
		return err
	}
	fmt.Println(cid)
	return nil
}

func crudComments(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	rows, err := api.CommentsFor(table, id)
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Printf("%s\t%s\t%s\n", r["id"][:min(8, len(r["id"]))], r["date_created"], r["comment"])
	}
	return nil
}

func crudAttach(paths []string, table, id, target, desc string, copyIn bool) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	var attID string
	if copyIn {
		attID, err = api.AttachFile(table, id, target, desc)
	} else {
		attID, err = api.AttachURI(table, id, target, desc)
	}
	if err != nil {
		return err
	}
	fmt.Println(attID)
	return nil
}

func crudAttachments(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	atts, err := api.AttachmentsFor(table, id)
	if err != nil {
		return err
	}
	for _, at := range atts {
		fmt.Printf("%s\t%s\t%s\t%s\n", at.ID[:min(8, len(at.ID))], at.Kind, at.Label, at.Description)
	}
	return nil
}

func crudCreate(paths []string, table string, assigns []string) error {
	fields, err := parseAssigns(assigns)
	if err != nil {
		return err
	}
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	id, err := api.Create(table, fields)
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func crudGet(paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	row, ok, err := api.Get(table, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no row %q in %q", id, table)
	}
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	for _, c := range cols {
		fmt.Printf("%s: %s\n", c, row[c])
	}
	return nil
}

func crudUpdate(paths []string, table, id string, assigns []string) error {
	fields, err := parseAssigns(assigns)
	if err != nil {
		return err
	}
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	return api.Update(table, id, fields)
}

func crudSetNull(paths []string, table, id, field string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	return api.SetFieldNull(table, id, field)
}

func crudDelete(verb string, paths []string, table, id string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	if verb == "markdelete" {
		return api.MarkDelete(table, id)
	}
	return api.Delete(table, id)
}

func crudQuery(paths []string, query string) error {
	st, api, err := openAPI(paths)
	if err != nil {
		return err
	}
	defer st.Close()
	rows, err := api.Query(query)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("(no rows)")
		return nil
	}
	cols := make([]string, 0, len(rows[0]))
	for c := range rows[0] {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	fmt.Println(strings.Join(cols, "\t"))
	for _, r := range rows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = r[c]
		}
		fmt.Println(strings.Join(vals, "\t"))
	}
	return nil
}
