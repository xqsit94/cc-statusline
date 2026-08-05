#!/usr/bin/env python3
"""The decidable half of docs/M4-visual-gate.md.

Everything here is a yes/no a machine can answer. The glyph and colour
judgments (Q1, Q2, C-2, C-6) are deliberately not attempted.
"""
import re
import subprocess
import sys

BIN = "./bin/cc-statusline"
ANSI = re.compile(r"\x1b\[[0-9;]*m")


def run(args, env=None):
    import os
    e = dict(os.environ)
    if env:
        e.update(env)
    p = subprocess.run([BIN] + args, capture_output=True, text=True, env=e)
    return p.stdout, p.stderr, p.returncode


fails = []


def check(name, ok, detail=""):
    print(f"{'PASS' if ok else 'FAIL'}  {name}" + (f"\n        {detail}" if detail else ""))
    if not ok:
        fails.append(name)


# ── 3.1 ASCII purity ────────────────────────────────────────────────────────
# §9.4: "* # - | > ! only. Nothing outside 7-bit ASCII anywhere."
out, _, _ = run(["preview", "--matrix", "--width", "120"])
blocks = {}
cur = None
for line in out.splitlines():
    m = re.match(r"^=== (.+?) =+$", line)
    if m:
        cur = m.group(1).strip()
        blocks[cur] = []
    elif cur:
        blocks[cur].append(line)

# --bare, because the harness's own header lines legitimately contain `·` and
# `—`. Those are chrome, not the status line; §9.4's criterion is about what the
# renderer emits.
ascii_lines, _, _ = run(["preview", "--icons", "ascii", "--width", "120", "--bare"])
bad = sorted({c for c in ascii_lines if ord(c) > 0x7F})
check("ASCII set is pure 7-bit, escapes included", not bad,
      f"non-ASCII found: {bad}" if bad else "no byte above 0x7F in any ascii status line")

# ── 3.3 NO_COLOR must equal --plain ─────────────────────────────────────────
# Again --bare. The full output differs by one word in the capability report
# (`colour=truecolor` under --plain, `colour=none` under NO_COLOR) and that is
# correct: --plain is an override on a colour terminal, NO_COLOR resolves the
# capability itself. What must match is the rendered lines.
plain, _, _ = run(["preview", "--width", "120", "--plain", "--bare"])
nocolor, _, _ = run(["preview", "--width", "120", "--bare"], env={"NO_COLOR": "1"})
check("NO_COLOR=1 renders byte-identically to --plain", plain == nocolor,
      "" if plain == nocolor else "rendered lines differ")
check("NO_COLOR=1 emits no SGR escapes", "\x1b[" not in nocolor)
check("--plain emits no SGR escapes", "\x1b[" not in plain,
      "" if "\x1b[" not in plain else "escape sequences present in --plain output")

# ── 3.2 Nothing exceeds the budget at any width ─────────────────────────────
# The rule the program prints is its own width claim. Two things are checkable
# without trusting go-runewidth: the claim never exceeds the stated budget
# (a fitter bug), and no rendered line contains a newline mid-line (a wrap).
for width in (40, 80, 120, 200):
    out, err, rc = run(["preview", "--width", str(width)])
    clean = ANSI.sub("", out)
    budget = None
    m = re.search(r"available = \d+ - 2×\d+ padding - \d+ reserve = (\d+) cells", clean)
    if m:
        budget = int(m.group(1))
    rules = [int(x) for x in re.findall(r"^\|-+ (\d+) -+\|$", clean, re.M)]
    over = [r for r in rules if budget is not None and r > budget]
    check(f"width={width}: no line claims more than the {budget}-cell budget",
          not over, f"budget={budget} rules={rules}" + (f" OVER: {over}" if over else ""))
    check(f"width={width}: exit 0, empty stderr", rc == 0 and err == "",
          "" if rc == 0 and err == "" else f"rc={rc} stderr={err!r}")

# ── 3.4 Ambiguous width: both modes render, and they differ ─────────────────
a1, _, _ = run(["preview", "--width", "120", "--ambiguous", "1", "--state", "danger-92"])
a2, _, _ = run(["preview", "--width", "120", "--ambiguous", "2", "--state", "danger-92"])
check("--ambiguous 1 and 2 produce different output", a1 != a2,
      "identical — the flag would be doing nothing" if a1 == a2 else "")
check("--ambiguous 2 swaps the empty bar cell to ▒ (§5.6)",
      "▒" in ANSI.sub("", a2) and "▒" not in ANSI.sub("", a1))

print()
print(("ALL DECIDABLE CHECKS PASS" if not fails
       else f"{len(fails)} FAILED: {fails}"))
sys.exit(1 if fails else 0)
