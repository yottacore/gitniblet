<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Project backlog

This is a product backlog just for pre-v1.0.0 release. After that, bugs, features, and enhancements will be managed in Github Issues, and/or [todo.md](../todo.md)

<!-- TOC ignore:true -->
## Table of contents
<!-- TOC -->

- [Conventions](#conventions)
- [Backlog](#backlog)
	- [Misc to-do](#misc-to-do)
	- [Bugs](#bugs)
	- [New features and enhancements](#new-features-and-enhancements)
	- [Done](#done)
		- [Done - Bugs](#done---bugs)
		- [Done - New features and enhancements](#done---new-features-and-enhancements)
		- [First steps](#first-steps)
	- [Deferred](#deferred)
	- [Canceled](#canceled)

<!-- /TOC -->

## Conventions

In each section, items are listed approximately from newest to oldest.

| Icon | Status
| :--: | :--
| 🔘   | Not started
| 🛠️   | Started, and/or partially complete
| ✋   | Defer
| ✅   | Complete
| 🚫   | Canceled

## Backlog

### Misc to-do

- 🛠️ Figure a clean and consistent way to hide list columns (or fields in general).
	- Design settled: `visible_form`/`visible_list` per field (spec in example.shcl, decisions in design.md UI section). The syntax migration it was waiting on is done; the display work itself is still to do.

- 🛠️ Figure a clean and consistent way to show emojis or other short symbolic text in TUI, for lookup items, rather than description.
	- Design settled: built-in `lookups`/`lookup_values` tables with a `symbol` column, `lookup_in_lists`/`lookup_in_forms: symbol|title|both` display enums, int `idx` reference keys, seed rows in the schema. Spec in example.shcl, decisions in design.md Lookups section. The syntax migration it was waiting on is done; idx assignment, the seed engine and the display enums are still to do.

- 🛠️ Need to research, decide, and document how to separate different concerns in the enterprise repo (and identify what the concerns even are), e.g.:
	- Customer-facing enterprise product
	- Our owned side used to (possibly) generate license keys, process payment, check subscription status, etc. This would basically be a completely separate company precense, ecommerce, and validation system.
		- Basic requirements:
			- 🔘 Minimal web dependencies. (To minimize contant cascading updates pulling in potentially hundreds of unknown/untrusted dependencies, near-constant churn with breaking and deprecating third-party updates; and also the growing threat of "supply-chain attack" risk.)
	- Decisions captured in the enterprise repo (`research-enterprise-concerns.md`); implementation is phased (see the folder-hierarchy item below).

### Bugs

### New features and enhancements

- 🛠️ Optional encrypted data in the transaction log. The local SQLite copy is always decrypted.
	- Reason: keep the log unreadable to the git host, or to anyone who gets the repo.
	- Note: some columns stay clear so git can still merge and replay (table and field names, row id, user, host, counts). Only the field values are private.
	- Done: phase one encrypts field values. A field with no key reads back empty and warns, so ciphertext never reaches the view. Full design is in design.md.
	- ✅ Key lives outside the repo, one per database. Users share it out of band.
	- ✅ `--init --encrypt=on` sets up the key and turns encryption on. It can be toggled per host afterward.
	- ✅ With encrypted data present but no key, the user is warned and can still read and write the clear fields.
	- ✅ DDL `encryption: always|never|auto` at the database, table, and field level. `always` and `never` lock the levels below; `auto` defers to the local preference.
	- 🔘 Deferred (phase two): also encrypt the DDL, query, and script sidecar files, with flags to decrypt and re-encrypt them for editing.
	- 🔘 Future: multiple keys by role or team; public and private keys for read-only roles; passphrase-generated keys; re-encrypting existing data with a new key.

### Done

#### Done - Bugs

#### Done - New features and enhancements

- ✅ Use sister project 'SHCL' as the engine for config and DDL files.
	- Done: one language now covers the schema, the queries sidecar, and ngdb's own registry, settings and web-credential files. The TOML dependency is gone, and so is the hand-rolled indent parser.
	- Done: schema files are `.shcl`, queries are `.queries.shcl`. A directory holding a pre-rename `.ddl` still opens.
	- Done: the schema's own key vocabulary is a validation file inside the binary, so a mistyped key is named with its line number instead of being ignored. A schema with errors marks its database unopenable, as before.
	- Done: renames go through the writer rather than editing lines of text, so comments survive by construction. The file comes back canonical, which does reflow hand-tuned column alignment once.
	- Note: syntax changes to existing schemas - `unique:`/`index:` entries instead of bare comma rows, `.` instead of `/` for path shorthand, relationships need names, seeds name their table. Details in syntax.md.
	- Note: restating a table or a query now merges it rather than dropping the repeat, since that is how the language joins nodes.

- ✅ Track the shcl 1.1 release.
	- Done: a section that combines with an earlier one is now reported with both line numbers, so a second `table: t` folding into the first is no longer silent. `example.shcl` was reorganized to open each section once.
	- Done: quoting a default escapes it again - `@null` is the sentinel, `"@null"` is the text. The schema file had documented this all along; it just could not be honoured before.
	- Done: cross-cutting messages (a table defined twice, a `unique:` naming a missing field) now cite a line like every other message.
	- Done: layout blocks validate at any nesting depth, and each section is described once instead of twice. Both were generated in code before, one only as deep as eight blocks.

- ✅ Track the shcl 1.2 release.
	- Done: a fault in the built-in vocabulary no longer switches schema checking off wholesale, so a mistake there can no longer hide real mistakes in a user's file. Faults now read as faults rather than pointing at a line number the user cannot see.
	- Done: renaming a field or table no longer pulls a commented-out section out of the block it was written in. Uncommenting it afterwards would have attached it to the wrong table.

- ✅ Shorter row and transaction ids in the log.
	- Done: ids are written as 22 characters of base64url instead of 32 of hex. Two ride on every line, so the log gets smaller to read and replay; the git repo barely changes, since both forms hold the same bits.
	- Done: logs written before the switch still replay - both forms are read, told apart by width. Ids are never re-encoded in place: an encrypted value is keyed off the id text, so rewriting one would strand its value.

- ✅ New `ref` field type for a reference to another row.
	- Done: stored as the same raw bytes as the `id` it points at, so it joins directly against the key and takes 16 bytes rather than 22 or 32. Read and written as the usual id text.

- ✅ Every key and reference in the view is binary; text ids live only in the log.
	- Done: the built-in link tables (`many2many`, `comments`, `audit_trail`, `access_rows`) hold their `parent_id` columns as references too, moved together with the queries that read them - a blob compared to a string matches nothing and reports no error, so a half-done conversion reads as "no results".
	- Done: hand-written SQL gets `id()` and `idtext()` to convert between an id's text and stored forms. `id()` rejects a malformed id rather than returning nothing.
	- Done: the id format moved to its own package, shared by the log and the view instead of owned by either.

- ✅ Comments pane in views: a `type: comments` layout block surfaces a table's built-in 1:m comments component as a detail pane.
	- Done: the pane follows a sibling list block's selected row, lists that row's thread, and adds to it - in the TUI (Tab to the pane, Enter to add) and the web UI (a per-row comments link loads the thread, an add form posts it). Comments never become a list column, and a comments block over a table without the feature is dropped with a warning.
	- Done: the demo board view gained the pane and the recorder's TUI beat adds a comment there.

- ✅ Demo gif polish.
	- Done: theme beat dropped; in its place a real TUI action - a task with an existing comment thread is selected and a comment is added in the board's comments pane (a linked detail the list view never shows).
	- Done: CLI part opens with a comment line, uses `--db=`/`--table=` explicitly, and the closing line is `# nano-git-db.`
	- Done: the faux window has square corners and fills the frame except a 4px black outline; terminal scrolls smooth (no jump scroll) with a blinking cursor.

- ✅ Enterprise license validation. Phones home to confirm an active subscription and allows a set number of copies at once. Does not fail if it cannot reach the server for a while, and does not bind to specific hardware.
	- Note: scheme decided in the enterprise repo; implementation is a later phase.

- ✅ Address a database by name on the CLI and in the UIs, not by its file paths.
	- Done: every CRUD verb now names a registered database (`ngdb create issues task ...`) and looks up its ddl/sqlite/log from the registry; the old three-path form is gone for data verbs. The name is the first positional, or `--db`/`--table` flags (both accepted, in any order), and resolves with or without a file extension.
	- Done: `build`/`replay`/`sync`/`gc` and the rename verbs take a name too, keeping an explicit-path form for pre-registration use; `--tui <db>` and `--serve <db>` open a registered database directly.
	- Done: demo recorder, `demos/` walkthrough + `seed.bash`, README, and syntax.md all switched to the name-based form; the recorder's TUI beat lands on a standard dark theme instead of high-contrast.

- ✅ Animated README demo (faux terminal, in cicd, skippable with `--quick`).
	- Done: `cicd/utility/demo-video/demo-video.py` drives a real ngdb (TUI first, then the same data from the CLI) inside a decorated xterm on a private Xvfb, typing at a realistic pace with the odd fixed typo. It renders a 1920x1080 mp4 and a looping 960x540 gif with a fade-to-black loop seam.
	- Done: the mp4 and full gif go to `../private/demo-video/{video,gif}` and GFS-rotate; the latest gif is copied to `assets/demo.gif` for the README. All content is anonymous (fake user/host, /tmp paths).
	- Note: adapted from the sister silkterm recorder, minus its GPU and audio work; deps are python3 stdlib + ffmpeg + xterm/xdotool/Xvfb/xfwm4. A web-UI leg can be added later.

- ✅ CI/CD improvements (all three repos).
	- Done: a small hosted `ci.yml` per repo runs vet, gofmt, test, and build on push and pull request, with Go pinned. The full pipeline stays local.
	- Done: a `dev` integration branch. Features merge to `dev`, and a `dev` -> `main` merge cuts a release. The version is authoritative in a source var, and the release skips if it was not bumped.
	- Done: goreleaser builds, archives, and checksums the release for the floss and vendor repos. The local cross-compile uses the same config.
	- Done: linter and tool versions pinned in one place. A dependabot config brings dependency and toolchain bumps as grouped pull requests.
	- Done: README badges for CI status, latest release, and Go version.
	- Done: first live release cut - v1.0.0-alpha.1 published (archives, .deb/.rpm, checksums, and Windows installers across linux/macOS/windows/freebsd on amd64+arm64).

- ✅ Donations model.
	- Done: a "Support nano-git-db" entry in Help/About and `--donate` shows one blurb and one link (DONATE.md). The CLI prints it, the TUI shows a dialog, and the web UI shows a Support page. Open-source build only.
	- Done: a Support section in README.md, a DONATE.md, and a `.github/FUNDING.yml`.
	- Done: the donate files and package are locked to the maintainer via `.github/CODEOWNERS`.
	- Note: to finish going live, enable a GitHub Sponsors profile and fill in the FUNDING.yml handles.

- ✅ Web UI basic login.
	- Done: an explicit `web_mode` setting (default `local`) picks the shape. `local` identifies the single user with no password (the git repo account, else the OS user) and refuses to serve if a proxy header appears, so an exposed box never runs passwordless. `proxied` requires a login: a session cookie checked against a local file of PBKDF2-hashed passwords, kept outside the synced tree. Add a user with the `webuser` verb.
	- Done: the signed-in user drives the data layer, so the existing user and group permissions apply to the web view.
	- Note: stronger sign-in methods stay an enterprise concern.

- ✅ Exe name should be 'ngdb'.
	- Done: the built binary and all user-facing references (CLI/TUI/web usage, version line, web title) are now `ngdb`; `cmd/nanogitdb` renamed to `cmd/ngdb`. The module path `nano-git-db` and the `NANOGITDB_USER`/`NANOGITDB_HOST` env vars are unchanged.

- ✅ Order of fields in txlog should NOT affect backward or forward schema compatibility.
	- Done: the reader maps columns by the header row's names instead of fixed positions. A column can be reordered, added, or dropped - an unknown extra column is ignored, a missing one defaults to empty (this subsumes the old host_name legacy-width special-case). Header rows are detected by their reserved column names, so even the header order can change. A record just needs enough fields to carry the required columns, else it's flagged torn.

- ✅ Set up the folder hierarchy to separate and delineate the concerns decided in the "Misc to-do" above. (Possibly an additional new private repo?)
	- Approach decided in the enterprise repo docs. Folder layout now in place: the repos are renamed and the additional private repo is created and scaffolded; the concern implementations follow in later phases.

- ✅ Make opening menu look more like a menu. Better spacing around and in beteen items.
	- Done: the startup picker is now a centred, bordered panel with margins instead of a full-screen list, and blank spacer rows sit between entries. Up/Down skip the spacers so the highlight only lands on real entries.

- ✅ When defining a db in the TUI or CLI, allow just pointing to a top-level repo - then figure everything else out (i.e. "[repo dir]/ngdb/[short spaceless db name]/") If the dir is not a github repo, use the directory exactly as entered (assuming it's empty or nonexistent). Don't ask for txlog or config files, just create them in the dir.
	- Done: the CLI `--init` already derived this; pulled the logic into shared `config.LogDirFor(location, name)` and switched the TUI create form to a single "location (repo or folder)" field. Point at a repo top level -> `<repo>/ngdb/<name>`; any other folder is used as-is (created if missing). No separate tx-log path to enter; config still auto-places in the user config dir.

- ✅ Ability to delete entries from TUI (e.g. invalid ones).
	- Done: in the startup picker, `d` (or Delete) on a database asks to remove it, then - only if it has files on disk - a second confirm offers to delete those too (default keep). Remove just deregisters (drops the registry record); delete also removes the tx-log dir and record dir. The `.ddl` schema is always kept. Broken `[!]` records are removable; read-only system records are not.

- ✅ Startup:
	- ✅ If the program is run from an active directory that has a DDL, use that one.
	- ✅ If the program is run without arguments and no DDL is in active directory, search user-level and system-level config directories for defined databases that point to valid DDLs.
		- If there are multiple, allow the user to choose them from an initial screen (or "create new").
		- If a database specified in the config directory cannot be loaded (e.g. can't find the DDL or it's irreversibly corrupt), include the database name in the list, but also with some kind of error indication, and brief text of why it can't be opened.
		- Rebuild anything that is missing and can be rebuilt or defaulted. (e.g. SQLite file.)
		- The list of databases should always include "Create new database", and "Open existing ...". Both will futher prompt for a location for the txlog and associated files. (User-specific, unsynced files go where user config file says. Autogenerate if non-existent.)
	- ✅ If no DDL and nothing in user config, the only options are "Create new database", and "Open existing ..."
	- Done: new `internal/core/config` package holds a per-db TOML registry. Records live under the OS user-config dir, one `config.toml` each, carrying the DDL path, log dir, sqlite path, key file, and encryption preference. `config.List` discovers them and checks the DDL parses; a missing sqlite is fine since it rebuilds from the log on open. Startup routing: a lone `*.ddl` in the current dir opens directly, otherwise the TUI shows a picker of registered databases plus Create-new and Open-existing. Unopenable databases still list, flagged with the reason.
	- Note: Create-new points at an existing DDL. Authoring a DDL from scratch is a separate concern, as are the flags in the next item.

- ✅ Add to README.md:
	- ✅ A section on the bare-minimum, super-simple DDL syntax
		- ✅ Plus an example to make the simplest "to-do" hierarchical database and nested list, with the most minimal features. (E.g. task, status, open and close date, parent task, and related comment list.)
	- ✅ A section with the most basic CLI options to get started.
	- ✅ A link to the full DDL and options syntax in `syntax.md`
		- ✅ Create that file.
	- Done: README gained a "Quick start" with a minimal schema and a full hierarchical to-do example, plus a basic CLI walkthrough and a pointer to the full syntax reference. Both DDL blocks were parse-verified, and the to-do one runs end to end.
	- Done: new `github/syntax.md` is the complete reference covering DDL grammar, types, system columns, uniques, features, access, renames, views, tunables, the sidecar files, all CLI verbs, run modes, and env vars. It documents only implemented behavior; example.ddl's not-yet-wired keys are flagged as forward-looking.

- ✅ Partial flag list:
	- ✅ `--init[=]["git repo root to auto-calculate subfolders for (e.g. 'ngdb/dbname/'), or override default with specific 'git-repo-root-path/subfolder(s)/'"]`  ## If no arg specified, and already in a git root or DDL is in $PWD, use it. Otherwise error.
	- ✅ `--config[=]"Alternate user config file folder, name, and/or full filespec"`  ## Default = "~/.config/ngdb/dbname/" (or Windows analogue).
	- ✅ `--[en]crypt[ion][=]on|off|auto[matic]|yes|no|true|false|y|n|t|f|enable[d]|disable[d]`  ## If on, may be overridden by DDL values of 'never'. If off, may be overridden by DDL values of 'always'. Default=auto.
	- Done (`--init`, `--config`): `--init [path]` registers the lone current-dir DDL as a database and materializes its view. It picks the tx-log dir automatically: inside a git repo it goes under `<repo-root>/ngdb/<name>`, an explicit path is taken verbatim, and outside a repo it lands in the current dir. `--config <dir>` is a global prefix that repoints the registry. Both accept the `=` form.
	- Done (`--encrypt`): landed with the encryption item below. `--encrypt[=on|off|auto]` is a global prefix (bare means on). Paired with `--init` it mints the per-db key and persists the preference.

- ✅ TUI colour themes: selectable, readable, dark and light (default dark)
	- Done: press `T` in the TUI for a picker of six themes - three dark, three light, default dark. Colours are fixed RGB rather than the terminal-palette names tview defaults to, so text stays legible whatever the surrounding terminal looks like. Switching applies live (the database picker included) and the choice persists in a user-global `settings.toml`, so it carries across sessions and databases.

- ✅ Add "host_name" to the transaction log, in addition to user. (Because the same user might use multiple hosts, and that might be useful info.)
	- Done: `host_name` appended as the last tx-log column, so older logs without it still parse as an empty host and both widths survive a union merge. It's stamped automatically from the OS hostname, overridable with `NANOGITDB_HOST`, the machine analogue of `NANOGITDB_USER`. Metadata only, so replay ignores it. All four front-ends inherit it.

- ✅ Tx-log garbage collection: mark + collect records older than N days
	- Done as the designed segment rotation, via `gc <ddl> <logdir>`. Entries for rows hard-deleted longer than `gc_age_days` ago (default 90) get dropped, survivors land in a write-once segment file, and the old files retire whole. No log line is ever edited in place, so the union merge stays safe. Reads walk the segments then the live file; new writes recreate it.
	- Note: sync now stages the whole log dir, so both the rotations and the attachments folder replicate. That also closes a replication gap left by the attachments item.
	- Note: if two clients GC at once they just duplicate survivors, which replay collapses on the next pass.

- ✅ Tunable options (e.g. `git_sync_frequency`, tx-log GC age)
	- Done: a `tunables:` DDL section accepting both `key: value` and `key = value` forms. `git_sync_frequency` (default 60, 0 disables) drives a background git sync loop in the TUI and web UIs when the log dir is a git work tree; a pull that brings new entries full-replays into the view, so two open UIs converge without restarting. `gc_age_days` (default 90) is parsed and waiting for the GC item. Unknown tunables warn but still store.

- ✅ DDL-defined scripting: triggers, events, before/after methods via the Lua host (now an enterprise-edition feature; the open-source build ignores `code:` hooks)
	- Done for the write path and app open: `code:` keys name functions in a `.lua` sidecar. Field `before_update` can veto or replace a value, table `before_update` can veto and runs after the field hooks, both `after_update` forms fire post-commit, and app `before_open` can veto the open. Trigger code gets the sandboxed `db.*` API, and writes from inside a trigger don't re-fire triggers, so hooks can't recurse. Hooks fire in the core CRUD API so all four front-ends behave identically; replay never fires them. Missing functions warn instead of blocking.
	- Note: sync and UI-lifecycle hooks and `validation: method:` are deferred (see design.md).

- ✅ CLI schema ops: `--rename-table`, `--rename-field` (updates DDL + SQLite, adds aliases)
	- Done: `--rename-table` and `--rename-field`. The DDL is rewritten line by line so comments and formatting survive, with the old name added to an `aliases:` line, and the SQLite view gets an ALTER TABLE rename. The part that makes it safe: replay translates aliased names in old tx-log entries to the current ones, so a log written before the rename still rebuilds correctly. The log itself is never rewritten, since append-only stays sacred.

- ✅ Granular access model: view -> table -> field|record, by user groups
	- Done with everything-open-until-configured semantics, so a userless setup keeps working. Identity is the write-stamp user resolved through the users table, with groups via the users/groups m:m. Blacklist wins and an empty whitelist means everyone; owners always pass and admins pass read and write but not delete. Writes are gated in the core CRUD API so no front-end can bypass the table, field, and row-grant rules. Reads are hidden at the catalog instead: unreadable tables and views vanish with a warning, unreadable fields drop from grids and forms, and ungranted rows filter out of row lists.
	- Note: raw SQL `query` stays open by design. The local file is the trust boundary and the write path is the hard gate (documented in design.md).

- ✅ Predefined queries: plain-text query file, dropdown in the UIs
	- Done: queries live in a `.queries` sidecar next to the DDL, parsed with the same grammar and warn-and-fix validation. The TUI offers them on a view via 'p', loading the dataset into the focused block with the query's own columns; the web view page gets a dropdown and Run. A view's `startup_named_query` naming an active query loads it the moment the view opens in both UIs, and an unresolvable name degrades to the usual empty open. Queries without a `view:` are offered on every view, and `active: no` hides one without deleting it.

- ✅ Opt-in table features driven by DDL: local + URI attachments, comments, audit trail, row-level access
	- Done in the core CRUD API so all four front-ends behave alike, and everything replicates through the tx-log like ordinary data. Audit capture rides the same commit as the change it records; same-user updates coalesce into one record keeping the oldest old-value per field, and a hard delete snapshots the row. Comments and both attachment kinds get API helpers plus UI across the TUI, web, CLI, and Lua. Copied files land under `<logdir>/attachments/` and sync with the log.
	- Note: Lua has no `attach_file`, since its sandbox has no file access.
	- Note: row-level access grants work here; enforcement belongs to the access-model item below.

- ✅ Added a new hierarchical levels:
	- 'ui:' above "views:".
		- Also with a new child, "default_view":
			- If no default view specified, defaults to the first one defined.
	- 'database:' above "tables:" and "relationships:".
	- Done: both wrappers parse transparently, so older flat DDLs still work, and the default view opens on startup in the TUI and web UI with blocks still empty per the no-records-until-asked rule. A `default_view` naming an unusable view warns and falls back to the first. The example's `methods:` key was renamed to `code:`, with the old key still read.

- ✅ DDL hierarchy doesn't have to be unique.
	- As an example, 'tables:'/'table:' can be defined multiple times
		- In that example, treat it as if there's only one 'tables:' definition, logically collapse the children with redundant 'tables:' into one.
	- Applies to all elements, not just 'tables:' and not just top-level.
	- Could be very helpful when lists get long and losing track of indentation could be a problem.
	- Hierarchies can be short-handed. For example, instead of "database:", "tables:", table: "table_name1", you could optionally specify `database/tables/table: "table_name1"`. (Or maybe '.' or '|' delimiter instead of '/'. Research what others do for DDL.)
	- Done: duplicate sections merge into the first occurrence at every level, while entities like table/field/view stay separate. A scalar redefined to a different value keeps the first and warns. The '/' path shorthand expands to real nesting.
	- Note: kept '/' as the delimiter since example.ddl already uses it; '.' collides with numbers and '|' reads as alternation.

- ✅ Validate the DDL file and transaction log upon load. Fix soft syntax errors (with a warning), that can be safely assumed around. If something is genuinely incorrect too ambiguous to assume around, hard error on startup.
	- Done: DDL load drops nameless or redefined tables and fields and system-column collisions (first definition wins), and treats an unknown field type as text, each with a warning. Mixed indentation stays a hard error. Tx-log load warns per torn line with its file and line number instead of skipping silently; repeated header rows still skip quietly as normal union-merge leftovers. Replay skips and warns on mangled row ids and unknown operations, which may come from a newer client, rather than aborting the whole rebuild. All four front-ends surface the warnings.

- ✅ On-the-fly schema change: update or rebuild local `.sqlite` when DDL changes with data present
	- Done additively: `build`, `replay`, and `sync` add a column for any field added to the DDL, and replay skips with a warning any log entry for a table or field the local schema doesn't know yet, so a schema edit can never strand an existing database. Type changes and field drops still mean a rebuild from the log, which loses nothing since the log is the truth.

- ✅ Transaction log enhancements:
	- ✅ Search and replace problematic symbols, e.g.:
		- Done: every entry is now exactly one physical line with no CSV quoting, so git's line-based union merge can never tear a record. Encoding and decoding happen only at the file boundary; the SQLite view holds real characters.
		- Note: 🗦NULL🗧 now expresses SQL NULL via a new `SetFieldNull`, and 🗦LDELIM🗧/🗦RDELIM🗧 escape literal delimiters so content can't spoof a marker.
		- ✅ Delimiters for "unsafe" content: 🗦 and 🗧, then what's in-between are "constants" describing the character. For example:
			- 🗦NEWLINE🗧
			- 🗦RETURN🗧
			- 🗦SINGLE_QUOTE🗧
			- 🗦DOUBLE_QUOTE🗧
			- 🗦BACKTICK🗧
			- 🗦BACK_SLASH🗧
			- 🗦FORWARD_SLASH🗧
			- 🗦COMMA🗧
			- 🗦MISC_ILLEGAL🗧 (bell, backspace, etc. - actually those should just be scrubbed before landing)
			- etc.

- ✅ View block `tree_grid` type + `parent_field`: hierarchical grid over a self-referential (same-table) parent link
	- Done, and it pulled in view rendering itself, since no front-end rendered views yet. Views now resolve once in the core, verifying tables, turning split hints into axis and percent, and inheriting readonly, then render in both TUI and web as nested split blocks. `tree_grid` orders rows depth-first along `parent_field` with indentation, and bad data like orphans, cycles, or a wrong field degrades visibly instead of hiding rows. Blocks open empty per the design and the "All" action loads them; `startup_named_query` hooks in once named queries land.

#### First steps

- ✅ Repo scaffold: `go.mod`, package skeleton, arg-dispatch `main`, size-optimized build script, `.gitignore`. Compiles, vets, and gofmt-clean; all four front-ends stubbed and the web server serves its embedded asset.

- ✅ DDL parser: `example.ddl` parses into an in-memory schema model through three layers (indent tree, type-directed values, semantic map). Tested, wired to `nanogitdb ddl <file>`.

- ✅ SQLite view: builds and migrates the local `.sqlite` from the schema using pure-Go `modernc.org/sqlite`, with system columns and uniques/indexes, idempotent. Tested, wired to `nanogitdb build`. On-the-fly migration of existing data is a separate item below.

- ✅ Transaction log: CSV read/write plus apply log to the SQLite view, with the log as source of truth. Append-only, field-granular replay (create/update/mark_delete/delete) in one tx. Tested, wired to `nanogitdb replay`. Added `row_id` to the entry (see design).

- ✅ Core CRUD API: the single internal API every front-end and Lua calls, covering Create/Update/SetField/MarkDelete/Delete/Get/Query. Writes are log-first (append to truth, then apply to view), and it owns id, timestamp, and user stamping. Tested including a full rebuild from log. Validation, access, and triggers are separate items.

- ✅ CLI front-end: full arg-based CRUD over the core, with verbs create/get/update/setnull/markdelete/delete/query each taking the same `<ddl> <sqlite> <logdir>` triple as replay and sync. Stateless and script-friendly until the config file lands. Writes stamp NANOGITDB_USER or the OS username, and `setnull` is its own verb so a literal "NULL" string stays expressible. Schema-op flags are their own backlog item.

- ✅ Git sync: pull/merge/commit/push of the tx-log dir with a background loop, shelling out to `git`. Append conflicts auto-resolve via `merge=union`, and replay sorts by (date, tx_id) so clients converge. Tested including two-client convergence, wired to `nanogitdb sync`.

- ✅ Lua host: `gopher-lua` bound to the same core CRUD as `db.{create,get,update,setfield,markdelete,delete,query}`, sandboxed to base/table/string/math/coroutine only with no os/io. Tested, wired to `nanogitdb --script`.
	- Note: now an enterprise-edition feature. The host moved behind the script seam, so the open-source build has no scripting host and drops the gopher-lua dependency.

- ✅ Auto tables: users, groups, and the opt-in feature tables (m:m, comments, audit, access, attachments), defined in the same DDL users write and embedded in one source file. Built idempotently at startup, with default groups (owners/admins/users/guests) seeded log-first so they replicate. audit_trail opts out of the universal system columns via the new `system_fields: no` key, and access_rows only appears when a table opts in to row_level_access. First-user membership in owners/admins is deferred until user rows exist.

- ✅ TUI front-end: `tview`/`tcell`, both pure Go with the cross-compile intact. Table list (user tables first, then built-ins), a rows grid that stays empty until a table is opened, a create/edit form that writes only changed fields, and soft/hard delete behind a confirm modal, all through the shared CRUD API. Tested headlessly on tcell's simulation screen, including a full boot-open-quit pass. `nanogitdb --tui <ddl> <sqlite> <logdir>`.

- ✅ Web UI: stdlib `net/http` + `html/template` + `embed` with vendored, pinned htmx (2.0.4). Table sidebar, a rows grid that renders nothing until a table is asked for, a create/edit form writing only changed fields, and soft/hard delete behind hx-confirm; the 127.0.0.1-only binding is the access control. Front-end bring-up and table metadata live in shared `schema.OpenClient`/`schema.Catalog`, so the four front-ends can't drift. `nanogitdb --serve <ddl> <sqlite> <logdir>`.

- ✅ CI/CD: a manual pipeline in `cicd/cicd.bash` that builds native (size-reported), cross-compiles windows-amd64/linux-arm64/windows-arm64 (pure Go, no extra toolchains; macOS stays deferred), zips windows into `dist/`, runs test.bash (vet, gofmt, tests, smoke), verifies the module and runs govulncheck, then dogfoods and publishes. Profiling beyond the size report waits until there's something worth profiling.
	- ✅ `-q/--quiet` (unattended, no prompt, flows to the publish step) and `-m/--message/--msg` (`-m MSG` or `-m=MSG`; auto-generated when `-q` and no `-m`), in both cicd.bash and the n8git publish helper. Stage output uses the existing `fEcho`/`fEcho_Clean` helpers (bracketed section headers, blank-line grouping).
	- ✅ `--quick`: skip the slow stages (cross-builds + Windows packaging/install, fuzz, profiler, screenshots, govulncheck, gosec); native build, lint, quick tests, go mod verify, dogfood, and publish still run.
	- ✅ Pipeline hardening: lint+format stage (`gofmt -w` + `staticcheck`), auto-discovered fuzz targets (`fuzz.bash` over the DDL/txlog/config parsers, seeds also run in the normal suite), `gosec` beside govulncheck (documented exclude list), and a non-gating profiler stage - samples the tx-log replay hot path into an inferno flamegraph (`cicd/artifacts/profiling/`, GFS-rotated) with a `flame-report.py` hotspot summary. Run log tee'd to `cicd/artifacts/lint/` for after-the-fact review (`lint-report.bash`).
	- ✅ Full cross-platform packaging (non-`--quick`): goreleaser cross-builds linux/macOS/windows/freebsd for amd64+arm64 (all pure Go, one Linux box, no C toolchain), then packages `.deb`/`.rpm` for linux, a `.tar.gz`/`.zip` per target, and checksums. A single self-contained Windows `.exe` installer per arch is built with NSIS (`windows-installer.bash`/`.nsi`) - installs, adds to PATH, upgrades an existing install. Builds split: a native debug binary drives testing/profiling, an optimized release binary is what gets packaged and dogfooded. `--no-arm` trims a run to amd64 only (arm is cheap here, so it's opt-out not opt-in). Deferred (each needs the target OS's own tooling/signing host): macOS `.dmg`, Windows `.msi`, FreeBSD `.pkg` (ship as archives); AppImage + Flatpak.

- ✅ Supply-chain hardening: vendored deps, `go mod verify`, `-mod=vendor`, and `govulncheck`, all wired into the pipeline. The first govulncheck run found five reachable Go-stdlib vulns; pinning `toolchain go1.26.4` cleared them and it now scans clean.

### Deferred

- ✋ Native GUI (Fyne/Wails) - rejected for now; both need cgo/webview and break the pure-Go cross-compile

- ✋ macOS build target - needs a Mac/SDK

- ✋ Make signing keys required to edit "about" modal with link to DONATE.md (that in turn has CODEOWNER):

	~~~bash
	project="PROJECT"; mkdir -p private/donation_keys; chmod 700 private/donation_keys
	ssh-keygen -t ed25519 -C "${PROJECT} donation signing" -f private/donation_keys/donation_ed25519
	cat private/donation_keys/donation_ed25519.pub | ct
	cat private/donation_keys/donation_ed25519 | ct
	~~~

### Canceled

- 🚫 Donate feature, with protected donation addresses (open-source build). List donation crypto addresses and URLs from CLI (`--donate`), TUI, and web.
	- Done: a `donate` package holds a fixed-order `{label, kind, value}` table shown by `--donate`, a TUI picker entry, and a web Donate page. Ships as placeholders that read "not yet configured" until the real values are filled in.
	- Done: the table is protected against a swapped address the way the sister project does it - a detached ed25519 signature (`ssh-keygen`) over the canonical table, with the signing key and trust anchor kept outside the repo, re-checked by a test gate that skips on placeholders or a keyless machine. Sign with `cicd/sign-donations.bash`; details in `cicd/donation-signing.md`.
