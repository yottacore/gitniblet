#!/usr/bin/env python3

##	Purpose: Convert `go tool pprof -traces` output (read on stdin) into the folded
##		stack format inferno-flamegraph consumes: one line per stack, root-first,
##		"a;b;c <samples>". This is the Go bridge inferno used to ship as
##		inferno-collapse-go, which recent inferno releases dropped. Sample counts
##		are pprof's 10ms buckets normalized to whole samples so the flamegraph's
##		total_samples/fg:w come out as clean sample counts.
##	Syntax: go tool pprof -traces bin prof | pprof-to-folded.py
##	History: At bottom of script.

##	Copyright (c) 2026 Bubbles
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT


import re, sys

## A trace block starts with an indented "<n><unit>  <leaf frame>" line, then one
## indented frame per line for the callers (leaf first, root last), until a
## "----" separator. pprof units: ns, us/µs, ms, s.
UNIT_NS  = {"ns": 1.0, "us": 1e3, "µs": 1e3, "ms": 1e6, "s": 1e9}
SAMPLE_NS = 1e7                                          # 100Hz pprof sampling = 10ms/sample
LEAF_RE  = re.compile(r"^\s+([\d.]+)(ns|µs|us|ms|s)\s+(\S.*)$")
FRAME_RE = re.compile(r"^\s+(\S.*)$")


def main():
	folded = {}                                          # "root;...;leaf" -> summed samples
	frames, ns = [], 0.0

	def flush():
		if frames and ns > 0:
			samples = max(1, round(ns / SAMPLE_NS))
			key = ";".join(reversed(frames))             # -traces is leaf-first; fold root-first
			folded[key] = folded.get(key, 0) + samples

	for line in sys.stdin:
		if line.startswith("---"):                       # block separator
			flush(); frames, ns = [], 0.0
			continue
		m = LEAF_RE.match(line)
		if m:
			flush(); frames, ns = [], 0.0                # a new leaf begins a new block
			ns = float(m.group(1)) * UNIT_NS[m.group(2)]
			frames.append(m.group(3).strip())
			continue
		m = FRAME_RE.match(line)
		if m and frames:                                 # a caller frame (only inside a block)
			frames.append(m.group(1).strip())
	flush()

	if not folded:
		sys.stderr.write("pprof-to-folded: no stacks parsed from input\n")
		sys.exit(2)
	for key, samples in folded.items():
		print(f"{key} {samples}")


if __name__ == "__main__":
	main()


##	History:
##		- 20260709: Created.
