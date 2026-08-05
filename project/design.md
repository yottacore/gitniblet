<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Project design

<!-- TOC ignore:true -->
## Table of contents
<!-- TOC -->

- [Goal](#goal)
- [CI/CD pipeline](#cicd-pipeline)
- [Language and stack](#language-and-stack)
- [Architecture](#architecture)
- [Logical code organization](#logical-code-organization)
- [Initial high-level features](#initial-high-level-features)
	- [User-definable DDL](#user-definable-ddl)
	- [Transaction log](#transaction-log)
	- [Scripting triggers](#scripting-triggers)
	- [Schema renames](#schema-renames)
	- [Tunable options](#tunable-options)
	- [Startup discovery and database registry](#startup-discovery-and-database-registry)
	- [UI](#ui)
	- [Predefined queries](#predefined-queries)
	- [Optional granular access model](#optional-granular-access-model)
	- [Startup notice and read-only mode](#startup-notice-and-read-only-mode)
	- [Web login](#web-login)
	- [Donations](#donations)
	- [Encrypted transaction log](#encrypted-transaction-log)
	- [Tables created automatically at new startup and verified in the background every startup](#tables-created-automatically-at-new-startup-and-verified-in-the-background-every-startup)
		- [Always created and used](#always-created-and-used)
			- [Users](#users)
			- [Groups](#groups)
			- [Lookups](#lookups)
		- [Automatic features that any table can opt-in to](#automatic-features-that-any-table-can-opt-in-to)
			- [Many-to-many relationships](#many-to-many-relationships)
			- [Comments](#comments)
			- [Audit trail](#audit-trail)
			- [Row-level group access](#row-level-group-access)
			- [Attachments](#attachments)
				- [URI](#uri)
				- [Copies](#copies)
	- [CLI arg-based API](#cli-arg-based-api)
	- [Internal API](#internal-api)
	- [Scripting](#scripting)

<!-- /TOC -->

## Goal

A super-robust database creation tool with many bells and whistles inherited by default without having to specify anything, and powerful additional "add-ons" that table definitions can opt-in to. The UI and data model are tightly correlated, in terms of definition.

The schema can be changed via DDL mid-project, without affecting existing data, no other users who haven't synced with the latest DDL yet.

In spite of being a SQL backend, data is saved one-field at a time - as long as the minimum required fields are met.

## CI/CD pipeline

Manually kicked off via `cicd/cicd.bash`. Stages, in order (each fails early and hard on a real problem, exit >0):

- **Build** native size-optimized, then cross-compile (windows-amd64, linux-arm64, windows-arm64; pure Go, no extra toolchains), zip windows into `dist/`.
- **Lint + format**: `gofmt -w` rewrites Go in place (never Bash), then `staticcheck` gates. `go vet` runs in test.bash.
- **Tests**: full regression suite (`go test`), which also runs every fuzz target's seed corpus.
- **Fuzz**: `cicd/utility/fuzz.bash` auto-discovers each `FuzzXxx` and runs it a short budget; a crash fails the run.
- **Security + supply-chain**: `go mod verify`, `govulncheck`, and `gosec` (with a documented exclude list justified for a local-first single-user git-synced tool).
- **Profiler** (non-gating artifact): samples the CPU-hot tx-log replay path, renders an inferno flamegraph into `cicd/artifacts/profiling/` (GFS-rotated), and prints a hotspot summary (`flame-report.py`). Tooling misses warn and skip.
- **Dogfood**: copy the native release to the local run path.
- **Screenshots**: regenerate the README thumbnails.
- **Publish**: `n8git_backup-and-publish`.

The whole run is tee'd to a rotated `cicd/artifacts/lint/run_<ts>.log` so warnings from any stage can be reviewed after the fact (`lint-report.bash`). `--quick` skips the slow stages (cross-builds, fuzz, profiler, screenshots, govulncheck, gosec); lint + tests still run. Every step is capped to at most half the cores (`cpu-limit.bash`: `GOMAXPROCS` + `-p`). `cicd/artifacts/` is gitignored.

## Language and stack

Go. Picked for trivial static cross-compilation (`GOOS`/`GOARCH`, `CGO_ENABLED=0`) to every target with no C toolchains. Hard rule: **stay pure-Go (no cgo)** - the moment a dependency needs cgo, per-target C cross-toolchains come back and the whole reason for choosing Go is gone.

Binary size is a real goal (though not the absolute top priority it would be for a Rust build): build with `-ldflags="-s -w"`, prefer lean deps, consider `upx`. Realistic floor is ~8-15 MB (GC runtime + pure-Go SQLite), vs the ~2-5 MB a Rust build would hit - the accepted cost of Go's cross-compile simplicity.

| Concern    | Choice                   | Why
| :--        | :--                      | :--
| SQLite     | `modernc.org/sqlite`     | Pure-Go (C transpiled to Go), no cgo. Larger/slower than `mattn/go-sqlite3` but cross-compiles clean.
| Git sync   | shell out to `git`       | Design only needs pull/merge/commit/push on the log dir. Avoids libgit2 bulk. Revisit only if conflict handling outgrows the CLI.
| Tx log CSV | stdlib `encoding/csv`    | Canonical, zero dep.
| Scripting  | `gopher-lua`             | Pure-Go Lua 5.1. Well-known embedded language, easy syntax, easy to sandbox and expose the CRUD API into. External `python3` via exec stays available as the worst-case fallback.
| IDs (GUID) | `google/uuid`            | v4/v7, stored as a 16-byte blob, base64url text at the boundaries.
| CLI        | TBD (lean stdlib `flag`) | Large flag surface; stdlib `flag` favours size, move to `cobra` only if subcommand ergonomics demand.
| Formatting | `gofmt`                  | Tabs-native, matches house style for free.

One binary, four front-ends (see Architecture). UI is two pure-Go tiers: a **TUI** (`tview`/`tcell`) and a **local web UI**. The web tier is deliberately stdlib-only - `net/http` (Go 1.22+ `ServeMux` does method + path routing, so no `chi`/`gorilla`), `html/template` (auto context-aware escaping), and `embed` to bake assets into the binary. Client-side interactivity is **htmx**: a single ~15 KB JS file, vendored and pinned by SRI hash - no npm, no build chain, no transitive deps. Net web-tier third-party Go deps: **zero**. A native GUI (Fyne/Wails) stays rejected - both need cgo/webview and would break the pure-Go cross-compile.

## Architecture

One static binary, a shared core with four thin front-end adapters over it. Build the engine once; each interface is a mode selected at startup.

~~~text
                 ------------ core engine (internal API) -------------
                 |  DDL parse . SQLite view . git tx-log . CRUD      |
                 -----------------------------------------------------
                    |          |           |              |
main dispatch:     CLI      Lua host      TUI         Web server
~~~

- **CLI arg API** - stdlib `flag`, calls core directly.
- **Lua native API** - `gopher-lua` registers Go funcs so scripts call the *same* core CRUD API (not a second code path).
- **TUI** - `tview`/`tcell`.
- **Web UI** - stdlib `net/http` + `html/template` + `embed`, htmx for dynamic grid/form swaps.

The four are different run modes, not concurrent contenders (`<verb>` = CLI, `--tui`, `--serve`); the web server is just goroutines. Lua is callable from any mode. Every front-end is a thin adapter - all real logic lives in the core, so behaviour can't drift between interfaces.

Supply-chain hardening (from day one): vendor everything (`go mod vendor`, commit `vendor/`) so builds never hit the network and every dep is diffable in PRs; `go mod verify` + hash-pinned `go.sum`; build `-mod=vendor`/`-mod=readonly`; `govulncheck` in the manual CI pipeline; minimal-dep policy (prefer stdlib, each module a deliberate call); vendored `htmx.min.js` pinned by SRI hash.

## Logical code organization

Standard Go layout, with the **module root under `source/`** so all source - including vendored deps - lives there. Core is a library; `main` is a thin dispatcher. Import paths are `github.com/jim-collier/nano-git-db/internal/...` (no `source` segment); run `go` from `source/`.

~~~text
source/go.mod, go.sum         - module root (module github.com/jim-collier/nano-git-db)
source/vendor/                - committed third-party deps; build uses -mod=vendor
source/cmd/ngdb/main.go  - arg dispatch -> one of the four modes
source/internal/core/         - engine: the single source of truth for all logic
	ddl/                      - DDL parser + schema model
	store/                    - SQLite view (modernc.org/sqlite), migrations
	txlog/                    - git-backed CSV transaction log, sync, GC
	crud/                     - CRUD API the front-ends + Lua all call
source/internal/cli/          - stdlib flag front-end
source/internal/lua/          - gopher-lua host, Go<->Lua CRUD bindings
source/internal/tui/          - tview/tcell front-end
source/internal/web/          - net/http handlers, html/template, embedded assets
	assets/                   - html/css + vendored, SRI-pinned htmx.min.js (go:embed)
~~~

`internal/` keeps the core unimportable by outside modules (no accidental public API surface). Front-ends depend on `core`, never on each other.

## Initial high-level features

### User-definable DDL

The schema is written in [SHCL](https://github.com/jim-collier/shcl), the sister project's small indent-nested `key: value` language. [example.shcl](example.shcl) is a worked example exercising every key. It defines tables, fields, relationships, indexes, unique constraints, validation, code execution (triggers, events), UI views, and opt-in prepackaged features.

The language decision, and what it bought:

- SHCL replaced a hand-rolled indent parser once it reached v1.0.0 and put its grammar, APIs and diagnostic codes under semver. The two grammars had converged on nearly the same shape, so existing schemas needed only four mechanical corrections - which is what made the swap cheap enough to be worth doing.

- It is one language for the whole product: the schema, the queries sidecar, and ngdb's own registry, settings and web-credential files. That retired the TOML dependency, so a user reads and edits one syntax everywhere.

- Its Go binding is a single dependency-free pure-Go file, so `CGO_ENABLED=0` and the one-static-binary rule are untouched (the reference implementation being Rust is irrelevant to us - we never link it).

- Schema *validation* came along with it. The DDL's own key vocabulary now lives in `ddl/schema.shcl`, embedded in the binary, and SHCL checks a loaded file against it - so an unknown key or an out-of-range value is reported with its line without this codebase parsing anything. Checks the schema file cannot express - a `unique:` naming a field that does not exist, a field defined twice - stay in Go, and cite the offending entity's own line.

- A mistake in the vocabulary file used to disable checking of the user's file entirely, so a broken vocabulary and a clean one both produced silence. That is no longer the case: whatever states cleanly still checks, and only the unknown-key sweep needs a fault-free vocabulary - a dropped rule would turn the keys it described into false unknowns. The two failure modes are reported apart, since a fault's line number belongs to the built-in file rather than to the schema in front of the user, and a test loads a key no real schema could hold to prove the sweep is running at all.

- The vocabulary file is the whole vocabulary, because two shapes it once could not state are *fragments*: a named, reusable shape mounted at a path. Layout blocks nest without limit by mounting a shape inside itself, rather than being generated out to a fixed depth and silently unvalidated past it. And a section that may sit under the `database:`/`ui:` wrapper or at the top level is one shape mounted at both paths, instead of a second generated copy of every path beneath it.

Consequences of SHCL's data model, which are load-bearing here:

- **Merging is the core rule.** Nodes merge when (field-name, value) match, so restating `database:` or `tables:` re-opens that section instead of creating a second one. The old parser had a section-merging pass of its own for exactly that readability win; this comes free. It extends further, though: restating an *entity* merges it too, so two `table: t` sections are one table. The mapper cannot see that - by the time it walks the document the two are one node - so the report comes from shcl itself, which flags any binding that combines with a non-adjacent earlier one and cites both lines. That report is deliberately passed through unfiltered even for the wrapper sections a schema is meant to re-open: when a wrapper and an entity inside it both merge, only the outermost is reported, so filtering the wrappers would take the entity case with it. The shipped `example.shcl` opens each section once so it stays quiet, which is the layout worth copying.

- **Empty values merge too**, so unnamed instances collapse into one. Relationships must be named for that reason; the name is otherwise just a label.

- **Field names fold ASCII case; values do not.** Anything whose identity is user-supplied and case-sensitive has to be modelled as an instance discriminated by that value, never as a child field name - see [Web login](#web-login), where two accounts differing only in case would otherwise share one password.

- **Loose strictness** is what the schema loads at, and it is almost exactly the tolerance this DDL already documented: `y`/`n` and `enable`/`disable` as booleans, a bare leading or trailing `.` on a number, a leading currency symbol, a fraction where a whole number was wanted. Only Strict can fail a load, so a schema file never aborts one; the registry (rather than the parser) is what refuses to open a database whose schema has errors.

- **Hierarchies short-hand with '.'**: `database.tables.table: t1` is the same as the nested form.

- **Wrapper levels** `database:` (over `tables:`/`relationships:`) and `ui:` (over `views:`, plus `default_view`) are transparent; flat schemas without them still parse. Same for `methods:`, renamed to `code:` - both keys are read.

- **Values containing commas need the verbatim read.** An unquoted comma splits a value into a list, so SQL and regexes go in a raw block or single-line backticks, which come back exactly as authored. Quoting distinguishes a sentinel from its own text: `@null` is the sentinel, `"@null"` is the five characters.

Also:

- If the schema file is updated while an existing DB has data, the local `.sqlite` is either updated on-the-fly, or rebuilt.

- The fields referenced in the transaction log are never deleted or changed. References to past and future nonexistent fields in the schema are ignored.

- Functions are written in a scripting language the main program hosts internally, with access to the internal CRUD API.

- Warn if it looks like user created a primary key field, which already exists.

- Load-time validation fixes what it safely can with a warning - nameless fields are dropped, a field colliding with an auto-added system column is dropped, an unknown field type stores as text - and never hard-errors. Same philosophy on the tx-log: torn or unreadable lines skip with a warning naming the file and line, repeated header rows (union-merge leftovers) skip silently, and replay skips-and-warns entry-local data errors (mangled row id, an operation this build does not know - possibly a newer client's) exactly like schema drift.

- Most schema keys are optional, with sane defaults.

- Special values (without quotes):

	- **Null / previous-value sentinels**: `@null`, `@previous`
	- **Function**: some_function_name()
	- **SQL**: a raw block, or \`SELECT * FROM ...\`
	- **String**: "A string with o'l spaces" or 'A string with "literal" quotes' or AstringWithNoSpaces.
	- **Boolean**: true|false|1|0|t|f|yes|no|on|off|enable[d]|disable[d]|y|n
	- **Number**; `10`, `.1`, `1.`, `0.1`, `1.0`, `0x1f`, with an optional leading currency symbol or trailing `%`.
	- **Date/time**: Anything internal parser or `printf` can interpret. String, linux/unix epoch float, Excel-style float on Windows, etc.

### Transaction log

The transaction log is the backbone of the database. The actual SQL database is just the local view into it, and not the single-source of truth.

After the program loads with a local view of the SQLite database, it begins in the background:

- Syncs the transaction log via git, with remote repo. (If log is in a git repo.)

- Auto-resolves conflicts. Mechanism: the log file is set to `merge=union` in `.gitattributes`, so two clients' concurrent appends combine instead of conflicting. Ordering is not left to the merge - replay sorts entries by `(date, tx_id)`, a deterministic total order, so every client's view converges to the same state regardless of how git interleaved the lines. (Shells out to `git`; a non-git log dir just skips sync.)

- Syncs the local SQLite database with the transaction log, handling conflicts.

- If the current record being shown by the app got deleted during that process, warn the user and allow them to recreate it if necessary, or else change to the next real record.

- CSV format

Fields:

- tx_id (GUID)
- date (GMT)
- table_name
- row_id  ## GUID of the affected row. Added during implementation - field-level ops can't be applied without a row identifier.
- field_name  ## Can be blank if record-level only
- operation (create, update, mark_delete, delete)
- new_value
- user_id
- ok_to_garbage_collect
- host_name  ## Machine that wrote the entry. One user can write from several hosts.

Both ids are a 16-byte UUIDv7 written as 22 characters of unpadded base64url rather than 32 of hex. Two ids ride on every line, so the shorter form trims a useful slice off a file that is read whole on every replay; it buys little in git, where both forms hold the same 128 bits and compress alike. An id pasted in the longer hex form is also accepted, since the two widths are exact and disjoint and nothing has to be sniffed. Ids are never re-encoded in place: an encrypted value derives its subkey from the tx_id and row_id strings and authenticates the row id, so rewriting an id from one form to the other would silently strand the ciphertext under it. Append-only makes that a non-issue in normal operation (GC rotation copies lines verbatim), but it rules out any tidy-up pass over historical ids.

Note the encoding does not sort in time order, since base64url is not ASCII-ordered. That costs nothing: replay orders by date first, and dates are nanosecond and strictly increasing per client, so the tx_id tiebreak only arises between two clients writing in the same nanosecond - where issue order is meaningless anyway. What the tiebreak has to be is identical on every client, and a string compare is. The raw bytes still climb, which is what keeps primary-key inserts at the b-tree tail.

A field declared `ref` holds a reference to another row's `id`, and holds it the same way the `id` itself is held - as the raw 16 bytes, not as their text form. So a reference joins directly against the key it points at, and costs 16 bytes rather than 22 or 32. The log still carries the value as ordinary id text, since the log is text; replay converts it on the way into the view, and reads convert it back, so nothing above the storage layer has to think about bytes. A `binary` field is also a blob, which is why the ref columns are tracked from the schema rather than inferred from the SQL type.

The built-in link tables (`many2many`, `comments`, `audit_trail`, `access_rows`) hold their `parent_id` columns the same way, so every key and every reference in the view is raw bytes and text appears only in the log. Their feature queries had to move with the columns: a blob column compared against a string parameter matches nothing and reports no error, so a half-done conversion reads as "no results" rather than as a failure.

That same trap is why a hand-written query gets two helper functions, `id(text)` and `idtext(blob)`. Ad-hoc SQL is a headline feature, and without them a perfectly reasonable `WHERE parent_id = '<id>'` would quietly return nothing; `id()` converts, and rejects a malformed id loudly rather than degrading to an empty result. They are registered on the driver, so any opened view has them.

Reads map columns by the header row's names, not by position, so field order does not affect compatibility. A column can be reordered, added, or dropped and older and newer clients still read each other's logs: an unknown extra column is ignored, and a column a record lacks (e.g. a pre-host_name row, or a narrower legacy header) defaults to empty. A record only needs enough fields to carry the required columns (through `user_id`); anything shorter is treated as torn. Header rows are recognized by carrying the reserved column names rather than by their first cell, so even the header can be reordered. tx_id remains the conventional first column.

`ok_to_garbage_collect` is populated automatically, when the original record (or table) referred to no longer exists. Only log records N-days old are garbage collected (by any client). Default is 90 days.

`host_name` is stamped automatically from the OS hostname (override with `NANOGITDB_HOST`), the machine analogue of `user_id`'s `NANOGITDB_USER`. It is txlog metadata only - replay does not apply it to the view.

On-disk encoding: every entry is exactly one physical line. Characters that would force CSV quoting or break line-based union merging (newline, return, comma, quotes, backtick, slashes) are stored as named markers - 🗦NEWLINE🗧, 🗦COMMA🗧, etc. - and a `new_value` of exactly 🗦NULL🗧 means SQL NULL (distinct from empty string). Literal 🗦/🗧 in content encode as 🗦LDELIM🗧/🗦RDELIM🗧, so user text can never spoof a marker. Other control characters are scrubbed. Decoding happens on read; the SQLite view always holds the real characters.

GC must preserve the append-only property the union merge depends on: collecting by rewriting lines in-place would make two clients' concurrent GC (or GC racing an append) a real merge conflict. So GC is segment rotation: a client that collects writes the surviving entries to a new log segment and retires the old one whole, rather than editing it. Replay also guarantees a hard-deleted row stays deleted even if its delete entry outlives the row's other entries: update entries sorted after a delete are skipped, only a later create revives the row id.

v1 GC (the `gc <ddl> <logdir>` verb): collectible = every entry of a row whose final state is hard-deleted and whose newest entry is older than `gc_age_days` (default 90); live, revived, and recently-deleted rows keep their history whole. Survivors land in a write-once `txlog-<stamp>-<rand>.csv` segment, the replaced files are removed, and new writes recreate `txlog.csv`; reads walk segments then the live file. Sync stages the whole log dir (so rotations - and the attachments folder - replicate), and `txlog-*.csv` carries the union merge attribute. Two clients collecting concurrently just duplicate survivors across two segments; replay is idempotent, so views converge and the next pass collapses them. Eligibility is computed at collect time; the `ok_to_garbage_collect` column stays in the format as a future cross-client signal.

### Scripting triggers

Scripting is an enterprise-edition feature. The core exposes a script seam (the same open-core shape as encryption); the enterprise build registers the Lua host, and the open-source build has none, so it ignores `code:` hooks and rejects `--script`.

The DDL's `code:` keys name Lua functions defined in a `.lua` sidecar next to the schema (same base name, like `.queries.shcl`); no sidecar means no triggers. The core wires the write-path hooks through the seam, so every front-end fires them identically:

- Field `before_update(table, field, value)` -> `pass [, new_value]`: an explicit `false` cancels the write; a second return replaces the value. Runs before the table-level hook, per the design's ordering note.
- Table `before_update(table, fields)` -> `pass`: sees the field hooks' output; explicit `false` vetoes the whole write.
- Field `after_update(table, row_id, field, value)` and table `after_update(table, row_id)`: fire after the commit lands; failures warn but cannot undo.
- App `before_open(name, logdir)` -> `pass`: `false` vetoes the open; `after_open` fires once triggers are live.
- Trigger code gets the same sandboxed `db.*` API as `--script`. Writes made while a trigger executes do not re-fire triggers (no reentrancy), so a hook updating another row cannot recurse.
- A `code:` key naming a function the sidecar does not define warns at open and never blocks writes. Replay never fires triggers - they ran on the originating client.
- Deferred: `before_sync`/`after_sync` (sync loop), `before_viewchange`/`after_viewchange`/`before_close`/`after_close` (UI lifecycle), and `validation:`'s `method:` key.

### Schema renames

`--rename-table` / `--rename-field` rewrite the DDL file in place (line-based, so comments and formatting survive; the old name is recorded on an `aliases:` line) and ALTER the SQLite view. The append-only tx-log is never rewritten: replay translates aliased table/field names to their current names, so history recorded under the old name keeps applying after any number of renames. First definition wins on conflicting aliases, like every other schema merge.

### Tunable options

- git_sync_frequency = 60  ## seconds between syncing the transaction log.
- gc_age_days = 90  ## only log records older than this are garbage collected.

v1 notes: tunables live in a `tunables:` DDL section; both `key: value` and the `key = value` form above parse. Unknown keys warn but still store (a newer client's tunable survives). `git_sync_frequency` paces the background sync loop the TUI and web UIs start when the log dir is a git work tree - each pull that brings new entries full-replays into the view (the safe apply until incremental apply lands); 0 disables the loop. `gc_age_days` is consumed by tx-log GC when that lands.

### Startup discovery and database registry

A database is addressed by the name it was registered under: every CLI verb and `--tui`/`--serve` takes that name as their target and resolve the `ddl`/`sqlite`/`logdir` from the registry record, so those paths never appear on the command line (CRUD is name-only by design; the low-level `build`/`replay`/`sync`/`gc` verbs keep an explicit-path form for pre-registration use). The name resolves with or without a file extension, and may be given positionally or via `--db`/`--table` flags.

How a run finds a database when it was not handed a name:

- A lone `*.shcl` in the current directory is used directly (the sqlite view defaults beside it, the tx-log dir is that directory). Zero or several schema files are ambiguous, so discovery falls through - as is a directory holding both a `.shcl` and a legacy `.ddl`. The queries sidecar is excluded from that count by its `.queries.shcl` suffix, so a schema and its sidecar are not mistaken for two candidates.
- Otherwise the interactive front-end (bare `ngdb` or `--tui`) shows a registry picker; the non-interactive ones (`--serve`, CLI verbs) require a name and error otherwise (no picker).

The registry is per-database SHCL records under a config base: `<os-user-config>/ngdb/<name>/config.shcl`, where the base is `$XDG_CONFIG_HOME` (else `~/.config`) on Linux, `%AppData%` on Windows, `~/Library/Application Support` on macOS - whatever `os.UserConfigDir` returns. Read-only system bases (`$XDG_CONFIG_DIRS`, default `/etc/xdg`; `%ProgramData%` on Windows) are also searched, after the user base. A record holds: `name`, `ddl_path`, `log_dir` (the git-synced artifacts), `sqlite_path`, `key_file` (local, unsynced; both default beside the record and are rebuildable / re-fetchable), `encryption` and `last_opened`. `key_file`/`encryption` are stored for a stable format ahead of the encryption feature.

Discovery lists every record across the bases, name-sorted, and lightly validates each: its DDL must exist and parse. A record that fails still appears in the picker, flagged `[!] <name>` with the reason (missing/corrupt DDL), so a broken database is visible rather than silently dropped. A missing sqlite view is never an error - it rebuilds from the log on open. The picker always offers "Create new database" (register a new record pointing at a DDL + tx-log, then open) and "Open existing ..." (open a DDL + tx-log ad-hoc, without registering). Opening a registered database stamps its `last_opened`.

The `--init`, `--config`, and `--encrypt` CLI flags drive the same registry from the command line (see the CLI/encryption sections). DDL-authoring for a truly-new database remains a separate, later item.

### UI

- When a view specifies `startup_named_query`, that named query's dataset loads as soon as the view opens. Only when it is empty or unspecified does the view open with no records shown - then you have to query, e.g. via "All" button, or via predefined query dropdown.
- The default view (`ui:` -> `default_view`, else the first view defined) opens on startup in both UIs. Its blocks still load empty per the no-records-until-asked rule; a `default_view` naming an unusable view warns and falls back to the first one.
- View rendering (v1): the DDL's layout blocks render as nested splits in both UIs (TUI flexes, web flexbox). A location hint's direction and percent set each split's axis and share; the relative-to element is ignored for now (blocks place in DDL order). Leaf blocks are `grid`, `tree_grid` (rows ordered depth-first along `parent_field`, indented by depth; orphaned or cyclic parents degrade to extra roots rather than hiding rows), `form` (single-record panel; shows the first record until block linking exists), or `comments` (a detail pane over the table's built-in comments component - it follows a sibling list block's selected row, listing that row's thread with an add affordance, and stays empty until a row is picked). The comments pane surfaces the 1:m `comments` feature a table opted into, without ever adding a column to the list view; a `comments` block over a table that has no comments feature is dropped with a warning. Blocks over unknown tables are dropped with a warning, a bad `tree_grid` degrades to a plain grid, and `readonly` (view-level, overridable per block) removes the edit affordances. Editing from a web view block currently jumps to the table's form; returning into the view is future polish.
- Field UI metadata (spec settled 2026-07, implementation waits for the shcl syntax migration; current vocabulary in [example.shcl](example.shcl)):
	- `visible` splits into `visible_form` and `visible_list`. Presentation only, never access control - a hidden field stays fully readable via CLI/query/SQL; the `access:` lists are the only thing that gates data.
	- `title` becomes `label`; `list_type` values are `literal|sql|lookup` (`sql` replaces `dynamic`); `lookup` wires a field to the built-in lookup tables (see Lookups).
	- A DDL field entry naming a system field (`id`, `is_active`, `date_created`) merges its `ui:` block onto it instead of being dropped. Presentation-only merge: `type:`, `validation:`, `defaultval:` on a system field warn and are ignored - system fields stay structurally immutable, presentationally customizable.
	- `defaultval` takes three forms: a static value, a sentinel from a closed set (`@null`, `@previous`), or a script function `"fFunc()"`. The `@` marker was chosen over brackets because `[` `]` are reserved value characters in shcl. Quoting escapes a sentinel, so `"@null"` is the literal text. `@previous` (value from the previously entered row, this session) applies in interactive front-ends only - programmatic writes never inherit a sticky session value; an omitted field is null or a required-field error.

### Predefined queries

Plain text file, similar structure to DDL, e.g.:

~~~text
query_name: "Some name"
	view:		"name of view"
	sort:       5.5
	active:     yes
	SQL:        `SELECT * FROM table_name WHERE field1 = 'Joe;'`
~~~

v1 notes: the file is a sidecar to the schema - same base name, `.queries.shcl` extension (`example.shcl` -> `example.queries.shcl`); missing file just means no queries. Grammar and load validation match the schema (nameless and no-SQL entries drop with a warning; two queries sharing a name are one query, per the merge rule). SQL belongs in a raw block so its commas survive. `view:` scopes a query to one view's dropdown; omitting it offers the query on every view; `active: no` hides it everywhere; dropdown order is by `sort`, file order within ties. Both UIs run a picked query into the view's first block (TUI: 'p' picks, and the dataset renders with the query's own columns; web: dropdown + Run above the blocks). A view's `startup_named_query` that resolves to an active query loads on open per the UI section; an unresolvable name degrades to the normal empty open (with a status note in the TUI when a queries file exists).

### Optional granular access model

Hierarchical access: View -> View's main table -> table -> { field | record }

v1 semantics (everything is open until a DDL populates a list):

- Identity: the write-stamp user (NANOGITDB_USER / OS username) resolved to a live users row by screen_name or github_name; group membership comes from the users<->groups m:m. No user row = no memberships, which with empty lists keeps a userless setup fully working.
- Rule evaluation: blacklist wins over whitelist; an empty whitelist means everyone. Built-in overrides per the DDL comments: owners always pass, admins pass read/write (delete still needs the rule). `mark_delete` and `delete` use the delete rule; create/update use write.
- Write enforcement is in the core CRUD API (all front-ends inherit it): table write/delete rules, field write rules on the touched fields, and - for row_level_access tables - the row's access_rows grants (a granted row needs a shared group; ungranted rows are open).
- Read enforcement is UI-level, computed at catalog build since a process serves one user: unreadable tables vanish from the UIs (with a warning), unreadable fields drop out of grids and forms silently, a view is hidden when its flat access rule fails or its main (first) table is unreadable, and row-granted rows the user lacks are filtered from grids. Raw `query` (CLI/Lua/SQL) is not row/table-gated - the local file is the trust boundary; the write path is the hard gate.

### Startup notice and read-only mode

Two small, general-purpose pieces the core provides so an edition can surface a notice at startup without the core knowing anything about why:

- A neutral `gate` seam, the same open-core shape as the encryption and scripting seams. It carries only a posture, a message, and an optional wait - Full (run normally), Nag (run, but show the message as a banner), or Blocking (hold on a start screen the user either waits out or dismisses). The core knows how to draw those three; it never learns what drives them.
- A generic read-only session flag in the core write funnel: one guard on the single commit path every write goes through, so setting it makes every front-end (CLI, TUI, web) refuse writes while reads keep working. A dismissed Blocking notice lands here.
- The open-source build registers no provider, so the seam always reports Full and the app runs unimpeded. Among the ways to shape this, it was decided to keep the seam neutral and put only the rendering here: any decision logic an edition adds lives on its side, never in the readable public core.

### Web login

The web tier binds `127.0.0.1` only, so on a single machine the loopback binding is the access control and no login is needed. Putting it behind a reverse proxy changes that, so which shape applies is an explicit setting, never guessed (a proxy also connects from loopback, so the server cannot tell the two apart on its own).

- `web_mode` (user-global setting, default `local`): `local` is the passwordless single-user shape; `proxied` requires a login. Any value but the exact word `proxied` falls back to `local`, so a typo fails toward the guarded-but-passwordless mode rather than a broken login wall.
- Local mode identifies the one user with no password - the git account of the log dir's repo, else the OS user (the same default-user resolution the other front-ends use) - and stamps every request as them. Safety-net: if a request ever carries a reverse-proxy header (`X-Forwarded-For` / `X-Real-Ip` / `Forwarded`) while in local mode, the server refuses to serve. A box accidentally exposed behind a proxy can never run passwordless; the operator must opt into `proxied` and add a login.
- Proxied mode requires a session for every path but the login endpoints and static assets. A login checks a username and password against a local credentials file, then sets a random session cookie (HttpOnly, SameSite=Lax, Secure when the request arrived over TLS); the session table is in memory, so a restart just re-prompts. The logged-in user is set as the acting user for that request, so the existing user/group access model applies to the web view exactly as it would elsewhere. Because that acting user is shared state, proxied requests are serialized - this is a local UI lightly extended to multi-user, not a high-throughput service.
- Credentials: a `webusers.shcl` of PBKDF2-HMAC-SHA256 hashes (stdlib `crypto/pbkdf2`, so no new dependency; random per-password salt, constant-time compare), kept in the config dir OUTSIDE any git-synced tree - password hashes must never ride along in the shared log repo. The `webuser` CLI verb adds or replaces a login (password from `NGDB_WEB_PASSWORD` or a prompt). Stronger methods - 2FA, passkeys, SSO - are an enterprise concern; this is the open-source baseline that makes the server safe to put behind a proxy at all.

### Donations

`--donate`, the TUI Support entry, and the web Support page show a short appeal and one link - the project's `DONATE.md`, which lists the ways to help (GitHub Sponsors and other platforms, wired through `.github/FUNDING.yml`). The in-app copy is a single small source file the front-ends only display; no addresses are embedded in the binary.

It is an open-source-only feature: `donate` is a top-level seam with an `Enabled` switch, and the enterprise (commercial) build turns it off before dispatch, so the paid product carries no `--donate` flag or Support screen.

Where support goes is decided by four files - `DONATE.md`, `.github/FUNDING.yml`, `.github/CODEOWNERS`, and the small `donate` source - and `CODEOWNERS` makes them maintainer-owned, so a pull request cannot quietly redirect donations without the maintainer's review. An earlier scheme instead embedded a crypto-address table and signed it with an out-of-repo key; that is set aside for now in favor of these simpler GitHub-native funding files.

### Encrypted transaction log

Optional at-rest encryption so the git-synced log is unreadable to the hosting provider or anyone who obtains the repo. The local SQLite view always holds cleartext (decrypted at replay), so queries, indexes, and uniques are unaffected.

- What is encrypted, and what leaks: only the field VALUE (`new_value`). Every other column - tx_id, date, table_name, row_id, field_name, operation, user_id, host_name, gc flag - stays cleartext, because git's line-union merge and the deterministic (date, tx_id) replay order must both work without a key. So a repo holder can see the schema shape, row ids, who/when/which-host, and row counts, but not the values. This leakage is inherent to keeping a mergeable text log; obscuring table/field names is a possible future hardening (it needs a synced name<->token map).
- Cipher (crypt package): AES-256-GCM under a per-value subkey derived (HKDF-SHA256) from the entry's unique tx_id (plus table/field for domain separation). Every log entry carries a fresh UUIDv7 tx_id, so each subkey encrypts exactly one value - nonce reuse is impossible regardless of record count, and GCM's random-nonce counting limit never applies. A random 12-byte nonce is stored with the ciphertext; the cleartext context (table/field/row) is bound as AEAD additional data, so editing those columns in the repo makes decryption fail rather than silently mis-attributing a value. Stdlib-only (crypto/aes, crypto/cipher, crypto/hkdf, crypto/rand) - no cgo, no new dependency.
- On-disk form: an encrypted value is stored as the `🗦ENC🗧` marker followed by base64url(nonce || ciphertext || tag), one physical line like everything else. A literal user value equal to the marker is escaped by the symbol encoder, so cleartext can never spoof it (the same collision-safety the NULL sentinel uses). Clear and encrypted values coexist per-field in one log, so encryption can be turned on or off at will without rewriting history.
- Key: one 256-bit random key per database, base64 in a text file (mode 0600) kept OUT of the repo - in the local config dir (`DBConfig.KeyFile`, default beside the record) for a registered database, or beside the DDL for an ad-hoc open. Users exchange it out-of-band. `--init --encrypt=on` mints it; it is never regenerated over an existing key (that would strand old data).
- Policy, two layers. DDL `encryption: always|never|auto` at database/table/field (shared, in git): scanning outermost-first, the first always|never locks - a lower level cannot override it (only a higher one can); all-auto defers. Local `--encrypt=on|off|auto` (per-host, in the unsynced registry record): the persistent per-user preference; auto = encrypt-if-key-present. Resolution per field: a DDL always/never wins; otherwise on -> encrypt (clear if no key), auto -> encrypt only if a key is present, off -> clear.
- Degraded and enforced modes: writing an `always` field with no key is refused (the whole write aborts - nothing half-committed). Reading without the key shows encrypted fields empty (the decrypt pass leaves them Enc-marked and Apply binds NULL - the view never holds ciphertext) and warns. Having the key but `--encrypt=off` warns too, for awareness. GC needs no key: it rotates encrypted entries as opaque lines.
- Wiring: encryption happens on the write path (crud commit seals the log copy while the view gets cleartext) and the decrypt pass runs before replay (schema.OpenClientWith / cli replay). All four front-ends inherit it through the shared bring-up.
- Deferred (documented, not built): encrypting the schema / `.queries.shcl` / `.lua` FILES themselves with decrypt/re-encrypt flags; the `ddl:` / `named_queries:` / `config:` file-level encryption labels; and the futures - multiple keys, public/private keys, passphrase-derived keys, and bulk re-encryption under a new key.

### Tables created automatically at new startup (and verified in the background every startup)

Unless otherwise noted, all tables that are automatically created at startup, inherit "## Fields that all tables get, always and immutably".

Core tables like Users, Groups, etc. also inherit things like audit trail, comments, attachments if appropriate, and necessary m:m relationships. Etc.

These tables, the ones are automatically managed, have `cascade_delete` enabled if appropriate.

These are defined in a single source code file (not end-user editable), with the [same schema language](example.shcl) that users use.

#### Always created and used

##### Users

Populated automatically. User can edit own, "admins" can edit, "owners" group can delete.

- id
- screen_name  ## Defaults to github name
- github_name
- email
- phone
- avatar [image blob]

Has audit trail, comments, and attachments enabled.

##### Groups

Default records that get populated on db creation:

- "owners"  ## First user in user table is part of.
- "admins"  ## First user in user table is part of.
- "users"  ## Regular users
- "guests"  ## Heavily restricted, mostly read-only.

Has audit trail, comments, and attachments enabled.

DB necessarily has this DDL defined implicitly:

~~~text
relationships:
	relationship:  ## auto-named
		type:  m:m
		parent: users
		child: groups
		cascade_delete: y
		enable_audit_trail: y
~~~

##### Lookups

Two built-in tables, created for every database, give any field a data-driven pick list without a schema change: `lookups` (the groups) and `lookup_values` (the members: title, symbol e.g. a single emoji, description, sort, is_active). Decisions made here:

- Unlike the other built-ins, these are meant to be user-edited in the UI, so front-ends list them like normal tables.
- Both keep the invisible system `id` (row identity in the tx log, untouched machinery) and add `idx`: a unique, read-only, auto-assigned (max+1) int. `idx` is the reference key - a field's `list_source` names a `lookups.idx`, and the field stores the picked `lookup_values.idx`. We chose a meaning-free int over a name (retitling a value or group never orphans data) and over a UUID (compact and legible in the DDL and log).
- The DDL can carry seed rows for these tables (`seed:` section). Seeding is idempotent and log-first at open: a seed row inserts only when its idx is absent; runtime edits to a seeded row win over the DDL forever after. Seeding via the DDL is the preferred way to mint values - idx conflicts then surface as ordinary git merge conflicts. Two offline clients minting values at runtime can collide on idx; the unique index drops one at replay with a warning.
- "Value must exist in lookup_values" is enforced at write time only, never at replay - a value legitimately picked while active must still apply after later deactivation.
- Deactivated values (`is_active: n`) can't be picked for new rows, but existing rows still render their symbol/title.
- Display is controlled per field by two enums, `lookup_in_lists` and `lookup_in_forms`, each `symbol|title|both`: lists default to symbol (falling back to title when a value has no symbol), forms default to both, rendered symbol then title. Hiding entirely is the job of the `visible_*` keys, not these.
- TUI note for implementation: emoji symbols mean double-width and occasionally unrenderable glyphs; the symbol column needs width handling, with title fallback. The web UI has no such problem.

#### Automatic features that any table can opt-in to

v1 behavior notes (all writes ride the normal log-first path, so features replicate like data):

- Audit capture happens in the core CRUD API, so every front-end gets it; the audit entries land in the same commit as the data change. "One record for all updates while active for a user" is implemented as: consecutive updates by the same user merge into that user's newest update record (keeping the oldest old-value per field); any other action or another user's update starts a new record. `values` doubles literal quotes (CSV-style) so old values containing quotes stay unambiguous. A hard delete snapshots the row's remaining values into its audit record.
- Copied attachments land in `<logdir>/attachments/<row id>` so they ride the same git sync as the tx-log. The copy is plain io.Copy, which on Linux already behaves like `cp --reflink=auto` (Go copies via copy_file_range(2), which reflinks where the filesystem can).
- Row-level access grants (access_rows + m:m to groups) are writable data now; ENFORCEMENT is the separate granular-access-model item.
- The Lua sandbox exposes comments/URI attachments but deliberately not file attachments - scripts have no file access, and attach-file would be a read-any-file hole.

##### Many-to-many relationships

Can be "opted-in" via the DDL. All relationships stored in one table, "many2many". Fields.

- id  ## Only useful for things like audit trail.
- table_name_1
- table_name_2
- parent_id_1
- parent_id_2

Unique constraint: table_name_1 + table_name_2 + parent_id_1 + parent_id_2 + is_deleted

##### Comments

Multi-row comments are an automatic feature that can be enabled in the DDL. Stored in a single SQL table. Fields:

- id
- table_name
- parent_id
- comment

##### Audit trail

An automatic feature that can be enabled in the DDL. Stored in a single SQL table. It's similar to the transaction log, but only one record exists for all updates to a record while it is active for a user. Fields:

- id
- table_name
- parent_id  ## Row's value of `id` from `table_name`.
- user_id
- date  ## Stored GMT, displayed local time.
- action  ## create, update, mark_delete, delete
- values  ## field1:"old value" field2:"old value"  ## Only list fields that had old values, that changed or were deleted.

This table is excluded from the "additional fields that get added to every table". These are the only fields.

##### Row-level group access

Main access table access_rows:

- id
- table_name
- parent_id  ## from specified table_name

If table_name has audit-trail enabled, then so are the records here belonging to `table_name`. (This applies universally, not just to this table.)

DB necessarily has this DDL defined implicitly:

~~~text
relationships:
	relationship:  ## auto-named
		type:  m:m
		parent: access_rows
		child: groups
		cascade_delete: y
		enable_audit_trail: y
~~~

##### Attachments

###### URI

URI attachments are an automatic feature that can be enabled in the DDL. They can either be referenced in-place via local path or URL, or copied and stored with the repo. (Two different tables.)

Table attachments_uri:

- id
- uri
- description

With a m:m implied between attachments_uri and whatever table has opted in to the feature.

###### Copies

Copied attachments are an automatic feature that can be enabled in the DDL. The file is copied to the same base path as the transaction log, under a subfolder `attachments`, and given the same name as the row ID (with no extension).

Table attachments_copy:

- id  ## Stored filename gets named this.
- name  ## Doc name, default to original document base name.
- description
- og_name  ## Full original filename
- extension  ## original extension

With a m:m implied between attachments_copy and whatever table has opted in to the feature.

Access model implied. Comments, etc.

When opening, a copy is made to local user tmp directory, and given `name_<copy date YYYYmmDD-HHMMSS user local time>.extension`.

Anytime a file is copied, the program should try the internal version of `cp --reflink=auto` (e.g. by API) if available.

### CLI arg-based API

Full crud CLI UI, plus:

- `--rename-table "old" "new"`  ## Renames in DDL and SQLite, and adds aliases in the DDL
- `--rename-field "old" "new"`

### Internal API

If external scripting files can be hosted in-executable, then it stands to reason an internal API should be exposed.

### Scripting

An external file stores functions for DDL-definable triggers, events, methods, etc.

Decided: **Lua** via pure-Go `gopher-lua`, hosted in-executable. Scripts get a `db` global bound to the core CRUD API - `db.create/get/update/setfield/markdelete/delete/query` - so scripting shares the exact code path as every other front-end. The host is sandboxed: only base/table/string/math/coroutine are opened (no os, io, package, or debug). Reachable now via `ngdb --script <script.lua> <ddl> <sqlite> <logdir>`; DDL-wired triggers/methods build on this later.

Original requirements (all met):

- Well-known

- Easy syntax

- Access to powerful programming paradigms even if not used.

- Ideally able to be hosted by the executable. But worst-case, can be invoked externally, and functions would have to call back into the executable with the CLI arg-based API.
