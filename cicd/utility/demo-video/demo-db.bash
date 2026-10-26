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
##		demo-db.bash <parent-dir> <gitniblet-bin>
##		  parent-dir : the tracker is built at <parent-dir>/team-issues (wiped)
##		  gitniblet-bin   : the binary to drive
##	History: at bottom.

##	Copyright (c) 2026 Bubbles
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Eeuo pipefail

# shellcheck source=../include/temp-run.bash
source "$(dirname "${BASH_SOURCE[0]}")/../include/temp-run.bash"   ## fSafeRm

parent="${1:?usage: demo-db.bash <parent-dir> <gitniblet-bin>}"
exe="${2:?usage: demo-db.bash <parent-dir> <gitniblet-bin>}"
[[ -x "$exe" ]] || { echo "demo-db: no binary at '$exe'" >&2; exit 1; }
exe="$(cd "$(dirname "$exe")" && pwd)/$(basename "$exe")"   # absolutize before we cd

export GITNIBLET_USER=demo GITNIBLET_HOST=workstation

dir="$parent/team-issues"
if [[ -e "$dir" ]]; then fSafeRm "$dir" > /dev/null; fi
mkdir -p "$dir"

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
gitniblet() { "$exe" "$@"; }

# register the folder's lone issues.shcl as the database "issues" (tx-log stays
# here in the folder; the sqlite view goes to the registry). Every command after
# this just names "issues" - no ddl/sqlite/log paths on screen.
gitniblet --init . >/dev/null

rel=$(gitniblet create issues task title="Ship v1.0 release" status=open priority=high assignee=alex opened="2026-06-30 09:00" | tail -1)
a=$(gitniblet create issues task title="Write user documentation" status=open priority=medium assignee=priya opened="2026-07-01 10:15" parent_task="$rel" | tail -1)
bug=$(gitniblet create issues task title="Fix login redirect bug" status=open priority=high assignee=sam opened="2026-07-02 08:30" parent_task="$rel" | tail -1)
gitniblet create issues task title="Design landing page" status=closed priority=low assignee=jordan opened="2026-06-28 14:00" parent_task="$rel" >/dev/null
gitniblet create issues task title="Draft API reference" status=open priority=medium assignee=priya opened="2026-07-03 11:00" parent_task="$a" >/dev/null
gitniblet create issues task title="Migrate CI to new runners" status=open priority=medium assignee=sam opened="2026-07-01 16:20" >/dev/null
gitniblet create issues task title="Quarterly backup audit" status=closed priority=low assignee=alex opened="2026-06-20 09:45" >/dev/null
gitniblet comment issues task "$bug" "Reproduced on staging, looking into the session cookie" >/dev/null
gitniblet comment issues task "$bug" "Root cause: cookie path mismatch after the subdomain move" >/dev/null

for p in "alex maintainer true" "priya contributor true" "sam contributor true" "jordan reviewer false"; do
	set -- $p
	gitniblet create issues person screen_name="$1" role="$2" active="$3" >/dev/null
done

echo "dir=$dir"


##	Script history:
##		- 20260715: Created; co-located short-path layout for the recorder.
##		- 20261025: The wipe goes through the shared guard, so it only clears a temp folder.
