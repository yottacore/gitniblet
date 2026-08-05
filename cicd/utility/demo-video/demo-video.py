#!/usr/bin/env python3

##	Purpose:
##		Record the nano-git-db README demo: drive a real ngdb (TUI first, then a
##		few CLI verbs) inside a decorated xterm on a private Xvfb, type at a
##		realistic pace (variable wpm, occasional fixed typos, a beat before flags),
##		and encode two deliverables from one script:
##		  video: 1920x1080 mp4 (h264), kept in private/ (not committed)
##		  gif:   960x540 looping at 50fps, hard cut to a 2s black loop seam,
##		         -> assets/demo.gif
##		The terminal is a plain faux window (Monaspace Argon SemiBold, dark-gray
##		bg, pale-green text, an anonymous colorized user@host prompt); nothing real
##		- fake user `demo`, host `workstation`, /tmp paths - ever reaches a frame.
##		ngdb's TUI is tcell (CPU text), so no GPU/VirtualGL is needed: x11grab at
##		the delivery rate captures a clean, judder-free source.
##	Syntax:
##		demo-video.py [--profile video,gif] [--seed N] [--keep-work]
##		              [--no-rotate] [--display :97] [--no-build]
##		Env: NGDB_BIN overrides the binary (default REPO/bin/ngdb).
##	Notes:
##		Content start is anchored with a white root flash (found afterwards via
##		signalstats), so the lead-in trims exactly. Long CLI lines (e.g. --help)
##		are truncated, never wrapped, via a tiny ngdb wrapper on PATH.
##	History: at bottom.

##	Copyright © 2026 Bubbles (ID: XଌฅრX۳ᛟԃლፀƅꓩหδლც)
##	Licensed under The MIT License (MIT). Full text at:
##		https://mit-license.org/
##	SPDX-License-Identifier: MIT

import argparse
import os
import random
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

ME_DIR  = Path(__file__).resolve().parent
REPO    = ME_DIR.parents[2]                       # cicd/utility/demo-video -> github_floss
PRIVATE = REPO.parent / "private" / "demo-video"  # ../private symlink -> synced tree
DEMO_DB = ME_DIR / "demo-db.bash"

BORDER   = 4                                      # black outline around the window
WM_THEME = "Demo-square"                           # squared Greybird-dark copy (prep_home)
FRAME_L, FRAME_R, FRAME_T, FRAME_B = 1, 1, 26, 1  # xfwm4 decoration extents

# the faux terminal look
ROOT_HEX = "#000000"       # the 4px outline around the frame is pure black
BG_HEX   = "#23262b"       # dark gray (not black)
FG_HEX   = "#b8f2c4"       # bright pale green
CUR_HEX  = "#d7ffe4"       # a hair brighter for the cursor
PROMPT   = (150, 156, 162) # standard gray for the prompt punctuation
# dimmer complementary tones the random user/host are drawn from
NAME_TONES = [(224, 144, 158), (222, 178, 134), (150, 190, 214),
	(190, 170, 214), (150, 200, 176), (214, 190, 150)]
USERS = ["juno", "vela", "orion", "wren", "koa", "ada", "sol", "iris", "nova", "remy"]
HOSTS = ["nimbus", "vela", "atlas", "birch", "cobalt", "delta", "ember", "flint", "onyx", "quartz"]

LEAD_S      = 0.7          # quiet lead kept before the first keystroke
BLACK_S     = 2.0          # hard cut to a solid black hold at the loop seam

# gif fps must divide 100: a gif frame delay is whole centiseconds, so 50 lands on
# an exact 2cs and every frame holds the same time. 30 does not - it quantizes to a
# repeating 3,3,4cs pattern, which is a permanent 10ms judder on every third frame.
PROFILES = {
	"video": dict(size=(1920, 1080), fps=60, font_pt=22, ext="mp4"),
	"gif":   dict(size=(960, 540),   fps=50, font_pt=13, ext="gif"),
}
GIF_ASSET_MAX_MB = 14

def log(msg):
	print(f"[demo] {msg}", flush=True)

def run(cmd, **kw):
	return subprocess.run(cmd, check=True, **kw)

def out_of(cmd, env=None):
	return subprocess.run(cmd, check=True, capture_output=True, text=True, env=env).stdout


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Recorder: display / WM / xterm / capture lifecycle

class Rec:
	def __init__(self, args, profile):
		self.p       = profile
		self.size    = profile["size"]
		self.fps     = profile["fps"]
		self.display = args.display
		self.num     = self.display.lstrip(":")
		self.auth    = f"/tmp/rpd-gui-headless/Xauthority-{self.num}"
		self.bin     = os.environ.get("NGDB_BIN", str(REPO / "bin/ngdb"))
		self.work    = Path(tempfile.mkdtemp(prefix="ngdb-demo-"))
		self.home    = self.work / "home"
		self.cfg     = self.home / ".config"   # hermetic registry base (XDG_CONFIG_HOME)
		self.keep    = args.keep_work
		self.app     = None
		self.ff      = None
		self.win     = ""
		self.spawn_comp = 0.0   # measured before capture, see calibrate_spawn
		self.flash_e = 0.0
		self.t0_e    = 0.0
		self.seg_marks = {}

	def env(self):
		e = dict(os.environ)
		e.update(DISPLAY=self.display, XAUTHORITY=self.auth)
		return e

	def xdo(self, *a):
		subprocess.run(["xdotool", *a], env=self.env(), check=False,
			stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

	# --- display + window manager ---------------------------------------------
	def start_display(self):
		gh = str(REPO / "cicd/utility/gui-headless.bash")
		e = dict(os.environ, RPD_HEADLESS_DISPLAY=self.display,
			RPD_HEADLESS_SIZE=f"{self.size[0]}x{self.size[1]}x24")
		subprocess.run([gh, "stop"], env=e, capture_output=True)
		run([gh, "start"], env=e)
		# our own xfwm4 (no compositor) draws a plain dark frame - that decoration
		# IS the "fake window" chrome. HOME points at the fake home so it finds the
		# squared theme copy prep_home wrote to ~/.themes.
		wmEnv = self.env()
		wmEnv["HOME"] = str(self.home)
		self.wm = subprocess.Popen(["dbus-run-session", "--", "sh", "-c",
			f'xfconf-query -c xfwm4 -p /general/theme --create -t string -s "{WM_THEME}"; '
			'xfconf-query -c xfwm4 -p /general/title_font --create -t string -s "Sans Bold 9"; '
			'xfconf-query -c xfwm4 -p /general/button_layout --create -t string -s "|HMC"; '
			"exec xfwm4 --compositor=off --vblank=off"],
			env=wmEnv, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
		time.sleep(2.0)
		subprocess.run(["xsetroot", "-solid", ROOT_HEX], env=self.env(), check=False)

	def stop_display(self):
		if getattr(self, "wm", None):
			self.wm.terminate()
			try:
				self.wm.wait(timeout=5)
			except subprocess.TimeoutExpired:
				self.wm.kill()
			self.wm = None
		gh = str(REPO / "cicd/utility/gui-headless.bash")
		subprocess.run([gh, "stop"],
			env=dict(os.environ, RPD_HEADLESS_DISPLAY=self.display), capture_output=True)

	# --- terminal geometry (self-calibrating) ---------------------------------
	def _client_wh(self, win):
		r = subprocess.run(["xwininfo", "-id", win], env=self.env(),
			capture_output=True, text=True).stdout
		w = int(re.search(r"Width:\s*(\d+)", r).group(1))
		h = int(re.search(r"Height:\s*(\d+)", r).group(1))
		return w, h

	def fit_geometry(self, font_pt):
		# launch a probe xterm to read the real cell size at this font, then size
		# the terminal to the most whole cells that fit; launch_term stretches the
		# client to exact pixels after, so the frame fills all but the 4px outline
		probe = subprocess.Popen(["xterm", "-fa", "Monaspace Argon SemiBold",
			"-fs", str(font_pt), "-geometry", "80x24", "-e", "sleep", "30"],
			env=self.env(), stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
		win = self._wait_win()
		cw, ch = self._client_wh(win)
		probe.terminate()
		self._kill_terms()
		cell_w, cell_h = cw / 80.0, ch / 24.0
		inner_w = self.size[0] - 2 * BORDER - FRAME_L - FRAME_R
		inner_h = self.size[1] - 2 * BORDER - FRAME_T - FRAME_B
		cols = max(40, int(inner_w // cell_w))
		rows = max(16, int(inner_h // cell_h))
		return cols, rows

	def _wait_win(self, timeout=20):
		deadline = time.time() + timeout
		while time.time() < deadline:
			r = subprocess.run(["xdotool", "search", "--class", "xterm"],
				env=self.env(), capture_output=True, text=True)
			if r.stdout.strip():
				return r.stdout.split()[0]
			time.sleep(0.3)
		raise RuntimeError("xterm window never appeared")

	def _kill_terms(self):
		for w in subprocess.run(["xdotool", "search", "--class", "xterm"],
				env=self.env(), capture_output=True, text=True).stdout.split():
			self.xdo("windowkill", w)
		time.sleep(0.3)

	# --- typing cadence calibration -------------------------------------------
	def calibrate_spawn(self):
		# Every keystroke is its own xdotool process, and that spawn plus X connect
		# sits between keys on top of the sleep the typist asked for. Measured at
		# run time, since it spans an order of magnitude across machines, and
		# guessing high silently types far above the wpm band (a 40ms overshoot on
		# a 75ms key turns 160 wpm into 330). Runs with no window up and before the
		# capture starts, so the throwaway keystrokes are never on camera.
		samples = []
		for _ in range(25):
			started = time.time()
			subprocess.run(["xdotool", "type", "--delay", "0", "--", "x"],
				env=self.env(), check=False,
				stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
			samples.append(time.time() - started)
		samples.sort()
		self.spawn_comp = samples[len(samples) // 2]   # median shrugs off a stray stall
		log(f"xdotool spawn {self.spawn_comp * 1000:.1f}ms (typing cadence compensates)")

	# --- capture --------------------------------------------------------------
	def start_capture(self):
		self.raw = self.work / "raw.mkv"
		self.ff = subprocess.Popen([
			"ffmpeg", "-hide_banner", "-loglevel", "error",
			"-progress", str(self.work / "ffprogress.txt"),
			"-f", "x11grab", "-framerate", str(self.fps),
			"-video_size", f"{self.size[0]}x{self.size[1]}", "-i", self.display,
			"-c:v", "libx264", "-preset", "ultrafast", "-qp", "0",
			"-pix_fmt", "yuv444p", str(self.raw)],
			env=self.env(), stdin=subprocess.DEVNULL,
			stderr=open(self.work / "ffmpeg.log", "w"))
		prog = self.work / "ffprogress.txt"
		deadline = time.time() + 30
		while time.time() < deadline:
			if prog.exists() and re.search(r"(?m)^frame=([1-9]\d*)", prog.read_text()):
				break
			time.sleep(0.3)
		else:
			raise RuntimeError("x11grab produced no frames (see ffmpeg.log)")
		time.sleep(0.6)
		subprocess.run(["xsetroot", "-solid", "white"], env=self.env(), check=False)
		self.flash_e = time.time()
		time.sleep(0.25)
		subprocess.run(["xsetroot", "-solid", ROOT_HEX], env=self.env(), check=False)
		time.sleep(0.4)

	def stop_capture(self):
		if self.ff:
			self.ff.send_signal(signal.SIGINT)
			try:
				self.ff.wait(timeout=30)
			except subprocess.TimeoutExpired:
				self.ff.kill()
			self.ff = None

	# --- the terminal ---------------------------------------------------------
	def launch_term(self, cols, rows, font_pt):
		u = self.rng_user
		# a leading newline in PS1 keeps one blank line above every prompt, so any
		# command whose output has no trailing blank still gets breathing room. The
		# gray-flag tail grays whatever is typed next once ~/.ngdb-gray exists (the
		# outro comment), without a line-editor plugin.
		gp = "\\[\\e[38;2;{};{};{}m\\]".format(*PROMPT)
		uc = "\\[\\e[38;2;{};{};{}m\\]".format(*u["uc"])
		hc = "\\[\\e[38;2;{};{};{}m\\]".format(*u["hc"])
		gray_flag = ("\\[$(test -f \"$HOME/.ngdb-gray\" && printf '\\033[38;5;245m')\\]")
		ps1 = ("\\n" + uc + u["user"] + gp + "@" + hc + u["host"] + gp + ":\\w$ "
			+ "\\[\\e[0m\\]" + gray_flag)
		e = self.env()
		e.update(SHELL="/bin/bash", HOME=str(self.home),
			XDG_CONFIG_HOME=str(self.cfg),   # same registry the build-time populate used
			PATH=f"{self.home}/bin:{os.environ['PATH']}",
			PS1=ps1, HISTFILE="/dev/null",
			NANOGITDB_USER="demo", NANOGITDB_HOST="workstation")
		self.app = subprocess.Popen(["xterm",
			"-fa", "Monaspace Argon SemiBold", "-fs", str(font_pt),
			"-geometry", f"{cols}x{rows}+{BORDER}+{BORDER}",
			"-b", "0", "-bw", "0", "+sb", "+j", "-bc", "-bg", BG_HEX, "-fg", FG_HEX,
			"-cr", CUR_HEX, "-title", "ngdb",
			"-xrm", "XTerm.vt100.allowTitleOps: false",
			"-e", "/bin/bash", "--noprofile", "--norc", "-i"],
			env=e, cwd=str(self.tracker),
			stdout=open(self.work / "xterm.log", "w"), stderr=subprocess.STDOUT)
		self.win = self._wait_win()
		self.xdo("windowmove", self.win, str(BORDER), str(BORDER))
		# stretch the client to exact pixels so the decorated frame fills the screen
		# minus the outline; the sub-cell slack just paints as terminal background
		inner_w = self.size[0] - 2 * BORDER - FRAME_L - FRAME_R
		inner_h = self.size[1] - 2 * BORDER - FRAME_T - FRAME_B
		self.xdo("windowsize", self.win, str(inner_w), str(inner_h))
		time.sleep(0.5)
		self.xdo("windowactivate", self.win)
		self.mouse_park()
		time.sleep(0.4)

	def mouse_park(self):
		# tuck the pointer into the far corner so no I-beam sits over the content
		self.xdo("mousemove", str(self.size[0] - 4), str(self.size[1] - 4))

	def kill_app(self):
		if self.app:
			self.app.terminate()
			try:
				self.app.wait(timeout=5)
			except subprocess.TimeoutExpired:
				self.app.kill()
			self.app = None

	def cleanup(self):
		self.stop_capture()
		self.kill_app()
		self.stop_display()
		if not self.keep and self.work.exists():
			shutil.rmtree(self.work, ignore_errors=True)


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Typing engine: realistic pace, letter/number split, a beat before flags, typos

NEIGH = {
	"a": "sq", "b": "vn", "c": "xv", "d": "sf", "e": "wr", "f": "dg", "g": "fh",
	"h": "gj", "i": "uo", "j": "hk", "k": "jl", "l": "k", "m": "n", "n": "bm",
	"o": "ip", "p": "o", "q": "wa", "r": "et", "s": "ad", "t": "ry", "u": "yi",
	"v": "cb", "w": "qe", "x": "zc", "y": "tu", "z": "x",
}

# Typing pace, and the main knob for how long the recording runs (the script itself
# is fixed). Deliberately brisk - a demo reads better than real-time typing does.
# Keep the wander and jitter proportional to the pace, or the cursor stutters.
WPM_BAND   = (260.0, 420.0)   # letters drift within this
WPM_START  = (300.0, 360.0)   # ... starting somewhere in here
WPM_DRIFT  = 20.0             # per-key wander inside the band
WPM_DIGITS = 200.0            # digits are hunted a touch slower and steadier

class Typist:
	def __init__(self, rec, rng):
		self.rec = rec
		self.rng = rng
		self.wpm = rng.uniform(*WPM_START)

	def _pause(self, secs):
		time.sleep(max(0.0, secs - self.rec.spawn_comp))

	def _delay(self, ch):
		if ch.isdigit():
			return (12.0 / WPM_DIGITS) * self.rng.lognormvariate(0.0, 0.14)
		self.wpm += self.rng.uniform(-WPM_DRIFT, WPM_DRIFT)
		self.wpm = max(WPM_BAND[0], min(WPM_BAND[1], self.wpm))
		return (12.0 / self.wpm) * self.rng.lognormvariate(0.0, 0.22)

	def _emit(self, ch):
		if ch == " ":
			self.rec.xdo("key", "--clearmodifiers", "space")
		else:
			subprocess.run(["xdotool", "type", "--delay", "0", "--", ch],
				env=self.rec.env(), check=False,
				stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

	def _backspace(self, n):
		for _ in range(n):
			time.sleep(self.rng.uniform(0.09, 0.16))
			self.rec.xdo("key", "--clearmodifiers", "BackSpace")

	def type(self, text, typos=0.02):
		self.rec.xdo("windowactivate", self.rec.win)
		time.sleep(0.25)
		i = 0
		while i < len(text):
			ch = text[i]
			# a barely-noticeable beat of thought before a flag/option token
			if ch == "-" and (i == 0 or text[i - 1] == " "):
				time.sleep(self.rng.uniform(0.14, 0.34))
			self._pause(self._delay(ch) * (1.6 if ch == " " else 1.0))
			# an expert's slip: wrong neighbor, catch it, fix it (letters only)
			if ch.lower() in NEIGH and self.rng.random() < typos:
				wrong = self.rng.choice(NEIGH[ch.lower()])
				self._emit(wrong)
				extra = 0
				if self.rng.random() < 0.4 and i + 1 < len(text) and text[i + 1] != " ":
					self._pause(self._delay(text[i + 1]))
					self._emit(text[i + 1])
					extra = 1
				time.sleep(self.rng.uniform(0.22, 0.45))
				self._backspace(1 + extra)
				time.sleep(self.rng.uniform(0.08, 0.2))
				self._emit(ch)
				if extra:
					self._pause(self._delay(text[i + 1]))
					self._emit(text[i + 1])
				i += 1 + extra
				continue
			self._emit(ch)
			i += 1

	def enter(self):
		time.sleep(self.rng.uniform(0.15, 0.4))
		self.rec.xdo("key", "--clearmodifiers", "Return")

	def key(self, keysym):
		self.rec.xdo("key", "--clearmodifiers", keysym)

	def keys(self, keysym, n, hz=6.0):
		for _ in range(n):
			self.key(keysym)
			time.sleep(max(0.05, self.rng.uniform(0.85, 1.15) / hz))

	def cmd(self, text, settle=1.0, typos=0.02):
		self.type(text, typos)
		self.enter()
		time.sleep(settle)


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Fake home: the ngdb wrapper (truncate-not-wrap for --help) + a build helper

def prep_home(rec, rng):
	home = rec.home
	binx = home / "bin"
	binx.mkdir(parents=True, exist_ok=True)
	# the on-camera `ngdb` resolves here first. For -h/--help we truncate long
	# lines to the visible width instead of letting the terminal wrap them; every
	# other invocation (TUI, verbs, feature output) execs the real binary untouched.
	wrap = binx / "ngdb"
	wrap.write_text(
		"#!/bin/dash\n"
		f'real="{rec.bin}"\n'
		'for a in "$@"; do\n'
		'\tcase "$a" in -h|--help) exec "$real" "$@" | cut -c "1-${COLUMNS:-100}"; esac\n'
		'done\n'
		'exec "$real" "$@"\n')
	wrap.chmod(0o755)
	# a squared copy of Greybird-dark: the stock corner pixmaps round off through
	# transparency, so fill it with the border color and the frame goes square.
	# Lands in the fake home's ~/.themes, which the wm is pointed at.
	src = Path("/usr/share/themes/Greybird-dark/xfwm4")
	dst = home / ".themes" / WM_THEME / "xfwm4"
	shutil.copytree(src, dst, dirs_exist_ok=True)
	for corner in ("top-left", "top-right", "bottom-left", "bottom-right"):
		for state in ("active", "inactive"):
			xpm = dst / f"{corner}-{state}.xpm"
			if xpm.exists():
				xpm.write_text(xpm.read_text().replace("c None", "c #1D1F1F"))
	# pick this recording's anonymous identity + two distinct name tones
	uc, hc = rng.sample(NAME_TONES, 2)
	rec.rng_user = dict(user=rng.choice(USERS), host=rng.choice(HOSTS), uc=uc, hc=hc)


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Demo database (anonymous) via the shared builder

def build_db(rec):
	# the tracker lands in the fake home as ~/team-issues (the synced ddl + tx-log),
	# registered under the name "issues" so on-camera commands are just the name.
	# HOME + XDG_CONFIG_HOME match the on-camera xterm, so the registry it writes
	# is the one the recording reads.
	env = dict(os.environ, HOME=str(rec.home), XDG_CONFIG_HOME=str(rec.cfg))
	out_of(["bash", str(DEMO_DB), str(rec.home), rec.bin], env=env)
	rec.tracker = rec.home / "team-issues"


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Scenes - TUI first, then CLI, then a quiet outro (web leg comes later)
##	The database is registered as "issues", so every command just names it -
##	no ddl/sqlite/log paths on screen. The terminal opens in ~/team-issues so
##	the closing `ls` shows the folder that database actually is.

DB = "issues"
# columns typed in the order ngdb prints them (it alphabetizes), so what you type
# is what you see - no confusing reorder on screen
QUERY_OPEN = "select assignee, status, title from task where status = 'open'"

def seg_tui(r, t):
	# launch into the tree_grid board; a comments pane sits below it and follows
	# the selected task. Walk to one that already has a synced discussion, then
	# add a comment - a 1:m detail the board list never shows as a column.
	# two kinds of beat: waiting for a paint, and holding for something to be
	# read. Trim them separately.
	t.cmd(f"ngdb --tui {DB}", settle=1.5)
	t.key("a"); time.sleep(1.1)              # load the task list
	t.keys("Down", 3, hz=2.6); time.sleep(1.3)  # onto a task with a comment thread
	t.key("Tab"); time.sleep(0.5)            # focus the comments pane below
	t.key("Return"); time.sleep(0.7)         # open the new-comment prompt
	t.type("Deployed the fix to staging"); time.sleep(0.35)
	t.key("Tab"); time.sleep(0.3)            # onto Add
	t.key("Return"); time.sleep(1.6)         # add; the thread reloads with it
	t.key("q"); time.sleep(0.7)              # back out of the TUI

def seg_cli(r, t):
	# the same data from the shell; a write shows up on the next read
	t.cmd("# same data, from the shell",
		settle=0.5, typos=0.0)
	t.cmd(f'ngdb query --db={DB} "{QUERY_OPEN}"', settle=1.8)
	t.cmd(f'ngdb create --db={DB} --table=task title="Screen flashing on refresh" '
		'status=open priority=high assignee=demo', settle=1.3)
	# longer: this is where the new row has to be spotted in the re-read
	t.cmd(f'ngdb query --db={DB} "{QUERY_OPEN}"', settle=1.9)
	# the payoff: the whole database is this folder - schema, view, append-only log
	t.cmd("ls -1", settle=1.9)

def seg_outro(r, t):
	(r.home / ".ngdb-gray").touch()
	r.xdo("windowactivate", r.win)
	time.sleep(0.3)
	r.xdo("key", "--clearmodifiers", "Return")   # fresh prompt picks up the gray flag
	time.sleep(0.5)
	t.cmd("# nano-git-db.", settle=0.3, typos=0.0)
	time.sleep(1.8)                              # hold the closing line before the seam

SCRIPT = [
	("tui",   seg_tui),
	("cli",   seg_cli),
	("outro", seg_outro),
]


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Post: find the sync flash, build the filter chain, encode

def find_flash(raw, work):
	stats = work / "stats.txt"
	run(["ffmpeg", "-v", "error", "-t", "8", "-i", str(raw),
		"-vf", f"signalstats,metadata=print:key=lavfi.signalstats.YAVG:file={stats}",
		"-f", "null", "-"])
	best_t, best_y, pts = 0.0, -1.0, 0.0
	for line in stats.read_text().splitlines():
		mo = re.search(r"pts_time:([0-9.]+)", line)
		if mo:
			pts = float(mo.group(1))
		mo = re.search(r"YAVG=([0-9.]+)", line)
		if mo and float(mo.group(1)) > best_y:
			best_y, best_t = float(mo.group(1)), pts
	if best_y < 180:
		raise RuntimeError(f"sync flash not found (max YAVG {best_y})")
	return best_t

def vf_chain(rec):
	# hard-cut to a solid black hold at the end (no fade). On a looping gif that
	# black bridge marks the loop boundary and lands back on the opening frame; a
	# single static black tail also compresses far smaller than a graduated fade.
	return (f"fps={rec.fps},format=rgb24,"
		f"tpad=stop_mode=add:stop_duration={BLACK_S:.3f}:color=black")

def encode(rec, video_end_e):
	flash_vt = find_flash(rec.raw, rec.work)
	log(f"sync flash at t={flash_vt:.3f}s")
	trim = flash_vt + (rec.t0_e - rec.flash_e)
	content = video_end_e - rec.t0_e              # seconds of real capture after trim
	vf = vf_chain(rec)
	cut = ["-ss", f"{trim:.3f}", "-t", f"{content:.3f}"]   # read only real content; tpad extends
	out = rec.work / f"demo.{rec.p['ext']}"
	if rec.p["ext"] == "mp4":
		run(["ffmpeg", "-v", "error", "-y", *cut, "-i", str(rec.raw), "-vf", vf,
			"-c:v", "libx264", "-preset", "slow", "-crf", "20",
			"-pix_fmt", "yuv420p", "-movflags", "+faststart", str(out)])
	else:
		pal = rec.work / "pal.png"
		run(["ffmpeg", "-v", "error", "-y", *cut, "-i", str(rec.raw),
			"-vf", f"{vf},palettegen=stats_mode=full:max_colors=160", str(pal)])
		# diff_mode=rectangle emits only the changed rectangle per frame, which is a
		# few cells for most of a terminal recording - that is what pays for the
		# higher frame rate instead of the file growing with it.
		run(["ffmpeg", "-v", "error", "-y", *cut, "-i", str(rec.raw), "-i", str(pal),
			"-lavfi", f"{vf}[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=4"
			":diff_mode=rectangle",
			"-loop", "0", str(out)])
	return out


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Output placement + GFS rotation (mp4 + gif in their own private dirs)

def rotate(out_dir, prefix, ext, no_rotate):
	if no_rotate:
		return
	inc = REPO / "cicd/utility/include/gfs-rotate.bash"
	subprocess.run(["bash", "-c",
		f'source "{inc}" && gfs_rotate "{out_dir}" {prefix} {ext}'], check=False)

def place(rec, out, no_rotate):
	ext = rec.p["ext"]
	sub = "video" if ext == "mp4" else "gif"
	out_dir = PRIVATE / sub
	out_dir.mkdir(parents=True, exist_ok=True)
	stamp = time.strftime("%Y%m%d-%H%M%S")
	dst = out_dir / f"ngdb-demo_{stamp}.{ext}"
	shutil.copy2(out, dst)
	mb = dst.stat().st_size / (1 << 20)
	# copy the README asset from the fresh render first: rotation below may rename
	# dst into a GFS bucket, so copying from `out` keeps this independent of it.
	if ext == "gif":
		if mb <= GIF_ASSET_MAX_MB:
			asset = REPO / "assets" / "demo.gif"
			shutil.copy2(out, asset)
			log(f"README asset: {asset} ({mb:.1f} MiB)")
		else:
			log(f"WARNING: gif is {mb:.1f} MiB (> {GIF_ASSET_MAX_MB}); "
				"assets/demo.gif left untouched - trim the script or lower fps/colors")
	rotate(out_dir, "ngdb-demo", ext, no_rotate)
	log(f"{sub}: {dst} ({mb:.1f} MiB)")


##•••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••••
##	Entry

def record(args, name, seed):
	rng = random.Random(seed)
	rec = Rec(args, PROFILES[name])
	try:
		prep_home(rec, rng)
		build_db(rec)
		rec.start_display()
		cols, rows = rec.fit_geometry(rec.p["font_pt"])
		log(f"[{name}] terminal {cols}x{rows} @ {rec.p['font_pt']}pt")
		rec.calibrate_spawn()
		rec.start_capture()
		rec.launch_term(cols, rows, rec.p["font_pt"])
		time.sleep(1.0)
		rec.t0_e = time.time() - LEAD_S

		t = Typist(rec, rng)
		for seg, fn in SCRIPT:
			log(f"[{name}] segment: {seg}")
			rec.seg_marks[seg] = time.time()
			fn(rec, t)
		time.sleep(0.3)
		video_end_e = time.time()

		rec.stop_capture()
		rec.kill_app()
		out = encode(rec, video_end_e)
		place(rec, out, args.no_rotate)
		if rec.keep:
			log(f"[{name}] work dir kept: {rec.work}")
	finally:
		rec.cleanup()

def main():
	ap = argparse.ArgumentParser(description="Record the nano-git-db README demo (mp4 + gif).")
	ap.add_argument("--display", default=os.environ.get("NGDB_DEMO_DISPLAY", ":97"))
	ap.add_argument("--profile", default="video,gif", help="comma list: video,gif")
	ap.add_argument("--seed", type=int, default=None)
	ap.add_argument("--keep-work", action="store_true")
	ap.add_argument("--no-rotate", action="store_true")
	ap.add_argument("--no-build", action="store_true", help="use the existing bin/ngdb")
	args = ap.parse_args()

	binp = Path(os.environ.get("NGDB_BIN", str(REPO / "bin/ngdb")))
	if not args.no_build or not binp.exists():
		run([str(REPO / "cicd/build.bash")])
	if not binp.exists():
		sys.exit(f"no binary at {binp}")

	seed = args.seed if args.seed is not None else int(time.time()) & 0xFFFF
	log(f"seed {seed}")
	for name in [p.strip() for p in args.profile.split(",") if p.strip()]:
		if name not in PROFILES:
			sys.exit(f"unknown profile: {name}")
		record(args, name, seed)

if __name__ == "__main__":
	main()


##	Script history:
##		- 20260715: Created. Adapted from the silkterm demo recorder, minus
##		  GPU/audio: xterm faux window, TUI-then-CLI script, hard black loop
##		  seam, mp4 (private) + gif (private + assets/demo.gif).
##		- 20260717: Square full-bleed frame on a black 4px outline (squared
##		  theme copy + pixel-exact resize), smooth scroll + blinking cursor,
##		  TUI edit-and-save beat replaces the theme beat, CLI uses --db/--table
##		  with a lead-in comment, shorter outro line.
##		- 20260717: TUI beat now adds a comment in the board's comments pane
##		  (a linked 1:m detail) instead of editing status.
##		- 20260804: Gif runs at 50fps (an exact 2cs frame delay, where 30 fell
##		  into a 3,3,4cs judder), typing cadence compensates for a measured
##		  xdotool spawn instead of a hardcoded 42ms, and the gif emits only the
##		  changed rectangle per frame.
##		- 20260804: Brisk typing is set by the wpm band now rather than falling
##		  out of the spawn subtraction, so the pace keeps proportional jitter.
##		  Fixed beats trimmed, holding the ones that exist to be read.
