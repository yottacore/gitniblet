# Changelog

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Automatic cleanup of old deleted records from the log.
- Register databases once and pick from a list at startup.
- Selectable light and dark themes in the terminal UI.
- Web sign-in for proxied mode, so the web UI can be exposed safely.

### Notes

- A separately licensed enterprise edition adds at-rest encryption of the synced log and Lua scripting, in the same binary.
