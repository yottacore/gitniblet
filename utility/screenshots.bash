#!/usr/bin/env bash

##	Purpose:
##		Regenerate the README screenshots. Builds a throwaway demo database with
##		anonymous content, drives the terminal UI on a private Xvfb display, and
##		captures five 1920x1080 shots (originals in assets/screenshots/large),
##		then downsamples each to 640x360 thumbnails in assets/screenshots.
##		Run this after any significant visual change to the TUI.
##	Syntax:
##		screenshots.bash            ## build the binary if missing, then capture
##		screenshots.bash --no-build ## use the existing bin/ngdb
##	Notes:
##		Needs Xvfb + xterm + xdotool + ImageMagick (import/magick) - the same set
##		gui-headless.bash uses. All content is fake (demo user, /tmp paths) so no
##		real name, path, or data ends up in a committed image.

##	Copyright (c) 2026 Bubbles
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

set -Eeuo pipefail

dirPath_Self="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
dirPath_Repo="$(cd "$dirPath_Self/.." && pwd)"
headless="$dirPath_Repo/cicd/utility/gui-headless.bash"
exe="$dirPath_Repo/bin/ngdb"
dirLarge="$dirPath_Repo/assets/screenshots/large"
dirSmall="$dirPath_Repo/assets/screenshots"

display=":96"
geometry="128x36"   ## fills 1920x1080 exactly at DejaVu Sans Mono 18
font="DejaVu Sans Mono"; fontSize=18
themeBg="#1c1c1c"     ## the default dark theme background, so any margin blends
demo=""               ## temp workspace, set in fSetup (removed on exit)

fEcho() { printf '\n[ %s ]\n' "$*"; }

fKillTerms() {
	local w
	for w in $(DISPLAY="$display" xdotool search --class xterm 2>/dev/null || true); do
		DISPLAY="$display" xdotool windowkill "$w" 2>/dev/null || true
	done
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
#	Demo database - anonymous content only.
#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••

fSetup() {
	# A fixed, tidy path (not mktemp) so the picker screenshot shows clean,
	# anonymous locations. Wiped first for a clean slate.
	demo="/tmp/ngdb-demo"
	rm -rf "$demo"
	mkdir -p "$demo"/{cfg,empty,databases/team-issues,databases/reading-list,databases/recipes}
	export NANOGITDB_USER=demo NANOGITDB_HOST=workstation

	cat > "$demo/databases/team-issues/issues.shcl" <<'DDL'
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
	default_view: "tasks"
DDL

	printf 'database:\n\ttables:\n\t\ttable: book\n\t\t\tfields:\n\t\t\t\tfield: title\n\t\t\t\t\ttype: string\n\t\t\t\tfield: author\n\t\t\t\t\ttype: string\n' \
		> "$demo/databases/reading-list/books.shcl"
	printf 'database:\n\ttables:\n\t\ttable: recipe\n\t\t\tfields:\n\t\t\t\tfield: name\n\t\t\t\t\ttype: string\n\t\t\t\tfield: cuisine\n\t\t\t\t\ttype: string\n' \
		> "$demo/databases/recipes/recipes.shcl"

	local d
	for d in team-issues reading-list recipes; do
		( cd "$demo/databases/$d" && "$exe" --config "$demo/cfg" --init . >/dev/null 2>&1 )
	done

	local ddl="$demo/databases/team-issues/issues.shcl"
	local sql="$demo/cfg/issues/issues.sqlite"
	local log="$demo/databases/team-issues"
	local rel a b
	rel=$("$exe" create "$ddl" "$sql" "$log" task title="Ship v1.0 release" status=open priority=high assignee=alex opened="2026-06-30 09:00" | tail -1)
	a=$("$exe" create "$ddl" "$sql" "$log" task title="Write user documentation" status=open priority=medium assignee=priya opened="2026-07-01 10:15" parent_task="$rel" | tail -1)
	b=$("$exe" create "$ddl" "$sql" "$log" task title="Fix login redirect bug" status=open priority=high assignee=sam opened="2026-07-02 08:30" parent_task="$rel" | tail -1)
	"$exe" create "$ddl" "$sql" "$log" task title="Design landing page" status=closed priority=low assignee=jordan opened="2026-06-28 14:00" parent_task="$rel" >/dev/null
	"$exe" create "$ddl" "$sql" "$log" task title="Draft API reference" status=open priority=medium assignee=priya opened="2026-07-03 11:00" parent_task="$a" >/dev/null
	"$exe" create "$ddl" "$sql" "$log" task title="Migrate CI to new runners" status=open priority=medium assignee=sam opened="2026-07-01 16:20" >/dev/null
	"$exe" create "$ddl" "$sql" "$log" task title="Quarterly backup audit" status=closed priority=low assignee=alex opened="2026-06-20 09:45" >/dev/null
	"$exe" comment "$ddl" "$sql" "$log" task "$b" "Reproduced on staging, looking into the session cookie" >/dev/null
	"$exe" comment "$ddl" "$sql" "$log" task "$b" "Root cause: cookie path mismatch after the subdomain move" >/dev/null
	local p
	for p in "alex maintainer true" "priya contributor true" "sam contributor true" "jordan reviewer false"; do
		set -- $p
		"$exe" create "$ddl" "$sql" "$log" person screen_name="$1" role="$2" active="$3" >/dev/null
	done
}

fCleanup() {
	fKillTerms
	RPD_HEADLESS_DISPLAY="$display" "$headless" stop >/dev/null 2>&1 || true
	[[ -n "$demo" && -d "$demo" ]] && rm -rf "$demo"
}
trap fCleanup EXIT

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
#	Capture - one xterm per shot, driven by a keystroke sequence.
#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••

# fShot <out> <cwd> <ngdb-args...> -- <xdotool keys...>
fShot() {
	local out="$1" cwd="$2"; shift 2
	local -a nargs=() keys=(); local seen=0 a
	for a in "$@"; do
		if [[ "$a" == "--" ]]; then seen=1; continue; fi
		if ((seen)); then keys+=("$a"); else nargs+=("$a"); fi
	done
	fKillTerms
	sleep 0.4
	( cd "$cwd" && DISPLAY="$display" xterm -fa "$font" -fs "$fontSize" -geometry "${geometry}+0+0" \
		-b 0 -bw 0 +sb -bg "$themeBg" -fg '#e4e4e4' \
		-e env NANOGITDB_USER=demo NANOGITDB_HOST=workstation "$exe" "${nargs[@]}" ) &
	sleep 2.5
	local wid; wid=$(DISPLAY="$display" xdotool search --class xterm 2>/dev/null | head -1 || true)
	DISPLAY="$display" xdotool windowfocus --sync "$wid" 2>/dev/null || true
	local k
	for k in "${keys[@]}"; do
		DISPLAY="$display" xdotool key --window "$wid" --clearmodifiers "$k"
		sleep 0.7
	done
	sleep 0.6
	import -display "$display" -window root "$dirLarge/$out"
	echo "  $out"
}

#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
#	Main.
#•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••

fMain() {
	if [[ "${1:-}" != "--no-build" || ! -x "$exe" ]]; then
		fEcho "Build"
		"$dirPath_Repo/cicd/build.bash"
	fi
	[[ -x "$exe" ]] || { echo "no binary at $exe" >&2; exit 1; }

	fEcho "Demo database"
	fSetup

	fEcho "Display"
	RPD_HEADLESS_DISPLAY="$display" RPD_HEADLESS_SIZE=1920x1080x24 "$headless" start >/dev/null
	DISPLAY="$display" xsetroot -solid "$themeBg"

	fEcho "Capture (1920x1080)"
	mkdir -p "$dirLarge" "$dirSmall"
	local ddl="$demo/databases/team-issues/issues.shcl"
	local sql="$demo/cfg/issues/issues.sqlite"
	local log="$demo/databases/team-issues"
	fShot 1-picker.png "$demo/empty" --config "$demo/cfg" --tui -- Down
	fShot 2-view.png   "$demo/empty" --tui "$ddl" "$sql" "$log" -- a
	fShot 3-table.png  "$demo/empty" --tui "$ddl" "$sql" "$log" -- Escape Down Return
	fShot 4-form.png   "$demo/empty" --tui "$ddl" "$sql" "$log" -- Escape Down Return Return
	fShot 5-theme.png  "$demo/empty" --tui "$ddl" "$sql" "$log" -- a shift+t

	fEcho "Downsample (640x360)"
	local f b
	for f in "$dirLarge"/*.png; do
		b="$(basename "$f")"
		magick "$f" -filter Lanczos -resize 640x360 -strip "$dirSmall/$b"
		echo "  $b"
	done

	fEcho "Done"
}

fMain "$@"


##	Script history:
##		- 20260704: Created.
##		- 20260704: Moved to github/utility/; headless helper still in cicd/utility/. Run from cicd under a non-quick build.
