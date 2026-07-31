#!/usr/bin/env python3

##	Purpose: Summarize the newest nano-git-db profiler flamegraph - self-time hot
##		spots, inclusive call buckets, subsystem attribution, and the caller chain
##		of the top leaves - by parsing the inferno SVG (its fg:w attribute = raw
##		sample counts). Runs two ways: plain (print the report, meant to run every
##		cicd run) and --check (print only when the newest flamegraph is newer than
##		the one last recorded in a local marker, then record it - meant for session
##		startup so a look is a no-op until there is something new to read).
##	History: At bottom of script.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


import argparse, html, os, re, sys

STEP      = 16              # flamegraph row height in the SVG, px (a child sits at parent_y - STEP)
SELF_TOP  = 22             	# self-time leaders to list
INCL_TOP  = 14             	# inclusive-time buckets to list
CHAIN     = 4              	# leaves whose caller chain we walk up to the root
SEEN_FILE = ".flame-seen"  	# marker basename, kept in the profiling dir - outside the git repo

NAME_RE   = re.compile(r"flame_(\d{8}-\d{6})_\w+\.svg$")
FRAME_RE  = re.compile(r"<title>(.*?)</title><rect ([^>]*?)/>", re.S)

## Subsystem attribution: self-time is bucketed by the function it burns in,
## keyed on a substring of the frame name. First match wins; order matters (the
## more specific package prefixes come before the catch-all runtime bucket).
BUCKETS = [
	("sqlite engine",     ("modernc.org/",)),   # sqlite/lib + its libc C-runtime shim
	("tx-log replay",     ("internal/core/txlog",)),
	("crud / seed",       ("internal/core/crud",)),
	("view build",        ("internal/core/store", "internal/core/schema", "internal/core/ddl")),
	("csv",               ("encoding/csv",)),
	("go runtime (gc/alloc/sched)", ("runtime.",)),
]


def fSkip(msg):
	##	2 = environmental skip (no dir / unparseable) - non-fatal, matches the
	##	cicd profiler stage which treats such things as a warning, not a failure.
	sys.stderr.write(f"flame-report: {msg}\n")
	sys.exit(2)


def fNewest(pdir):
	##	Sort on the timestamp, NOT the role suffix: GFS rotation retags the role
	##	(frequent -> latest -> hour/day/...) as time passes, but the timestamp in
	##	the name is stable.
	best = None
	for name in os.listdir(pdir):
		m = NAME_RE.match(name)
		if m and (best is None or m.group(1) > best[0]):
			best = (m.group(1), name)
	return best


def fParse(path):
	text = open(path, encoding="utf-8").read()
	m = re.search(r'total_samples="(\d+)"', text)
	total = int(m.group(1)) if m else 0
	frames = []                                      # each: (name, x, y, w) in raw samples
	for fm in FRAME_RE.finditer(text):
		attrs = fm.group(2)
		def val(key):
			vm = re.search(re.escape(key) + r'="([\d.]+)"', attrs)
			return float(vm.group(1)) if vm else None
		y, x, w = val("y"), val("fg:x"), val("fg:w")
		if None in (y, x, w):
			continue
		name = re.sub(r"\s*\(\d[\d,]* samples.*$", "", html.unescape(fm.group(1)))
		frames.append((name, x, y, w))
	if not total or not frames:
		fSkip(f"could not parse a flamegraph out of {path}")
	return total, frames


def fBucket(name):
	for label, needles in BUCKETS:
		if any(n in name for n in needles):
			return label
	return "other"


def fAnalyze(total, frames, top):
	byY = {}
	for fr in frames:
		byY.setdefault(fr[2], []).append(fr)
	eps = 1e-6

	def kids(fr):
		_, x, y, w = fr
		return [c for c in byY.get(y - STEP, []) if c[1] >= x - eps and c[1] + c[3] <= x + w + eps]

	def parent(fr):
		_, x, y, w = fr
		for p in byY.get(y + STEP, []):
			if p[1] <= x + eps and p[1] + p[3] >= x + w - eps:
				return p
		return None

	def selfW(fr):
		return fr[3] - sum(c[3] for c in kids(fr))

	selfBy, inclBy, byName, attrib = {}, {}, {}, {}
	for fr in frames:
		name = fr[0]
		byName.setdefault(name, []).append(fr)
		inclBy[name] = inclBy.get(name, 0.0) + fr[3]
		s = selfW(fr)
		selfBy[name] = selfBy.get(name, 0.0) + s
		if s > 0:
			attrib[fBucket(name)] = attrib.get(fBucket(name), 0.0) + s

	def pct(v):
		return f"{v / total * 100:5.1f}%"

	print("attribution (self-time by subsystem):")
	for label, _ in BUCKETS + [("other", None)]:
		if attrib.get(label, 0.0) > 0:
			print(f"  {pct(attrib[label])}  {label}")
	print()

	print("top self-time (where CPU actually burns):")
	for name, v in sorted(selfBy.items(), key=lambda kv: -kv[1])[:top]:
		if v < 1:
			break
		print(f"  {pct(v)} {int(v):4d}  {name}")
	print()

	print("top inclusive (call buckets):")
	for name, v in sorted(inclBy.items(), key=lambda kv: -kv[1])[:INCL_TOP]:
		print(f"  {pct(v)} {int(v):4d}  {name}")
	print()

	print(f"caller chains of the top {CHAIN} leaves:")
	for name, v in sorted(selfBy.items(), key=lambda kv: -kv[1])[:CHAIN]:
		fr = max(byName[name], key=selfW)
		print(f"  {name}  ({pct(v)} self)")
		cur, depth = parent(fr), 0
		while cur and depth < 12:
			print(f"      {cur[0]}")
			if cur[0] in ("all", "root"):
				break
			cur, depth = parent(cur), depth + 1


def main():
	here = os.path.dirname(os.path.abspath(__file__))
	default_dir = os.path.normpath(os.path.join(here, "..", "artifacts", "profiling"))

	ap = argparse.ArgumentParser(description="Summarize the newest nano-git-db profiler flamegraph.")
	ap.add_argument("--dir", default=default_dir, help="profiling directory (default: %(default)s)")
	ap.add_argument("--file", help="analyze this SVG instead of the newest in --dir")
	ap.add_argument("--top", type=int, default=SELF_TOP, help="self-time leaders to list")
	ap.add_argument("--check", action="store_true",
	                help="startup gate: print only if newer than the local marker, then record it")
	ap.add_argument("--force", action="store_true", help="with --check, report even if already seen")
	ap.add_argument("--no-mark", action="store_true", help="with --check, do not update the marker")
	a = ap.parse_args()

	if a.file:
		path = a.file
		if not os.path.isfile(path):
			fSkip(f"no such file: {path}")
		name = os.path.basename(path)
		m = NAME_RE.match(name)
		ts = m.group(1) if m else ""
	else:
		if not os.path.isdir(a.dir):
			fSkip(f"no profiling dir: {a.dir}")
		nb = fNewest(a.dir)
		if not nb:
			fSkip(f"no flamegraphs in {a.dir}")
		ts, name = nb
		path = os.path.join(a.dir, name)

	marker = os.path.join(a.dir, SEEN_FILE)
	if a.check and not a.force:
		seen = ""
		try:
			seen = open(marker).read().strip()
		except OSError:
			pass
		if ts and seen and ts <= seen:
			print(f"SEEN {name}  (nothing newer than {seen})")
			return

	total, frames = fParse(path)
	print(f"{'NEW' if a.check else 'FLAME'} {name}  ({ts or 'n/a'}, {total} samples)")
	print()
	fAnalyze(total, frames, a.top)

	if a.check and not a.no_mark and ts:
		try:
			open(marker, "w").write(ts + "\n")
		except OSError as e:
			sys.stderr.write(f"flame-report: could not write marker: {e}\n")


if __name__ == "__main__":
	main()


##	History:
##		- 20260709: Created (adapted from the SilkTerm flame-report for Go subsystems).
