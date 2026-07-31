<!-- markdownlint-disable MD007 -- Unordered list indentation -->
<!-- markdownlint-disable MD010 -- No hard tabs -->
<!-- markdownlint-disable MD033 -- No inline html -->
<!-- markdownlint-disable MD055 -- Table pipe style [Expected: leading_and_trailing; Actual: leading_only; Missing trailing pipe] -->
<!-- markdownlint-disable MD041 -- First line in a file should be a top-level heading -->
# Nano Git DB - demo walkthrough

This is a "script" for demonstrating the tool through the CLI, the terminal UI, and the local web UI, using the small issue-tracker schema in this folder (`issues.shcl` + `issues.queries.shcl`). All three front-ends share one core, so the same database opens identically in any of them.

The story to land: *the append-only tx-log is the database*. SQLite is a disposable local view rebuilt from the log, and the log is plain text that lives in git - so the whole database syncs, merges, and diffs like source code.

## Setup

The built binary is at `../bin/ngdb` (run `cicd/build.bash` if it's missing). Everything below uses fake content, so nothing personal ends up on screen.

Pick one starting point:

- **Do the CLI part first** (recommended) - it creates the data by hand, then you open that same database in the terminal and web UIs. One continuous story.

- **Jump straight to a UI** - run `./seed.bash` first to populate a database, then start at [Part B](#part-b---tui) or [Part C](#part-c---web-ui).

The schema at a glance: one `issue` table (with subtasks via a self-referencing `parent_issue`, plus comments, an audit trail, and URI attachments turned on), a `person` table, and a `component` table. Two views: a **board** (hierarchical issue tree) and a **people** grid.

## Part A - CLI

Work in a scratch directory so the demo is self-contained and easy to reset:

```bash
mkdir -p /tmp/ngdb-demo && cd /tmp/ngdb-demo
cp <this-repo>/github_floss/demos/t7em6_issue-tracker/issues.shcl issues.queries.shcl .
export NANOGITDB_USER=demo NANOGITDB_HOST=workstation

exe=<this-repo>/github_floss/bin/ngdb
```

Register this folder's schema as a database named **issues** (one time). The name is all you type from here on - ngdb remembers where the schema, tx-log, and view live:

```bash
$exe --init .
```

```bash
registered "issues"
  config:  <config>/ngdb/issues/config.shcl
  ddl:     /tmp/ngdb-demo/issues.shcl
  tx-log:  /tmp/ngdb-demo
  sqlite:  <config>/ngdb/issues/issues.sqlite
  note: the tx-log dir is not in a git repo; syncing stays off until it is
```

> "Notice where things landed: the schema and the append-only log stay here in the folder - that's the database, and it's what syncs in git. The SQLite view is a disposable cache ngdb keeps for itself. From now on I just say `issues` and never touch those paths."

### 1. Look at the schema

> "The schema is just a readable text file - indented key/values, no SQL DDL."

```bash
$exe ddl ./issues.shcl
```

```bash
tables=3 relationships=0 views=2
  table "issue": fields=9 uniques=1 indexes=2
  table "person": fields=4 uniques=1 indexes=0
  table "component": fields=2 uniques=1 indexes=0
```

### 2. Add some data

> "Create a couple of people and components. Each write prints the new row's id."

```bash
$exe create issues person screen_name=alex  full_name="Alex Kim"  role=maintainer  active=true
$exe create issues person screen_name=priya full_name="Priya Rao" role=contributor active=true
$exe create issues component name=web lead=alex
$exe create issues component name=api lead=priya
```

Now an epic with two subtasks under it. Capture the epic's id and use it as the subtasks' `parent_issue`:

```bash
EPIC=$($exe create issues issue ref=NGD-1 title="Ship v1.0 release" status=in_progress priority=high component=web assignee=alex opened="2026-07-10 09:00" | tail -1)

BUG=$($exe create issues issue ref=NGD-2 title="Fix login redirect bug" status=open priority=high component=web assignee=priya opened="2026-07-11 08:30" parent_issue="$EPIC" | tail -1)

$exe create issues issue ref=NGD-3 title="Write user documentation" status=open priority=medium component=api assignee=priya opened="2026-07-12 10:15" parent_issue="$EPIC"
```

And a couple of standalone issues:

```bash
$exe create issues issue ref=NGD-4 title="Migrate CI to new runners" status=open   priority=medium component=api assignee=alex opened="2026-07-09 16:20"
$exe create issues issue ref=NGD-5 title="Quarterly backup audit"    status=closed priority=low    component=api assignee=alex opened="2026-06-20 09:45"
```

### 3. Read it back

> "Fetch one issue. Note the id renders as a short base64url string - that's the form you pass back in."

```bash
$exe get issues issue "$BUG"
```

Then a real query - open issues, highest priority first:

```bash
$exe query issues "SELECT ref, priority, status, assignee, title FROM issue WHERE status <> 'closed' AND is_deleted = 0 ORDER BY priority, ref"
```

> "The `query` verb is plain read-only SQL against the view - anything SQLite can do."

If you prefer to be explicit about which is the database and which is the table, every data verb also accepts flags in any order - `$exe get --db=issues --table=issue "$BUG"` is the same call.

### 4. Update, comment, attach

> "Move the bug into progress."

```bash
$exe update issues issue "$BUG" status=in_progress
```

The `issue` table opted into comments and URI attachments, so:

```bash
$exe comment issues issue "$BUG" "Reproduced on staging - session cookie path looks wrong"
$exe comment issues issue "$BUG" "Root cause: cookie path mismatch after the subdomain move"
$exe comments issues issue "$BUG"

$exe attachuri issues issue "$BUG" "https://example.com/logs/session-trace.txt" "staging trace"
$exe attachments issues issue "$BUG"
```

### 5. The audit trail wrote itself

> "That table also has an audit trail on. Every change was captured in the same commit as the change - no extra work."

Ids and references are stored as raw bytes, so a hand-written query wraps the text form in `id()`. The inverse is `idtext()`.

```bash
$exe query issues "SELECT action, user_id, \"values\" FROM audit_trail WHERE parent_id = id('$BUG') ORDER BY date"
```

```bash
action	user_id	values
create	demo	
update	demo	status:"open"
```

> "The update row records the prior value - status was `open` before we moved it."

(A `mark_delete` row joins these after the next step.)

### 6. Soft-delete

> "Delete is a soft-delete - the row drops out of views but its history stays in the log."

```bash
$exe markdelete issues issue "$BUG"
$exe query issues "SELECT ref, title FROM issue WHERE is_deleted = 0 ORDER BY ref"
```

The bug (NGD-2) is gone from the list.

### 7. The payoff - the log *is* the database

> "Here's the actual source of truth: a plain CSV, one line per change, git-friendly by design. It's right here in the folder."

```bash
head -6 txlog.csv
```

Now rebuild the SQLite view entirely from that log - the view is a throwaway cache, so this is a full from-scratch rebuild:

```bash
$exe replay issues
$exe query issues "SELECT ref, status, title FROM issue WHERE is_deleted = 0 ORDER BY ref"
```

```bash
applied 155 log entries into <config>/ngdb/issues/issues.sqlite
  issue: 4 live row(s)
  person: 2 live row(s)
  component: 2 live row(s)
```

> "Everything came back. The database is the log; SQLite is just a cache. Commit that log to git and the whole database syncs, branches, and merges like code."

## Part B - TUI

Open the **same database** in the terminal UI. It's the same core as the CLI, just interactive - no browser, no server, works over SSH.

```bash
$exe --tui issues
```

It opens straight onto the **board** view (the default view), which auto-runs the **Open issues** query, so the issue tree is already on screen. The bottom line is a live key hint - read it aloud once, then drive:

```
a=load-all  p=queries  enter=edit  tab=next-block  esc=back  T=theme  q=quit
```

### On the board view

- Point out the hierarchy: NGD-2 and NGD-3 sit indented under the NGD-1 epic - the `tree_grid` following `parent_issue`, same as the web board.

- Press **p** to open the named-query picker; choose **High priority**, **Closed**, or **Everything** and watch the block reload. These are the queries from `issues.queries.shcl`.

- Press **Enter** on a row to open its edit form; change `status` or `priority`, save. That write is a log entry plus an audit row, exactly like the CLI `update`.

- Press **Esc** to back out to the main list.

### The main list and a table

The main list shows the views (as "name (view)") and below them the raw tables. Its key hint is:

```
enter=open table | in rows: enter=edit n=new d=del x=hard-del c=comments/attachments u/f=attach uri/file r=reload esc=back | T=theme q=quit
```

- Open the **issue** table (Enter). You get a flat rows grid.

- **n** starts a new row; **Enter** edits the highlighted one; **c** opens the comments/attachments panel (add a comment inline); **u** / **f** attach a URI / a file; **d** soft-deletes (**x** hard-deletes); **r** reloads.

- Open **people (view)** or the **person** table the same way to show it's not issue-specific.

### One nice beat

- Press **T** for the theme picker - three dark, three light. The UI restyles live and the choice sticks for next time.

- **q** quits. Nothing was lost: quit and `head txlog.csv`, or open `--serve issues`, and every change you just made in the TUI is already there. Same log, different face.

## Part C - web UI

Serve the **same database** you just built (or a `./seed.bash` one). The web UI binds to loopback only - that binding is the access control for a local single-user UI.

```bash
$exe --serve issues
```

Open <http://127.0.0.1:8765> in a browser.

### Board view

- The page opens on the **board** view. It auto-runs the **Open issues** named query on load, so the issue tree is already showing.

- Point out the hierarchy: NGD-2 and NGD-3 sit indented under the NGD-1 epic - that's the `tree_grid` following `parent_issue`.

- Use the query dropdown (top of the block): switch between **Open issues**, **High priority**, **Closed**, **Everything**, and click **Run query**. Same named queries the CLI's SQL mirrored.

### Edit an issue

- Click **edit** on any issue row - a form opens on the right.

- Change `status` or `priority`, click **Save**. The change is a log entry and an audit row, exactly like the CLI `update`.

- On an issue you edit, scroll the form: there's a **comments** section (add one inline) and an **attachments** section (add a URI). These appear because the `issue` table enabled those features.

### Create and delete

- Click a table in the left sidebar (e.g. **issue**), then **New** to get a blank form; fill a couple of fields and **Save**.

- The row list has **del** (soft-delete, with a confirm) and **DEL** (hard-delete). Soft-delete matches the CLI `markdelete`.

### The people view

- In the sidebar click **people (view)**. It's a flat grid over the `person` table.

- Blocks open empty by design - click **All** to load the rows.

### Tie it together

- Leave the server running, drop back to a terminal, and run `$exe create issues ...` or `$exe update issues ...`. Reload the web page (or re-run a query) - the new data is there. Both front-ends are thin skins over the one core and the one log.

## Reset

The schema and log live under `/tmp/ngdb-demo` (and `./seed.bash`'s under `/tmp/ngdb-issues-demo`); the registry keeps a small record per database. To clean up:

```bash
rm -rf /tmp/ngdb-demo /tmp/ngdb-issues-demo
```

Then drop the `issues` record from the registry: run `$exe` with no arguments to open the picker, highlight it, and press **d** (it also offers to delete any leftover files).

## Cheat sheet

| Beat | CLI | TUI | Web
| :--                    | :--                            | :--                          | :--
| Schema is plain text   | `ngdb ddl ./issues.shcl`       | main list: views + tables    | left sidebar lists tables + views
| Create                 | `create issues <table> f=v`    | `n` -> fill -> save          | **New** -> fill -> **Save**
| Read                   | `get` / `query issues ...`     | a view, or `a`=load-all       | a view, or **All** on a block
| Named queries          | the same SQL by hand           | `p` -> pick                  | query dropdown -> **Run query**
| Update                 | `update issues <table> <id> f=v` | `enter` -> change -> save   | **edit** -> change -> **Save**
| Comments / attachments | `comment` / `attachuri issues ...` | `c` panel, `u`/`f` attach  | sections in the edit form
| Audit trail            | `query issues "... audit_trail ..."` | (captured the same way) | (captured the same way)
| Soft-delete            | `markdelete issues <table> <id>` | `d` (`x`=hard)             | **del** button
| Log is the truth       | `head txlog.csv`, `replay issues` | -                          | -

## Demo and doc history

- 20260713-14 JC: Created.
- 20260714 JC: Added TUI leg; folder moved under demos/.
- 20260715 JC: Switched to name-based commands (register once, then just the db name).
