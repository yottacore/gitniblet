# Changelog

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v1.0.0-beta.2

### Added

- Install scripts for one-line setup: `install.bash` for Linux, macOS, FreeBSD and WSL, `install.ps1` for Windows and anywhere else with PowerShell 7. Both verify the download against the published checksum, say what they are about to do, and are safe to re-run.
- `--help` prints the usage block and succeeds.

### Fixed

- The local web UI accepted writes driven by another page open in the same browser, and answered to any hostname pointed at it. Writes now have to come from the UI itself, and it only answers under the address it was bound to.
- The web sign-in answered much faster for an unknown user than a real one, which revealed which accounts exist.
- The read-only notice could be dismissed when no notice was showing, leaving a running server read-only.
- An unknown command, or one missing arguments, printed usage on standard output and exited 0. It now exits non-zero with usage on standard error.
- `--table` without a database name took the first `field=value` as the database.
- `--tui` with two arguments opened the picker instead of reporting the mistake.
- The password typed at the `webuser` prompt was echoed to the terminal.
- A second process writing at the same time failed instead of waiting for the database file.
- For a database kept beside its schema, sync committed the derived SQLite file, and would have committed an encryption key placed there.
- The name of an attachment's working copy could point outside the temporary directory.
- Collecting old entries from the log could bring back a record whose table had been renamed, drop a write made while the collection ran, and leave a peer's sync permanently stuck.
- A write the view refused - a duplicate value in a `unique:` group - printed a new id and exited 0, leaving the field empty for good. It is now reported.
- Editing a hard-deleted record made a record holding only that field, which vanished again at the next rebuild.
- A control character in a value was kept in the view but could not be stored in the log, so a rebuild silently changed the value.
- A misspelled field name in `uniques:` or `indexes:` let a database list as fine and then fail to open with a raw SQL error.
- The web sign-in had no rate limit, so a burst of attempts could tie up the machine.
- A background sync could rebuild the view in the middle of another write.

### Changed

- Rebuilding a view from the log is about two and a half times faster, which shows up when opening a database and on every sync that brings changes.

## v1.0.0-beta.1

First public beta.

### Added

- `ref` field type, for a link to another record. It holds the raw key, so it joins straight against it.
- `id()` and `idtext()` SQL functions, for matching binary key and reference columns from a hand-written query.
- Schema validation. An unknown key, or a value outside the set a key accepts, is reported with its line number.

### Changed

- The schema and config files are written in [shcl](https://github.com/jim-collier/shcl). A schema is now `<name>.shcl` and its saved queries `<name>.queries.shcl`; a `.ddl` schema still loads.
- Every key and reference in the database is binary. Ids appear as text only in the transaction log.
- Record and transaction ids in the log are shorter. The longer form is still accepted wherever an id is typed.
- Repeating a table, field, or query name merges the two definitions instead of dropping the second. For the same reason, relationships now need a name.
- Unique groups and indexes are written one per line, as `unique: field1, field2` and `index: field1`.
- Tunables are written `key: value`. The older `key = value` form is gone.
- Renaming a table or field rewrites the schema file in canonical form. Comments and blank lines survive; hand-tuned column alignment does not.

### Fixed

- `@null` and `@previous` in a default reached the database as literal text instead of being treated as placeholders.

## v1.0.0-alpha.2

### Added

- Comment panes in views: a comment list that follows the selected row, in both the terminal and web UIs.

### Notes

- Pre-release tags (alpha, beta, rc) are now flagged as pre-releases automatically, and the release badge reflects it.

## v1.0.0-alpha.1

First public alpha. The pre-1.0 feature set is complete.

### Added

- One small static binary, no external dependencies. Runs on Linux, Windows, and macOS.
- Three interfaces over one shared core: a command-line interface, a terminal UI, and a self-hosted local web UI.
- Text-based DDL for the schema. No SQL needed to define or manage a database.
- Append-only transaction log as the source of truth, synced and auto-merged with git. The local SQLite database is a rebuildable view of it.
- Schema changes at any time. Old and new records stay compatible both ways, so there is no migration step.
- Rename tables and fields without losing or rewriting history.
- Views, including hierarchical tree grids over a self-referential parent link.
- Saved queries kept next to the schema and picked from a menu.
- Per-table extras: a comment list per record, an automatic audit trail, and file or link attachments.
- Users and groups, with permissions down to the table, field, and row.
- A `gc` pass that drops the log entries of long-deleted records.
- Register databases once and pick from a list at startup.
- Selectable light and dark themes in the terminal UI.
- Web sign-in for proxied mode, so the web UI can be exposed safely.

### Notes

- A separately licensed enterprise edition adds at-rest encryption of the synced log and Lua scripting, in the same binary.
