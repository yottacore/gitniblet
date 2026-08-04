<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->

<!-- TOC ignore:true -->
# Nano Git DB - full syntax reference

The complete DDL, sidecar-file, CLI, and run-mode reference. For a five-minute introduction see the "Quick start" section of the [README](README.md); this file is the details behind it. Everything here is what the current build actually does - a few DDL keys shown in `project/example.shcl` are design placeholders whose behavior is still evolving; where that matters it is called out below.

<!-- TOC -->

- [Schema file format](#schema-file-format)
	- [Special values](#special-values)
- [Tables](#tables)
	- [Fields](#fields)
	- [Field types](#field-types)
	- [System columns](#system-columns)
	- [Uniques and indexes](#uniques-and-indexes)
	- [Opt-in table features](#opt-in-table-features)
	- [Access control](#access-control)
	- [Encryption](#encryption)
	- [Aliases and renames](#aliases-and-renames)
- [Relationships](#relationships)
- [Views](#views)
- [Tunables](#tunables)
- [Sidecar files](#sidecar-files)
	- [Named queries (.queries.shcl)](#named-queries-queriesshcl)
	- [Triggers and code (.lua)](#triggers-and-code-lua)
- [Command-line interface](#command-line-interface)
- [Run modes](#run-modes)
- [Startup discovery and the database registry](#startup-discovery-and-the-database-registry)
- [Environment variables](#environment-variables)

<!-- /TOC -->

## Schema file format

The schema is an [SHCL](https://github.com/jim-collier/shcl) file (`*.shcl`) - a small indent-nested `key: value` language, also what ngdb writes its own config in. The grammar in brief:

- **Indent to nest.** A line indented further than the line above is that line's child. Tabs are the convention here; what matters is that each level's indent is a prefix of its children's. A line whose indent matches no open level is reported and skipped, and the rest of the file still loads.
- **`key: value`** on one line, or **`key:`** with indented children below it. Quote a value (`"like this"`) when it contains a comma or matters at the edges.
- **Comments** start with `#` and run to end of line, on their own line or trailing a value. (This project's files use `##` by convention - that is just a comment whose first character happens to repeat.)
- **Repeating a key makes an instance.** `table:` appears once per table and `field:` once per field; the value tells them apart. Two entries with the *same* name and value are the same node and their children merge, which is what lets you re-open `database:` or `tables:` further down the file instead of scrolling back to the right indent. The flip side: **two tables with the same name are one table**, the second entry's fields folding into the first. Any such combining is reported at load with both line numbers, so it is never silent - if the report is about a section you re-opened on purpose, it is telling you the fold worked; if it names a table you meant to be new, you have two schemas fighting over one name.
- **Path shorthand.** `database.relationships.relationship: r1` is the same as the nested form. Use it for a one-off rather than indenting four levels.
- **Wrappers are transparent.** `database:` (around `tables:`/`relationships:`) and `ui:` (around `views:`/`default_view:`) are organizational; a flat schema without them parses the same.
- **Lists** are comma-separated (`aliases: old_task, older_task`), or one `* item` per line.
- **Raw blocks** hold a value verbatim: three or more backticks or tildes, optionally with an info string, on the lines below the key.

	```
	list_source:
		~~~sql
		SELECT idx, title FROM lookups ORDER BY title
		~~~
	```

	Reach for one whenever a value contains commas or quotes - SQL and regexes especially, since an unquoted comma would otherwise split the value into a list. A single-line backtick value (`` `SELECT ...` ``) reads verbatim too.

Loading is deliberately forgiving. Nothing a schema file can contain aborts the load: a malformed line is skipped, an unknown key or an out-of-range value is reported against its line number, and cross-cutting problems (a `unique:` naming a field that does not exist) name the entity instead. Nameless or system-colliding fields are dropped and an unknown field type stores as text, each with a warning. Warnings surface on the command line and in the front-ends; a schema carrying outright *errors* still parses, but the registry marks that database unopenable rather than opening a half-built view.

The key vocabulary is itself an SHCL schema, shipped inside the binary, so a mistyped key (`defaultvalue:` for `defaultval:`) is named with its line number rather than silently ignored.

A minimal schema is just a table with fields:

```
database:
	tables:
		table: task
			fields:
				field: title
					type: string
				field: status
					type: string
```

### Special values

- **Booleans** - `true`/`false`, `yes`/`no`, `on`/`off`, `1`/`0`, and also `y`/`n`, `t`/`f`, `enable[d]`/`disable[d]`.
- **Numbers** - `10`, `.1`, `1.`, `1.0`, `0x1f`. A leading currency symbol or a trailing `%` is accepted and stripped, and a fraction where a whole number is wanted rounds rather than failing.
- **Sentinels** - `@null` and `@previous`, in `defaultval:` position. `@null` means no default. `@previous` repeats the value from the last row entered this session, and only in the interactive front-ends - a programmatic write never inherits it. To mean the literal text instead of the sentinel, quote it: `defaultval: "@null"` stores the five characters.
- **Function refs** - `some_function()` names a script function to call rather than a literal to store.
- **SQL and regexes** - use a raw block or single-line backticks, so commas and quotes survive intact.

## Tables

A `table:` has a name and a `fields:` block, plus optional `uniques:`, `indexes:`, `features:`, `access:`, `code:`, and `aliases:`.

### Fields

Each `field:` has a name and, at minimum, a `type:`. Other per-field keys the current build honors:

- `type:` - see below.
- `aliases:` - former names, so a renamed field's history still replays (see [Aliases and renames](#aliases-and-renames)).
- `access:` - per-field read/write rules (see [Access control](#access-control)).
- `code:` - `before_update` / `after_update` trigger hooks (see [Triggers](#triggers-and-code-lua)).

Additional keys appear in `project/example.shcl` (`ui:`, `validation:`, `special:`, `defaultval:`, widgets, formats). These parse but their runtime behavior is still being built out; treat them as forward-looking until a release note says otherwise.

### Field types

| Type | Stored as | Notes
| :-- | :-- | :--
| `string` | text | The default when `type:` is blank.
| `int` | integer |
| `float` | real |
| `bool` | integer | 0 / 1.
| `datetime_local` | text | UTC on disk, shown in the user's local time.
| `datetime_utc` | text | UTC.
| `datetime` | text | Alias handling for `datetime_local`.
| `binary` | blob |
| `ref` | blob | A reference to another row's `id`. Holds the same raw bytes the `id` does, so it joins directly against it; you still read and write it as the usual id text.

Anything else stores as text with a warning, so a typo never breaks the load.

### System columns

Every table automatically gets these; you do not declare them:

- `id` - the row's primary key, a GUID. Stored as raw bytes; reads render it as 22 characters of base64url (that is the form you pass back to `get`/`update`), and the longer 32-character hex form is also accepted as input. First column, unique, indexed.
- `is_active` - defaults to on.
- `date_created` - set at insert.
- `is_deleted` - hidden; set by a soft-delete (`markdelete`), and folded into every unique index so a deleted row does not block re-creating its natural key.

A table can opt out of these with `system_fields: no` (used internally by the audit trail); such a table gets exactly the columns you declare.

### Uniques and indexes

```
			uniques:
				unique: title, parent_task    ## a composite unique (also indexed)
				unique: slug                  ## and another
			indexes:
				index: status                 ## a plain index
```

Each constraint is its own `unique:` or `index:` entry; the fields in one entry are a comma-separated list. Unique constraints are partial (they ignore soft-deleted rows) and auto-named `ux_<table>__<fields>`. Declaring an index identical to a unique warns (the unique already indexes it).

### Opt-in table features

```
			features:
				comments: yes
				local_attachments: no
				uri_attachments: no
				audit_trail: no
				row_level_access: no
```

- `comments` - each row gets its own list of related comment rows.
- `uri_attachments` - attach link-in-place URIs to a row.
- `local_attachments` - copy a file into `<logdir>/attachments/<id>` and attach it (replicates through the log).
- `audit_trail` - capture field-level changes into the `audit_trail` table, in the same commit as the change.
- `row_level_access` - enable per-row grants to user groups (see [Access control](#access-control)).

Features are off unless set. The front-ends surface only the features a table enabled.

### Access control

Open until configured: with no users/groups set up, everything is allowed. Once populated, identity is the write-stamp user resolved through the built-in `users` table, and group membership through the users<->groups link.

```
			access:
				read:
					whitelist:     ## empty = everyone
					blacklist:     ## empty = no one
				write:
					whitelist:
					blacklist:
				delete:
					whitelist:
					blacklist:
```

Rules: blacklist wins over whitelist; an empty whitelist means everyone. `owners` always pass; `admins` pass read and write but not delete. The same block can appear on a `field:` (read/write only) and on a `view:` (a single whitelist/blacklist gate).

Writes are enforced in the core API, so no front-end can bypass them. Reads are pruned at the catalog: an unreadable table or view disappears, unreadable fields drop from grids and forms, and ungranted rows filter out of row lists. The raw SQL `query` verb stays open by design - the local file is the trust boundary; the write path is the hard gate.

### Encryption

Optional at-rest encryption of field values in the git-synced tx-log, so the hosting provider (or anyone who gets the repo) cannot read your data. Only field values are encrypted; the local SQLite view always holds cleartext, so queries and uniques are unaffected. Everything else in the log - table and field names, row ids, user, host, timestamps, row counts - stays readable (that is what lets git merge and replay work without a key).

Turn it on with `--init --encrypt=on`, which mints a per-database 256-bit key file (kept out of the repo, in your config dir) and records the preference. Share the key out-of-band; never commit it.

Per-level policy with the `encryption:` key, at the database, table, or field level:

```
database:
	encryption: auto          ## database-level default
	tables:
		table: person
			fields:
				field: name
					type: string
				field: ssn
					type: string
					encryption: always   ## never stored in the clear
				field: public_id
					type: string
					encryption: never    ## always stored in the clear
```

- `always` - the value is always encrypted; a lower level cannot override it. Writing such a field with no key is refused; reading it without the key shows it empty.
- `never` - the value is never encrypted (e.g. a field a log preprocessor must read); a lower level cannot override it.
- `auto` (the default) - defers to your local `--encrypt` preference, and can be overridden higher or lower in the hierarchy.

Resolution scans database -> table -> field outermost first: the first `always`/`never` wins; if all are `auto` your local preference decides (`on` = encrypt, `off` = clear, `auto` = encrypt only if a key is present). Without the key, encrypted values read as empty and you are warned; you can still read and write cleartext fields.

### Aliases and renames

`aliases:` lists a table's or field's former names. The `--rename-table` / `--rename-field` CLI ops maintain them for you: they rewrite the schema file, `ALTER` the SQLite view, and add the old name as an alias. Replay then translates old names in historical log entries to the current names, so the append-only log is never rewritten and pre-rename history still rebuilds a correct view.

A rename re-emits the schema in SHCL's canonical form. Your comments and the blank lines between sections come through untouched, but hand-tuned column alignment does not - `type:          string` comes back as `type: string`. Once a file has been through one rename it is already canonical, so later ones leave it alone.

## Relationships

Relationships are optional; two tables can relate through a plain id field without declaring anything. Declare one when you want cascading deletes or a many-to-many.

```
database:
	relationships:
		relationship: task_subtask
			type: 1:m
			parent: task
			child: subtask
			parent_id_field: parent_task   ## indexed automatically
			cascade_delete: yes
		relationship: task_tag
			type: m:m                       ## uses the built-in many-to-many table
			parent: task
			child: tag
			cascade_delete: no
```

Name each relationship. The name is only a label, but it is what tells two `relationship:` entries apart - leave them both blank and they merge into one.

## Views

Views are the UI layouts, under `ui:` -> `views:`. Each `view:` has a `layout:` of nested `block:`s; `default_view:` (or the first view defined) opens on startup.

```
ui:
	views:
		view: "tasks"
			readonly: no
			startup_named_query: "All open"   ## optional; loads this named query on open
			layout:
				block: "list"
					table: task
					type: tree_grid
					parent_field: parent_task
	default_view: "tasks"
```

Block keys:

- `table:` - the table the block shows.
- `type:` - `grid` (flat table), `tree_grid` (hierarchical, ordered depth-first along `parent_field`, indented by depth), `form` (single-record panel), or `comments` (a detail pane over the table's `comments` feature - it follows a sibling list block's selected row and lists that row's thread with an add affordance; the comments never appear as a list column).
- `parent_field:` - for `tree_grid`, the field holding a row's parent id (a self-reference). Rows with an empty or unknown parent are roots; orphans and cycles degrade to extra roots rather than hiding rows.
- `location:` - `<relative-to>, <above|below|left|right>, <percent>` - sets a split's axis and share. The direction and percent are honored; the relative-to element is currently ignored (blocks place in DDL order).
- `readonly:` - view-level, overridable per block.

Blocks open empty (the "no records until asked" rule); the UI's "All" action or a named-query dropdown fills them, and `startup_named_query` fills one on open. A block over an unknown table is dropped with a warning; a bad `tree_grid` degrades to a plain grid; a `comments` block over a table without the comments feature is dropped with a warning.

## Tunables

```
tunables:
	git_sync_frequency: 60     ## seconds between background git syncs; 0 disables
	gc_age_days: 90            ## only log entries of rows deleted longer ago are collected
```

Unknown tunables warn but are stored (a newer client's tunable survives a round-trip). `git_sync_frequency` paces the background sync loop the TUI and web UI run when the log dir is a git work tree.

## Sidecar files

Two optional files sit next to the schema, sharing its base name.

### Named queries (.queries.shcl)

`schema.queries.shcl` (same base name as `schema.shcl`, `.queries.shcl` extension). Same grammar as the schema:

```
query_name: "All open"
	view:   "tasks"     ## optional; a query with no view is offered on every view
	sort:   1           ## dropdown order
	active: yes         ## no = defined but hidden
	SQL:
		~~~sql
		SELECT title, status FROM task WHERE status = 'open' AND is_deleted = 0
		~~~
```

Put the SQL in a raw block (or single-line backticks) so its commas and quotes are taken verbatim. The UIs offer active queries in a per-view dropdown; a view's `startup_named_query` names one to load on open.

### Triggers and code (.lua)

Scripting is an enterprise-edition feature. The open-source build ignores the `code:` hooks and rejects `--script`; the enterprise build runs them.

`schema.lua` (same base name, `.lua` extension) holds the functions named by the DDL's `code:` keys. The enterprise build fires:

- Field `before_update(table, field, value)` -> return `false` to cancel, or a second value to replace it. Runs before the table hook.
- Table `before_update(table, fields)` -> return `false` to cancel.
- Field / table `after_update` - fire after the commit.
- App `before_open` -> return `false` to cancel the open; app `after_open`.

Trigger code gets the sandboxed `db.*` CRUD API (no `os`/`io`, no `dofile`). Writes made from inside a trigger do not re-fire triggers, so hooks cannot recurse. Replay never fires triggers. A `code:` key naming a function the sidecar does not define warns instead of blocking. Other hooks in `example.shcl` (`before_sync`, `viewchange`, `validation: method:`) are not wired yet.

## Command-line interface

Every verb names a registered database as its first argument - `<db>` below (extension optional). Register one with `--init`; ngdb looks up its schema, view, and log from the registry, so those paths never appear on the command line. Run `ngdb` with no matching verb to print this usage.

Schema and log:

```
ngdb ddl <file>       parse a DDL file and print a summary
ngdb build <db>       build / migrate the SQLite view
ngdb replay <db>      rebuild the view from the tx-log
ngdb sync <db>        sync the tx-log via git, then migrate and replay
ngdb gc <db>          collect entries of long-deleted rows (gc_age_days)
```

Data (each takes a `<db>`, then a `<table>`):

```
create <db> <table> f=v [f=v ...]      insert a row; prints its id
get <db> <table> <id>                  print one row
update <db> <table> <id> f=v [f=v ...] set fields
setnull <db> <table> <id> <field>      set a field to SQL NULL
markdelete <db> <table> <id>           soft-delete
delete <db> <table> <id>               hard-delete
query <db> <sql>                       read-only SQL against the view
```

The database and table may also be given as flags in any order instead of positionally: `--db=<name>` / `-d <name>` and `--table=<name>` / `-t <name>` (e.g. `ngdb get --db=todo --table=task <id>`).

#### Ids in a hand-written query

Keys and `ref` fields hold raw bytes, so comparing one to an id string finds nothing - and SQL reports that as an empty result, not an error. Two functions cross the line on purpose:

- `id(text)` - the id text as raw bytes, for matching a key or a `ref`. A malformed id raises an error rather than matching nothing, so a typo says so.
- `idtext(blob)` - the inverse, for displaying or string-matching an id.

```sql
SELECT idtext(id), title FROM task WHERE parent_task = id('AZeR9kQ_c0-Bq2VvKQpXhw')
```

Neither is needed to join a `ref` to the key it points at - those are already the same bytes: `... FROM task c JOIN task p ON c.parent_task = p.id`.

Opt-in features (the table must enable them in its DDL `features:` block):

```
comment <db> <table> <id> <text>           add a comment to a row
comments <db> <table> <id>                 list a row's comments
attachuri <db> <table> <id> <uri> [desc]   attach a link-in-place URI
attachfile <db> <table> <id> <path> [desc] copy a file in and attach it
attachments <db> <table> <id>              list a row's attachments
```

Schema ops (rewrite the DDL file and the SQLite view; the old name becomes an alias so existing log entries still replay):

```
ngdb --rename-table <db> <old> <new>
ngdb --rename-field <db> <table> <old> <new>
```

The low-level `build`/`replay`/`sync`/`gc` and rename verbs also accept an explicit-path form (`build <ddl> <sqlite>`, `replay <ddl> <sqlite> <dir>`, `sync <logdir>`, `gc <ddl> <logdir>`) for use before a database is registered; a bare `sync <logdir>` still just reconciles a log directory with git.

`setnull` is its own verb so a literal `NULL` string stays expressible via `update`. An empty value (`f=`) is the empty string.

Setup flags:

```
ngdb --init [path]              register the $PWD .shcl as a database (name = its base name)
ngdb --config <dir> ...         use <dir> as the registry root, then run the rest
ngdb --encrypt[=on|off|auto] .. set the local encryption preference, then run
ngdb webuser <username>        set a proxied-mode web login (password from
                               NGDB_WEB_PASSWORD or a prompt); see Run modes
```

`--init` is the command-line equivalent of the picker's "Create new database": it registers the lone `*.shcl` in the current directory and materializes its view (build + seed), so it opens cleanly next time.

- `--init` with no arg: inside a git repo it places the tx-log at `<repo-root>/ngdb/<name>`; otherwise it uses the current directory.
- `--init <repo-root>`: same auto-placement under the given repo root.
- `--init <dir>`: uses `<dir>` verbatim as the tx-log location.

`--config <dir>` (or `--config=<dir>`) is a prefix on any invocation: it points the database registry at `<dir>` instead of the OS default, then the remaining arguments run normally (`--config /alt --init`, `--config /alt --tui`, etc.). The `=` form works for `--init` too.

`--encrypt[=on|off|auto]` is likewise a prefix (bare `--encrypt` means on). It sets this host's persistent encryption preference: `--init --encrypt=on` mints the per-db key and records the preference; on a later open it updates the stored preference. Accepted spellings include on/off/auto, yes/no, true/false, y/n, enabled/disabled. See [Encryption](#encryption).

## Run modes

The first argument selects the front-end; the default is the CLI.

```
ngdb <verb> ...            CLI (above)
ngdb --tui  [<db>]                           terminal UI (no name: pick / create a db)
ngdb --serve [<db>]                          local web UI on 127.0.0.1:8765
ngdb --script <file.lua> <ddl> <sqlite> <logdir>   run a Lua script (enterprise build)
ngdb --donate                                ways to support the project
ngdb --version                               print the version and exit (also -v)
```

The web UI binds to `127.0.0.1` only - that binding is the access control for a single-user local UI. All four front-ends share one core CRUD API, so they behave identically.

Web login: the `web_mode` setting (in `settings.shcl`, default `local`) picks how the web UI authenticates. `local` identifies the single user with no password - the git account of the log dir's repo, else the OS user - and refuses to serve if a reverse-proxy header ever appears (so an accidentally exposed box can't run passwordless). `proxied` requires a username and password for every request: add logins with `ngdb webuser <username>` (hashed into `webusers.shcl` in the config dir, outside the synced tree), and the signed-in user's group permissions then apply to the web view. Stronger methods are an enterprise feature.

In the TUI, press `T` to pick a colour theme (three dark, three light; the default is dark). Themes use fixed colours for readability regardless of your terminal palette, and the choice is remembered for next time.

## Startup discovery and the database registry

When `--tui` or a bare `ngdb` is run without a database name, it finds one instead of erroring:

- A lone `*.shcl` in the current directory is opened directly (its view defaults beside it, the tx-log dir is that directory).
- Otherwise the TUI shows a picker of registered databases (unopenable ones flagged with the reason), plus "Create new database" and "Open existing ...".

`--serve` has no picker, so with no name it uses a `$PWD` DDL if present, else asks for one.

Registered databases are per-database records at `<user-config>/ngdb/<name>/config.shcl`, where `<user-config>` is `$XDG_CONFIG_HOME` (else `~/.config`) on Linux, `%AppData%` on Windows, `~/Library/Application Support` on macOS. Read-only system locations (`$XDG_CONFIG_DIRS`, `%ProgramData%`) are also searched. Records are SHCL, the same language as the schema. One holds:

```
name: todo
ddl_path: /path/to/schema.shcl    ## git-synced
log_dir: /path/to/logdir          ## git-synced tx-log + attachments
sqlite_path: ...                  ## local, unsynced; rebuilds from the log
key_file: ...                     ## local encryption key (see Encryption)
encryption: auto                  ## this host's preference: on | off | auto
last_opened: ...                  ## RFC3339, refreshed on open
```

`sqlite_path` and `key_file` default beside the record and are omittable. A missing SQLite view is never an error - it rebuilds from the log on open.

## Environment variables

- `NANOGITDB_USER` - stamps the `user_id` of writes; defaults to the OS username.
- `NANOGITDB_HOST` - stamps the `host_name` of writes; defaults to the OS hostname. (One user writing from several machines stays distinguishable in the log.)
