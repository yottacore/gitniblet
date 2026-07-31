#!/usr/bin/env bash

##	Purpose:
##		Build a populated copy of the issue-tracker demo in a scratch dir, so a
##		live demo (especially the web UI) starts with data already in it. The
##		walkthrough itself creates this same data by hand; this is the shortcut
##		when you'd rather skip straight to showing it off. The database is
##		registered as "issues" in a registry kept inside the scratch dir (so it
##		never touches your real one), and every command just names it.
##	Syntax:
##		seed.bash [workdir]     ## default workdir: /tmp/ngdb-issues-demo
##	Notes:
##		Content is fake (demo user, /tmp paths). Prints the exact serve + CLI
##		commands to use against the seeded db when it finishes.

set -Eeuo pipefail

dirPath_Self="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exe="$dirPath_Self/../../bin/ngdb"
work="${1:-/tmp/ngdb-issues-demo}"

[[ -x "$exe" ]] || { echo "no binary at $exe - run cicd/build.bash first" >&2; exit 1; }

export NANOGITDB_USER=demo NANOGITDB_HOST=workstation
# a self-contained registry under the scratch dir - the demo never pollutes the
# real one, and `rm -rf "$work"` is a clean reset.
export XDG_CONFIG_HOME="$work/.config"

rm -rf "$work"; mkdir -p "$work"
cp "$dirPath_Self/issues.shcl" "$dirPath_Self/issues.queries.shcl" "$work/"

cd "$work"
"$exe" --init . >/dev/null   # registers the folder's issues.shcl as "issues"

"$exe" create issues person screen_name=alex  full_name="Alex Kim"  role=maintainer  active=true  >/dev/null
"$exe" create issues person screen_name=priya full_name="Priya Rao" role=contributor active=true  >/dev/null
"$exe" create issues component name=web lead=alex  >/dev/null
"$exe" create issues component name=api lead=priya >/dev/null

epic=$("$exe" create issues issue ref=NGD-1 title="Ship v1.0 release" status=in_progress priority=high   component=web assignee=alex  opened="2026-07-10 09:00" | tail -1)
bug=$( "$exe" create issues issue ref=NGD-2 title="Fix login redirect bug" status=open priority=high   component=web assignee=priya opened="2026-07-11 08:30" parent_issue="$epic" | tail -1)
"$exe" create issues issue ref=NGD-3 title="Write user documentation" status=open      priority=medium component=api assignee=priya opened="2026-07-12 10:15" parent_issue="$epic" >/dev/null
"$exe" create issues issue ref=NGD-4 title="Migrate CI to new runners"  status=open      priority=medium component=api assignee=alex  opened="2026-07-09 16:20" >/dev/null
"$exe" create issues issue ref=NGD-5 title="Quarterly backup audit"     status=closed    priority=low    component=api assignee=alex  opened="2026-06-20 09:45" >/dev/null

"$exe" comment issues issue "$bug" "Reproduced on staging - session cookie path looks wrong" >/dev/null
"$exe" comment issues issue "$bug" "Root cause: cookie path mismatch after the subdomain move" >/dev/null
"$exe" attachuri issues issue "$bug" "https://example.com/logs/session-trace.txt" "staging trace" >/dev/null

cat <<EOF

Seeded demo at: $work

Point ngdb at this demo's registry, then just name "issues":
  export XDG_CONFIG_HOME="$work/.config"

Web (opens on 127.0.0.1:8765):
  $exe --serve issues

CLI:
  $exe query issues "SELECT ref, status, priority, title FROM issue WHERE is_deleted = 0 ORDER BY ref"
EOF
