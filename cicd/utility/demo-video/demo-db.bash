#!/usr/bin/env bash

##	Purpose:
##		Build a throwaway, fully anonymous demo database for the README recorder.
##		A small issue tracker: a self-referencing `task` (subtasks) with comments
##		+ audit and a tree_grid `tasks` view. It is registered as the database
##		"issues" so on-camera commands just name it; the synced ddl + append-only
##		tx-log live in the folder (the "your database is a folder" payoff `ls`
##		shows), while the rebuildable sqlite view goes to the registry. All content
##		is fake (user `demo`, host `workstation`) so nothing real lands in a frame.
##	Syntax:
##		demo-db.bash <parent-dir> <ngdb-bin>
##		  parent-dir : the tracker is built at <parent-dir>/team-issues (wiped)
##		  ngdb-bin   : the binary to drive
##	History: at bottom.

##	Copyright (c) 2026 Bubbles
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Eeuo pipefail

parent="${1:?usage: demo-db.bash <parent-dir> <ngdb-bin>}"
exe="${2:?usage: demo-db.bash <parent-dir> <ngdb-bin>}"
[[ -x "$exe" ]] || { echo "demo-db: no binary at '$exe'" >&2; exit 1; }
exe="$(cd "$(dirname "$exe")" && pwd)/$(basename "$exe")"   # absolutize before we cd

export NANOGITDB_USER=demo NANOGITDB_HOST=workstation

dir="$parent/team-issues"
rm -rf "$dir"; mkdir -p "$dir"

cat > "$dir/issues.shcl" <<'DDL'
database:
	tables:
		table: task
			fields:
				field: title
					type: string
				field: status
					type: string
				field: priority
					type: string
				field: assignee
					type: string
				field: opened
					type: datetime_local
				field: parent_task
					type: ref
			features:
				comments: yes
				audit_trail: yes
		table: person
			fields:
				field: screen_name
					type: string
				field: role
					type: string
				field: active
					type: bool
ui:
	views:
		view: "tasks"
			layout:
				block: 1
					table: task
					type: tree_grid
					parent_field: parent_task
				block: 2
					table: task
					type: comments
					location: 1, below, 35%
	default_view: "tasks"
DDL

cd "$dir"
ngdb() { "$exe" "$@"; }

# register the folder's lone issues.shcl as the database "issues" (tx-log stays
# here in the folder; the sqlite view goes to the registry). Every command after
# this just names "issues" - no ddl/sqlite/log paths on screen.
ngdb --init . >/dev/null

rel=$(ngdb create issues task title="Ship v1.0 release" status=open priority=high assignee=alex opened="2026-06-30 09:00" | tail -1)
a=$(ngdb create issues task title="Write user documentation" status=open priority=medium assignee=priya opened="2026-07-01 10:15" parent_task="$rel" | tail -1)
bug=$(ngdb create issues task title="Fix login redirect bug" status=open priority=high assignee=sam opened="2026-07-02 08:30" parent_task="$rel" | tail -1)
ngdb create issues task title="Design landing page" status=closed priority=low assignee=jordan opened="2026-06-28 14:00" parent_task="$rel" >/dev/null
ngdb create issues task title="Draft API reference" status=open priority=medium assignee=priya opened="2026-07-03 11:00" parent_task="$a" >/dev/null
ngdb create issues task title="Migrate CI to new runners" status=open priority=medium assignee=sam opened="2026-07-01 16:20" >/dev/null
ngdb create issues task title="Quarterly backup audit" status=closed priority=low assignee=alex opened="2026-06-20 09:45" >/dev/null
ngdb comment issues task "$bug" "Reproduced on staging, looking into the session cookie" >/dev/null
ngdb comment issues task "$bug" "Root cause: cookie path mismatch after the subdomain move" >/dev/null

for p in "alex maintainer true" "priya contributor true" "sam contributor true" "jordan reviewer false"; do
	set -- $p
	ngdb create issues person screen_name="$1" role="$2" active="$3" >/dev/null
done

echo "dir=$dir"


##	Script history:
##		- 20260715: Created; co-located short-path layout for the recorder.
