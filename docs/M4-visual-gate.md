# M4 — the manual visual gate

PRD §9.4. This is the checklist; `cc-statusline preview` is the instrument.

Everything else in this project is a test. This is not, and cannot be made into
one:

- The goldens measure display width with the same `go-runewidth` the renderer
  uses. They prove the arithmetic is **self-consistent**. They cannot prove a
  terminal agrees with it, because both sides of the comparison are the same
  library.
- Nothing in Go can tell whether `U+F06A9` rendered as a robot or as a
  replacement box. The bytes are identical either way.
- Nothing in Go can tell whether a green-to-red gradient is legible against a
  particular terminal background, or whether the 30% and 40% cells of a
  ten-cell bar are visibly different colours.

So a person looks. What follows makes that take about twenty minutes.

---

## 0. Build

```sh
go build -o bin/cc-statusline .
```

`preview` renders the four §5.1 reference states from `internal/refstate` — the
same payloads the goldens assert against and the same ones `TestReferenceStates`
quotes §5.1's criteria for. It reads the **embedded defaults** and never
`~/.config/cc-statusline/config.toml`, so the gate is reproducible on a machine
that is not yours.

---

## 1. How to read the output

```
--- danger-92 · 92% — warn marker and a 1M window · available=88
◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 45m │ 5h:85% 7d:62%
|-------------------------------- 70 --------------------------------|
```

The second line is a **width rule**: exactly 70 cells, drawn in ASCII. Every
character in it is East Asian Narrow, so it occupies 70 columns in every
terminal, in every locale, with every ambiguous-width setting. It is the one
line on screen this program cannot be wrong about.

| what you see | what it means |
|---|---|
| the status line ends **exactly** where its rule ends | this terminal and `go-runewidth` agree — the case everything is built on |
| the status line ends **past** its rule | some glyph is wider on screen than we think. This is the failure mode that wraps a line and costs a terminal row on every prompt |
| the status line ends **before** its rule | some glyph is narrower than we think. Harmless to layout, but it means the fitter is dropping segments it did not need to |

A screenshot of this is a record. A screenshot of a status line alone is an
opinion.

---

## 2. Scope — and a correction to §9.4

§9.4 names **Kitty, iTerm2, Terminal.app, and Windows Terminal**. That list does
not survive contact with what v1 ships or with the machine it is being built on.

- **Windows Terminal is out.** §13 defers Windows support entirely — git
  discovery, XDG paths, and the install prefix each need an answer nobody has
  written. Gating v1 on a platform v1 does not support is a spec error, not a
  missing screenshot.
- **iTerm2 and Terminal.app are macOS.** §10.1 ships `darwin` binaries, so they
  are in scope for the product; they are not reachable from this Linux machine.
  They are listed below as *deferred*, to be run on a Mac before v0.1 is tagged,
  not silently dropped.
- **Kitty is not installed here.** The terminals that are: **ghostty** (current)
  and **alacritty**.

The honest gate, then, is: everything that can be checked here is checked here
and recorded; the macOS half is recorded as outstanding with the exact commands
to run. §9.4 has been amended to match.

### The locale half is also not what it says

§9.4 asks for `LANG=en_US.UTF-8` and `LANG=ja_JP.UTF-8`. On this machine:

```
LANG=en_IN                 ← not a generated locale; only en_US.utf8 exists
locale -a → en_US.utf8, C, POSIX
```

Two things follow, and they pull in opposite directions:

- **Our code does not care.** §6.4 resolves the ambiguous width by string-
  matching the `LANG` / `LC_CTYPE` / `LC_ALL` prefix against `zh`, `ja`, `ko`.
  It never calls `setlocale`, so `LANG=ja_JP.UTF-8 cc-statusline preview` takes
  the CJK path whether or not the locale is generated. That is deliberate — it
  is what lets M7's wizard preview a locale it is not running in.
- **The terminal may not care either, and that is the actual question.** Most
  terminal emulators decide ambiguous width from their own configuration, not
  from the process's locale. So `LANG=ja_JP.UTF-8` is *our* switch, and the
  terminal has a separate one — which may not exist, may be off, and may not be
  named the same thing twice.

Step 4 measures which behaviour the terminal actually has, rather than assuming
either. Do not generate `ja_JP.UTF-8` on the strength of §9.4; it would change
nothing about what is being tested.

---

## 3. The run

For each terminal, at each width, take one screenshot of each command.

### 3.1 The four capability sets, side by side

```sh
./bin/cc-statusline preview --matrix --width 120
```

This is §9.4's "ASCII, Unicode, Nerd Font, Powerline" — four sets, four
reference states each, every line with its rule.

**Check:**

- [ ] **ASCII** — `* # - | > !` only. Nothing outside 7-bit ASCII anywhere.
- [ ] **Unicode** — `◆ ▓ ░ │ ⎇ ⚠` all render. Every line ends on its rule.
- [ ] **Nerd Font** — every glyph is a glyph, not `▯` and not a blank. **This is
      §12 Q1**, and it is the check most likely to fail: the model marker is
      `U+F06A9` (nf-md-robot), the branch is `U+E725` (nf-dev-git_branch), the
      warning is `U+F071` (nf-fa-warning). All three are Private Use Area, which
      Unicode classes as East Asian **Ambiguous** — so `go-runewidth` counts each
      as one cell, and a terminal that advances two for them is not disagreeing
      with the table so much as resolving the same ambiguity differently. If the
      Nerd Font line ends past its rule by exactly the number of icons on it,
      that is what happened, and either the glyphs change or the Nerd Font
      column needs a width rule of its own.
- [ ] **Powerline** — the `U+E0B0` arrow renders, and it is the *only* difference
      from the Nerd Font row. See §5 below on what it deliberately is not.

### 3.2 The narrow and wide extremes

```sh
./bin/cc-statusline preview --width 40
./bin/cc-statusline preview --width 200
```

- [ ] At 40, every line is clipped or dropped, and **nothing wraps**. A wrapped
      line is the failure stage 3 exists to prevent.
- [ ] At 200, all four states are whole and match §5.1 exactly.

### 3.3 Colour and the gradient

```sh
./bin/cc-statusline preview --width 120            # colour, as detected
./bin/cc-statusline preview --width 120 --plain    # no colour, for comparison
NO_COLOR=1 ./bin/cc-statusline preview --width 120 # must be identical to --plain
```

- [ ] The bar's fill is a visible green→amber→red ramp, not a flat band with
      noise on it. **This is §12 Q2 and C-2.**
- [ ] Compare `normal-42` (4 filled cells) against `danger-92` (9 filled). The
      gradient is *fill-relative*: both bars run the full ramp across however
      many cells they have. If the two read as "the same rainbow, different
      length" rather than "one is early, one is nearly full", C-2 resolves to
      **switch to a solid `ramp(pct/100)`** and §5.5 changes.
- [ ] Every colour is legible against this terminal's background. Note the
      background — a palette checked only against dark is checked once.

### 3.4 Ambiguous width — which behaviour does this terminal have?

Run both and see which one lines up:

```sh
./bin/cc-statusline preview --width 120 --ambiguous 1 --state danger-92
./bin/cc-statusline preview --width 120 --ambiguous 2 --state danger-92
```

`◆ ▓ ▒ │ …` are East Asian **Ambiguous**; `░ ⚠ ⎇` are Narrow (§5.6, corrected at
M3). Under `--ambiguous 2` the bar's empty cell becomes `▒` so that both bar
cells stay in one width class.

- [ ] **`--ambiguous 1` lines up** → this terminal renders ambiguous glyphs
      narrow. This is the common case and the default outside CJK.
- [ ] **`--ambiguous 2` lines up** → this terminal renders them wide. Then check
      whether it does so unconditionally or only under a CJK locale, by
      re-running with `LANG=ja_JP.UTF-8` and `LANG=en_US.UTF-8`.
- [ ] **Neither lines up** → the terminal is inconsistent across the glyph set.
      Record which glyph is off; `[general] ambiguous_width` is the escape
      hatch, but the glyph choice may be the better fix.

iTerm2 and Terminal.app both expose an explicit "treat ambiguous-width
characters as double-width" preference, and xterm has `-cjk_width`. Whether
ghostty and alacritty have one at all is exactly what the two commands above
answer without needing to know.

---

## 4. C-7 — measuring `width_reserve`

§5.6 reserves 12 columns on the right "because Claude Code renders system
notifications there". **Nobody measured 12.** It became load-bearing at M3: it
is the two cells that stop §5.1's danger state from fitting at 80 columns
(§5.1's blockquote). Ten would make it fit exactly.

It cannot be measured from inside the process — Claude Code captures stdout, so
there is no terminal to interrogate. So print a ruler and let Claude Code draw
it.

```sh
go build -o ~/.local/bin/cc-statusline .          # the probe ships in the binary
cp ~/.claude/settings.json ~/.claude/settings.json.pre-m4.bak
```

Then set `statusLine.command` in `~/.claude/settings.json` to exactly:

```
/home/xqsit/.local/bin/cc-statusline preview --probe
```

(replacing the `spike capture -- bash …` passthrough for the duration of the
measurement). The next prompt renders two lines exactly `COLUMNS` wide:

```
        10        20        30        40        50 ...
----+----1----+----2----+----3----+----4----+----5 ...
```

**Read off three numbers:**

1. **Does the ruler start at screen column 1?** If not, Claude Code applies left
   padding, and `[general] padding` should account for it.
2. **What is the highest column still visible?** Call it `V`. Anything covering
   the right-hand end is the chrome the reserve exists to avoid.
3. **How long is the ruler?** Call it `C`, read from its last label. If `C` is
   80 in a terminal that is not 80 columns wide, Claude Code is **not** exporting
   `COLUMNS` and the ruler is showing `defaultColumns` instead — which refutes
   §5.6's central assumption and is a larger finding than C-7. Widen the window
   and re-run to tell the two apart.

Then `width_reserve = C - V`, and if that is not 12, §5.6's default changes and
`TestReferenceStatesAtEighty` gets regenerated.

Do it twice: once with a notification on screen (start a long tool call), once
without. The reserve has to cover the worse case.

```sh
# restore
cp ~/.claude/settings.json.pre-m4.bak ~/.claude/settings.json
```

> The M0 spike is still installed as the current status line and should go back
> afterwards — C-4 and C-5 are still open and it is still collecting.

---

## 5. C-6 — does Powerline earn a background palette?

What ships today: `CC_STATUSLINE_POWERLINE=1` swaps the separator glyph for
`U+E0B0` and changes nothing else. A real Powerline prompt fills each segment with a
background colour and draws the arrow as the previous background against the
next, so the arrow reads as a seam between two solid blocks.

That needs a background per segment and a contrasting foreground on top, and
§7.2's `[colors]` table has neither. Choosing roughly sixteen colours without
looking at a terminal is the exact decision this gate exists to prevent.

- [ ] Look at the `nerdfont · powerline` row from step 3.1. Does the bare arrow
      read as intentional, or as a broken Powerline?
- [ ] If it reads as broken: C-6 resolves to **implement the fills**, §7.2 grows
      a `[colors.background]` table, and it is M5 work. `Style.ClipStyled`
      already appends its reset unconditionally, so stage 3 does not need
      revisiting.
- [ ] If it reads as intentional: C-6 closes as **shipped as designed**, and the
      README says "Powerline separators", not "Powerline".

---

## 6. Findings

### 6.0 The decidable half — run and passing

`scripts/gate-check.py` runs every part of this checklist a machine can answer,
so the human pass is spent only on what needs eyes. Run it with the binary
built:

```sh
make build && python3 scripts/gate-check.py
```

Passing as of `cbad960`, on ghostty (`TERM=xterm-ghostty`, `COLORTERM=truecolor`,
`LANG=en_IN`, `COLUMNS` unset):

| check | result |
|---|---|
| §3.1 ASCII set is pure 7-bit, SGR escapes included | pass — no byte above `0x7F` in any `--icons ascii` line |
| §3.3 `NO_COLOR=1` renders byte-identically to `--plain` | pass |
| §3.3 neither emits any SGR escape | pass |
| §3.2 no line claims more than the budget, at 40/80/120/200 | pass |
| §3.2 exit 0 and empty stderr at every width | pass |
| §3.4 `--ambiguous 1` and `2` differ, and `2` swaps the empty cell to `▒` | pass |

Two notes from the run:

- **`--plain` and `NO_COLOR=1` differ by one word** in the harness's capability
  report — `colour=truecolor` versus `colour=none` — and that is correct, not a
  bug. `--plain` is an override applied on a colour terminal; `NO_COLOR`
  resolves the capability itself. The rendered lines are identical, which is
  what §3.3 is actually asserting. `gate-check.py` compares under `--bare` for
  this reason.
- **`danger-92` claims 64 cells at width 80 and 70 at width 120.** It is losing
  a segment at 80, which is §5.1's blockquote behaving as documented and is the
  same two cells C-7 is about.

**What this does not do.** It never renders to a screen. Every check above is
about bytes, and the three questions this gate exists for — does `U+F06A9` draw
a robot or a box, is the ramp legible on *this* background, does the bare arrow
read as intentional — are untouched by it. A green run here is a green run on
the half that was never in doubt.

### 6.1 The human half — outstanding

| terminal | version | OS | 40 | 120 | 200 | ASCII | Unicode | NerdFont | Powerline | ambiguous | notes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| ghostty | | Linux | | | | | | | | | |
| alacritty | | Linux | | | | | | | | | |
| iTerm2 | | macOS | — | — | — | — | — | — | — | — | **deferred: needs a Mac** |
| Terminal.app | | macOS | — | — | — | — | — | — | — | — | **deferred: needs a Mac** |
| ~~Windows Terminal~~ | | — | — | — | — | — | — | — | — | — | out of scope, §13 |

| id | question | resolution | changes |
|---|---|---|---|
| §12 Q1 | Nerd Font glyph selection | | |
| §12 Q2 | gradient stops | | |
| C-2 | is the fill-relative gradient legible? | | |
| C-6 | Powerline without background fills | | |
| C-7 | `width_reserve` = ? | | |

**M4's exit criterion is not "it looked fine."** It is: every row above filled
in, C-2 / C-6 / C-7 resolved with a reason, and any glyph or stop that changed
reflected in `config/default.toml` and the goldens regenerated.
