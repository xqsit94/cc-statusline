# cc-statusline — Product Requirements Document

| | |
|---|---|
| **Status** | Implemented through M8 |
| **Version** | 0.1.0 |
| **Owner** | xqsit94 |
| **Module** | `github.com/xqsit94/cc-statusline` |
| **Language** | Go 1.26+ |
| **Requires** | Claude Code ≥ 2.1.153 |

This is the reference for *what the program does and why*: the data contract, the
visual specification, the width model, and the capability matrix. Section numbers
are cited from the source, so they are stable — do not renumber. What is still
unresolved lives in [`TODOS.md`](../TODOS.md); what changed and when, in
[`CHANGELOG.md`](../CHANGELOG.md).

---

## 1. Overview

`cc-statusline` is a Claude Code status line: a small binary that reads a JSON
session payload on stdin and prints two formatted lines to stdout. Claude Code
renders those lines beneath the prompt on every assistant turn.

It exists to answer four questions at a glance, without breaking focus:

1. How much context is left before a compaction event?
2. What is this session costing?
3. How much of the 5-hour and 7-day rate limit is burned?
4. What branch am I on, and how much have I changed?

The design target is **peripheral-vision legibility**. You should be able to read
the state of the session from the corner of your eye without moving focus off the
code. That constraint drives every visual decision here, and it is why nothing on
the line updates more often than once a minute.

### 1.1 Non-goals

- Not a TUI dashboard. The render path is a one-shot pipe with no event loop.
- Not a transcript analyzer. No parsing of `.jsonl` transcripts.
- Not an API client. No calls to `/api/oauth/usage` or any network endpoint.
- Not a plugin framework. Segments are Go code; configuration selects and styles
  them but does not define new ones.
- **Not a git client.** v1 renders the branch name and nothing else from git.
  See §13.

### 1.2 Minimum version rationale

- `COLUMNS` / `LINES` in the environment (§5.6's only width source): **2.1.153+**
- `context_window.total_input_tokens` current-context semantics: **2.1.132+**

Below 2.1.153 the binary still renders; `COLUMNS` is unset and the 80-column
fallback applies.

---

## 2. Design thesis

The render path and the configuration path have opposite complexity budgets.
Render must be fast, non-interactive, and degrade to a dumb terminal.
Configuration wants to be rich and interactive. `cc-statusline` splits them into
subcommands, which is what makes a Bubble Tea dependency correct: its event loop
never runs during render.

The binding architectural constraint is **"render never blocks and never forks"**.
As of rev 4 the render path spawns no subprocesses at all.

---

## 3. Data contract

### 3.1 Payload source of truth

| Display element | Payload source | Absent when |
|---|---|---|
| Model name | `model.display_name` | never |
| Context percent | computed: `total_input_tokens / context_window_size × 100`, falling back to `context_window.used_percentage`. See §5.3. | both absent |
| Context window size | `context_window.context_window_size` | never (200000 default) |
| Session cost | `cost.total_cost_usd` | never |
| Session duration | `cost.total_duration_ms` | never |
| Lines added | `cost.total_lines_added` | never |
| Lines removed | `cost.total_lines_removed` | never |
| 5-hour limit | `rate_limits.five_hour.used_percentage` | non-subscriber, or pre-first-response |
| 7-day limit | `rate_limits.seven_day.used_percentage` | non-subscriber, or pre-first-response |
| Project name | basename of `workspace.project_dir` | never |
| Reasoning effort | `effort.level` | the current model has no effort parameter |

**Path resolution input** (read, never displayed): `workspace.current_dir` drives
git discovery. See §5.8.

**Available but not rendered.** Fields with no v1 segment; adding them is post-v1
work tracked in §13: `cwd`, `workspace.added_dirs`, `workspace.repo.*`,
`workspace.git_worktree`, `worktree.*`, `session_name`, `version`,
`output_style.name`, `thinking.enabled`, `fast_mode`, `vim.mode`,
`agent.name`, `pr.*`, `exceeds_200k_tokens`, `context_window.current_usage.*`,
`context_window.remaining_percentage`, `cost.total_api_duration_ms`, `prompt_id`,
`transcript_path`, `session_id`, `rate_limits.*.resets_at` (**unix epoch seconds
as a number**, not an ISO-8601 string — measured, §3.1.1).

> **`vim.mode` interaction.** If a `vim` segment is ever added, `init` must also
> set `"hideVimModeIndicator": true`, or the mode renders twice.

### 3.1.1 Contract verification — MEASURED (M0, 2026-08-05)

Everything below §3 was downstream of assumptions that had never been run once.
M0 ran them. Method: `cc-statusline capture` installed as the status line in
passthrough mode, 35 payloads spooled, analysed by `cc-statusline report`.
Both questions are answered; the measurement's limits are stated at the end.

**Q1 — do the fields exist with the shapes claimed?** Yes. Every key §3.1 names
was observed, with these exceptions and corrections:

| Finding | Detail |
|---|---|
| Never observed | `workspace.git_worktree`, `worktree.*`, `pr.*`, `vim.mode`, `agent.name` — all conditional on state this machine did not enter. Absent, not refuted. |
| Type correction | `rate_limits.*.resets_at` is a **number** (unix epoch seconds, e.g. `1785951600`), not an ISO-8601 string. Anything rendering "resets in 42m" must parse it as epoch. |
| Shape confirmed | `workspace.repo` = `{host, name, owner}`. `context_window.current_usage` = `{input_tokens, cache_read_input_tokens, cache_creation_input_tokens, output_tokens}`. |
| No drift | Zero undocumented keys. §3.1's inventory is complete as written. |
| Confirmed, not corrected | `context_window.total_input_tokens` equalled the summed input-side `current_usage` fields in 33/33 payloads — it is the current window's input tokens, exactly as §1.2 already claimed for 2.1.132+. The document was right; the spike's first code comment asserted the opposite and was wrong. Recorded because the field's *name* invites that error. |

**Q2 — what is `used_percentage` a percentage OF?** The **raw context window**.

```
used_percentage = round( context_window.total_input_tokens / context_window_size × 100 )
```

Reproduced exactly in 19/19 distinct observations (35 payloads). Output tokens
are excluded from the numerator: including them reproduces only 26/27, and the
single disagreement sits on a rounding boundary — which is precisely where a
wrong numerator reveals itself. Independently, the feasible-denominator interval
across all observations is 99.8%–100.9% of `context_window_size`, which brackets
100% and excludes any autocompact deduction (a threshold would move the
denominator by tens of percent, not by one).

**Consequences, in order of severity:**

1. **§5.4's thresholds are calibrated against a scale the number may never
   reach.** Compaction fires before `used_percentage` hits 100, so `danger = 85`
   may sit *after* the compaction event — the bar's most important state,
   unreachable. This is §1's first stated purpose, and it is still miscalibrated.
   The remaining measurement is the **compaction point itself**: run a session
   into a real compaction and record the `used_percentage` it fired at. That
   number, not 100, is the scale §5.4 must derive from. Tracked as C-4 in
   `TODOS.md`.
2. **The percentage is integer-rounded by Claude Code, and the spec never said
   which value the bar uses.** Fixed in §5.3, which now defines `p_exact` and
   `p_shown` once: `p_exact` drives fill and ramp, `p_shown` drives the number
   and the bands. §5.5 and §5.7 defer to it rather than each naming a `pct` of
   their own. The gain is small — at most one cell of fill, near a boundary —
   but the ambiguity was not.
3. **§5.3's context size marker must not be derived from the percentage.**
   `used_percentage × context_window_size` is a rounded product, not a token
   count. Read `context_window_size` directly.
4. **Cost reaches three digits.** An observed session hit
   `total_cost_usd = 107.43094200000006`. §5.7's `FormatFloat(v, 'f', 2, 64)`
   already absorbs the float noise correctly — no spec change needed — but every
   reference state in §5.1 shows a 5-cell cost, so the fitting matrix never
   exercises a wide one. Added as the `wide-cost.json` fixture in §9.1.

> **Two claims retracted.** A first pass through these findings also asserted
> that `total_input_tokens` being window-scoped corrected the document, and that
> §5.6's width reserve underestimated cost. Neither survived checking: §1.2
> already documented the former, and `width_reserve` is a global allowance for
> Claude Code's own notifications, not a per-segment budget. Both are corrected
> above. Measuring something is not the same as the document having been wrong
> about it, and the difference is worth keeping straight in a document whose
> whole purpose is to be trustworthy.

**What M0 did not establish.** One machine, one model (`claude-opus-5[1m]`,
1,000,000-token window), one Claude Code version (2.1.222), one subscriber
account, mid-session throughout. Specifically unverified:

- The **200k window** case. Every observation had `context_window_size =
  1000000` and `exceeds_200k_tokens = false`.
- **`used_percentage` being null early in a session.** Never observed null,
  because capture began mid-session. The startup reference state in §5.1 still
  rests on the docs, not on measurement.
- **Non-subscriber behaviour** for `rate_limits`. Present in every payload here.

Re-run `cc-statusline report` after a fresh session and after a compaction; the
spike stays installed until C-4 in `TODOS.md` closes.

### 3.1.2 Drift detection

The highest-probability failure over twelve months is a payload schema change.
`(value, ok)` accessors turn that into a silently missing segment the user never
notices.

`doctor` therefore diffs the observed payload's key set against the expected set
and reports unknown and missing keys. That is the difference between a bug report
and a quiet uninstall.

### 3.2 Non-payload data

One value is not in the payload: the **git branch name**, read from `HEAD` in the
resolved git directory. No subprocess. Algorithm in §5.8.

> **The diffstat needs no git call.** The `+150/-30` in the mockups comes from
> `cost.total_lines_added` / `cost.total_lines_removed`. These are session-scoped
> counts, which is the more useful quantity here.

> **The dirty flag is deferred to v0.2.** Rendering one asterisk previously
> required a subprocess, a cache, atomic writes, a GC, a timeout context, and a
> lock protocol — the largest subsystem in the document, serving none of §1's four
> questions, and displaying a value that lagged reality by minutes. See §13.

### 3.3 Failure contract

Claude Code blanks the status line if the command exits non-zero or produces no
output. Therefore, **on the render path**:

- **Always exits 0.** Always writes at least one non-empty line. Never writes to
  stderr.
- **Output is buffered.** Both lines are assembled into a `bytes.Buffer` and
  written to stdout with exactly one `Write` call, after all rendering succeeds.
- **No goroutines on the render path.** In Go, a panic in a goroutine that does
  not recover *in that goroutine* kills the process regardless of what `main`
  does. Banning them is what makes `main`'s `recover()` a complete backstop.
- **`recover()` resets the buffer before writing the fallback.** Without the
  reset, buffering accomplishes nothing: a panic after partial assembly would
  emit garbage followed by the fallback line.
- Fallback line is `◆ {display_name}` if the payload decoded that far, otherwise
  the literal `cc-statusline`.
- A payload of `{}`, of `null`, of malformed JSON, or of zero bytes each produce a
  valid line and exit 0.

**Carve-out.** `render --payload <file>` is a debugging path. An unreadable or
malformed file exits 1 with a message on stderr.

**Diagnostics.** `render` never writes to stderr, but on any silent-default or
recovered panic it appends one line to
`$XDG_CACHE_HOME/cc-statusline/last-error.txt`. `doctor` surfaces it. Without
this the failure mode is "it looks slightly wrong forever" with no signal
anywhere.

Absence and null stay distinct through the accessor layer: accessors return
`(value, ok)`, never a sentinel. Each `rate_limits` window may be independently
absent.

---

## 4. Architecture

### 4.1 Binary and subcommands

One static binary, no cgo, no runtime dependencies, **no subprocesses**.

| Invocation | Purpose | Exit codes |
|---|---|---|
| `cc-statusline render [--payload f]` | Render. The hot path. | 0 always; 1 only for an unreadable `--payload` |
| `cc-statusline config` | Bubble Tea wizard with live preview | 0; 1 on unwritable config |
| `cc-statusline init` | Install: write config, patch `settings.json` | 0; 1 on failure |
| `cc-statusline uninstall` | Remove the `statusLine` key surgically | 0; 1 on failure |
| `cc-statusline capture [file]` | Tee stdin + environment to a file, then render | 0 always |
| `cc-statusline preview [--matrix]` | §9.4's gate harness: reference states with a width rule, at capability sets this terminal lacks | 0; 2 on a bad flag |
| `cc-statusline preview --probe` | A column ruler instead of a status line, for measuring C-7 | 0 always |
| `cc-statusline doctor [--json]` | Capabilities, resolved config, payload key diff, last error | 0 usable; 1 only if config is malformed |
| `cc-statusline version` | Version, commit, build date | 0 |

**`render` is required, not inferred.** Sniffing stdin with
`os.Stdin.Stat()` / `ModeCharDevice` misfires on `/dev/null`, file redirects, and
closed descriptors — that is, under cron, under CI, and under the `env -i` test
§9.3 mandates. `init` writes the subcommand explicitly, so there is no cost to
requiring it. Bare invocation prints usage and exits 0.

**`doctor` exits 0 when `git` is absent.** The branch segment is cosmetic and
§9.3 requires render to succeed without git; exit 1 would mean "unusable" and
would trip health checks over a missing branch name.

**`preview` is a development command that ships anyway.** It is how §9.4's gate
is run, and a gate whose instrument is not in the released binary can only be
run by someone with a Go toolchain and a checkout — which excludes every user
who reports "the icons look wrong on my terminal". It costs one file, the
embedded reference payloads §10.4's wizard needs regardless, and nothing on the
`render` path. It reads the embedded defaults and never `~/.config`: §5.1's
criteria are for the default preset, and a gate run against the developer's own
config is a gate on that file.

**`capture` records the environment, not just the payload.** It writes the raw
stdin JSON plus `COLUMNS`, `LINES`, `TERM`, `COLORTERM`, `LANG`, `LC_CTYPE`, and
`NO_COLOR` into a sidecar. Without those, §6's capability model is unverifiable
and §10.3's wizard cannot reproduce the real render environment. It defaults to
`$XDG_CACHE_HOME/cc-statusline/last-payload.json` and always writes that path in
addition to any explicit file. A `capture` write failure never affects the render
or the exit code.

### 4.2 Package layout

Seven internal packages — five at rev 4, plus `refstate` at M4 and `settings` at
M5. Packages that always change together are one package.

```
cc-statusline/
├── main.go                     # arg dispatch + top-level recover, no logic
├── go.mod
├── cmd/
│   ├── render.go   config.go   init.go   uninstall.go
│   ├── capture.go  doctor.go   version.go  preview.go
├── internal/
│   ├── payload/                # stdin JSON structs, (value, ok) accessors
│   │   ├── payload.go  accessors.go  keydiff.go
│   ├── config/                 # TOML load, XDG resolve, env overlay, defaults
│   │   ├── config.go  defaults.go  env.go  paths.go  validate.go
│   ├── style/                  # colors, gradient, glyph sets, capability resolution
│   │   ├── capabilities.go     # the Capabilities struct — see §6.4
│   │   ├── profile.go  gradient.go  glyph.go  theme.go
│   ├── line/                   # segments + assembly + fitting
│   │   ├── segment.go          # Segment, Rendered, Truncatable
│   │   ├── model.go   context.go   cost.go     duration.go
│   │   ├── ratelimit.go branch.go   diffstat.go project.go
│   │   ├── join.go    fit.go    width.go   registry.go
│   ├── gitinfo/
│   │   └── discover.go         # upward walk, gitdir indirection, HEAD parse
│   ├── settings/               # the settings.json editor — see below
│   │   ├── settings.go
│   │   └── testdata/*.json     # the §9.3 corpus
│   └── refstate/               # §5.1's payloads, embedded — see below
│       ├── refstate.go
│       └── payloads/*.json     # + *.git.json sidecars, see §9.1
├── config/
│   ├── config.example.toml
│   └── presets/{default,minimal}.toml
├── testdata/
│   └── golden/{plain,styled}/
├── docs/PRD.md
├── README.md
├── .goreleaser.yaml
├── .github/workflows/release.yml
├── install.sh
└── Makefile
```

No `-tags minimal` build tags. §13 defers that pending measurement, and §8.1
already declares the runtime init floor uncontrollable, so the tags would be
speculative complexity.

**`refstate` is a package rather than testdata, added at M4.** The §5.1 payloads
are consumed by three things and only one of them is a test: the acceptance
criteria in `internal/line`, the §9.2 goldens, and §9.4's `preview`. §10.4 makes
it four — the wizard falls back to a bundled fixture when the cache is empty. A
`testdata/` directory reaches none of the non-test consumers, and `go install`
leaves no repository on disk for them to read from, so the bytes are embedded.
The alternative was four copies of the payload the gate is supposed to validate.

**`settings` is a package rather than code in `cmd/`, added at M5.** Editing
someone else's configuration file is the riskiest thing this program does, and
the rules that make it safe — refuse a file that is not plain JSON, resolve
symlinks to their target, preserve mode, back up before writing, temp-and-rename,
restore the trailing newline, insert at the file's own indentation — are the
same rules for `init` and `uninstall` and would have been duplicated across two
command files. §9.3's corpus tests the rules directly rather than through a
command, which is why the corpus lives beside them.

> The tree above still lists `internal/line` as one file per segment and
> `internal/config` with `env.go` / `paths.go`. The implementation put the eight
> segments in `segments.go` and the resolution in `load.go`. That drift is
> cosmetic and is noted rather than resolved, because the file names are not
> what §4.2 is asserting — the package boundaries are.
>
> The §9.3 settings corpus moved from a repository-root `testdata/` into
> `internal/settings/testdata/`, because Go only makes a `testdata` directory
> available to the package it sits beside. `internal/line`'s goldens stay where
> §4.2 puts them for the same reason, in reverse.

### 4.3 The Segment interface

```go
// Rendered carries both the styled bytes and the unstyled text used for
// width measurement. Width is never computed from the styled form.
type Rendered struct {
    Styled string
    Plain  string
}

type Segment interface {
    Name() string
    // A zero Rendered (empty Plain) means "no data" — the joiner omits the
    // segment AND its adjacent separator, rather than emitting an empty gap.
    Render(ctx Context) Rendered
}

// Truncatable is implemented by segments that can shrink in place.
// Implemented by: branch (floor 8 cells), model (floor 10 cells).
type Truncatable interface {
    Truncate(r Rendered, cells int) Rendered
}

// Context carries everything a segment may read. Segments never perform I/O,
// never read the environment, and never learn the terminal width.
type Context struct {
    Payload *payload.Payload
    Git     gitinfo.Info
    Config  *config.Config
    Style   *style.Style        // already resolved from Capabilities
}
```

Three rules: segments never perform I/O; empty means absent; segments know
nothing about layout. Fitting is entirely `line/fit.go`'s job.

### 4.4 Render pipeline

```
  stdin ──► payload.Decode ──────────────┐
                                          │
  env ────► style.Detect ──► Capabilities ├──► config.Load ──► Context
                                          │         ▲
  cwd ────► gitinfo.Discover ─────────────┘         │
            (.git/HEAD, no subprocess)         embedded defaults
                                                    │
                                               file (XDG)
                                                    │
                                               CC_STATUSLINE_* env

  Context ──► for each [[line]]:
                for each segment:  Render(ctx) ──► Rendered{Styled, Plain}
                                                        │
                                   join (plain | powerline)
                                                        │
                                   fit(available) ──► drop ──► truncate ──► clip
                                                        │
              ─────────────────────► bytes.Buffer ◄─────┘
                                          │
                     one os.Stdout.Write ──┘   (panic here ⇒ buffer.Reset + fallback)
```

Every arrow is in-process. Nothing forks, nothing dials, nothing awaits.

---

## 5. Visual specification

### 5.1 Reference states

Byte-identical acceptance criteria for the default preset, **at 82 columns or
wider**.

> **The width qualifier is new in rev 7, and it is not cosmetic.** M3 measured
> the four states at 62, 59, 70, and 44 cells. §5.6's budget is
> `COLUMNS - 2×padding - width_reserve`, so the default 80 columns leaves 68 —
> two cells short of the danger state, which therefore drops its duration and
> renders as
> `◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 5h:85% 7d:62%`.
>
> That is the fitter behaving exactly as §5.6 specifies. What was wrong was
> §5.1, which stated four byte-identical criteria without stating the width at
> which any of them holds — so the danger state was unsatisfiable at the
> document's own default. `TestReferenceStatesAtEighty` pins the 80-column
> rendering so the next person to notice `45m` missing finds a test rather than
> files a bug.
>
> Whether `width_reserve = 12` is the right number is a separate question, and
> one nobody can answer from a document: it exists to avoid Claude Code's own
> notifications, and 10 would make the danger state fit at 80 exactly. Settled
> at M4, in front of a terminal.

**Normal (42%)**
```
◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42% │ $0.85 │ 3m │ 5h:15% 7d:8%
⎇ main │ +150/-30 │ my-project
```

**Warning (75%)**
```
◆ Claude Sonnet 4.6 │ ▓▓▓▓▓▓▓▓░░ 75% │ $3.20 │ 12m │ 5h:48%
⎇ feat/auth │ +280/-45 │ my-project
```

**Danger (92%)**
```
◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 45m │ 5h:85% 7d:62%
⎇ main │ +500/-120 │ api-server
```

**Startup**
```
◆ Claude Opus 4.6 │ ░░░░░░░░░░ 0% 1M │ $0.00
claude-temp
```

> **Deltas from the original mockups, with reasons.**
> (a) Danger bar is 9 cells, not 10: `round(92 × 10 / 100) = 9`, and no single
> rounding rule produces the original (4, 8, 10) triple.
> (b) Duration is minute-granular (`3m`, not `3m42s`). A seconds digit ticking in
> peripheral vision contradicts §1, and it forced a 10-second refresh timer.
> (c) The dirty asterisk is gone; deferred to v0.2 per §3.2.
> (d) Startup model name is `Claude Opus 4.6`, matching the others.
> (e) The bar and percent are **one segment** (§5.3), which is what makes the
> single-space join between them expressible at all.

### 5.2 Line composition

**Line 1:** `model` → `context` → `cost` → `duration` → `ratelimits`
**Line 2:** `branch` → `diffstat` → `project` (omitted entirely if all empty)

Default separator is ` │ ` (U+2502). Output is `line1\nline2\n`, or `line1\n`
when line 2 is empty. Exactly one trailing newline.

### 5.3 Segment detail

Eight segments.

| Segment | Renders | Style keys | Empty when |
|---|---|---|---|
| `model` | `◆ {name}` | `model_marker`, `model_name` | never |
| `context` | `{bar} {n}%{warn}{size}` | §5.5, band color | `context_window` absent |
| `cost` | `${n}` 2dp | `cost` | never |
| `duration` | `{m}m` / `{h}h{m}m` / `{d}d{h}h` | `duration` | `total_duration_ms < 60000` |
| `ratelimits` | per-window, §5.7 | `ratelimit`, `warning` above threshold | both windows absent |
| `branch` | `⎇ {name}` | `branch` | no git dir found |
| `diffstat` | `+{a}/-{r}` | `added`, `diffstat_delim`, `removed` | both counts zero |
| `project` | basename of `project_dir` | `project` | never |

> **The single-window rate-limit segments — added after M8.** Two more, which
> makes ten. They are the same two windows `ratelimits` already renders, one
> segment each. `ratelimits` is unchanged and stays the default.
>
> | Segment | Renders | Empty when |
> |---|---|---|
> | `ratelimit_5h` | `5h:15%`, or `5h:15% ↻ 12:30` | that window's `used_percentage` absent |
> | `ratelimit_7d` | `7d:8%`, or `7d:8% ↻ 10 Aug 12:30` | that window's `used_percentage` absent |
>
> Style keys are `ratelimits`': the number takes `ratelimit`, or `warning` at or
> above `thresholds.ratelimit_warn` (§5.4). The marker and the time always take
> `ratelimit` — the number is what changes state, and a clock painted `warning`
> says the time is alarming, which it never is.
>
> **Why segments and not placeholders on `ratelimits`.** Everything a user wants
> from the *pair* is a layout decision — one window per row, different `drop`
> priorities, a `{name="flex"}` between them — and layout is expressed in §7.2 by
> where a segment sits in a `[[line]]` block. A segment is the unit the fitter
> drops, truncates and positions, so none of those is reachable from inside one
> at any price. Placeholders would have made the text configurable and left all
> three impossible.
>
> **And why the reset time is not a segment.** The same argument from the other
> side. `5h:15%` and `5h:15% ↻ 12:30` differ in none of the things being a
> segment buys — not where they sit, not what they cost, not whether the fitter
> may drop them — so they are two values of `format`, and §5.7's `{reset}` is the
> whole of what decides which you get. Making them separate segments (the first
> attempt did) bought four names, four config tables and four sidebar rows for
> zero expressiveness. §10.4 is the ring the wizard cycles them with.
>
> **`{icon}` and `{reset}` carry their own leading space**, the rule this section
> already states for `{warn}` and `{size}`. `resets_at` is optional
> independently of `used_percentage`, so one format string produces both
> `5h:15% ↻ 12:30` and `5h:15%`. They are one decision, never two lookups: a
> payload with no `resets_at` must not leave a lone `↻` on the line. A trailing
> placeholder that renders empty takes any space in front of it too — `§4.3`'s
> rule that an absent thing takes its own spacing, applied at the end of a format
> the way `{bar}` already applies it at the start.
>
> **The reset time is a §5.6 stage-2 truncation, all or nothing.** A window
> showing one is the widest segment in the schema, and the clock is the half
> worth losing first — `↻ 12:3` and `↻ 10 A` are not less precise, they are
> wrong. Stage 2 asks for the bare percentage or nothing, which is `Truncatable`
> refusing exactly as that section allows. A window wearing a format with no
> `{reset}` in it stays `Truncatable` and always refuses: stage 2 skips a segment
> that is not `Truncatable` at all, so the alternative is a segment being
> reachable by the fitter or not depending on which format it is wearing — a
> presentation choice reaching into layout.
>
> **The zone is an input, not something a segment resolves.** `resets_at` is unix
> epoch seconds (§3.1.1) and means nothing without a location, so `line.Context`
> carries one and `cmd` sets it to `time.Local`. A segment reading `time.Local`
> for itself would be an input no test and no preview could vary — and a golden
> that read the machine's zone would pass on one laptop and fail in CI with
> nothing in the diff to say why.

> **The `effort` segment — added after M8.** Eleven. §12 Q3 asked which of
> `effort.level`, `fast_mode`, `thinking.enabled`, `pr.number` and
> `session_name` earn a segment; this settles the first and leaves the rest
> open.
>
> | Segment | Renders | Style keys | Empty when |
> |---|---|---|---|
> | `effort` | `Effort: high` | `effort` | `effort` absent from the payload |
>
> It is on neither shipped line. `ratelimits` and `context` answer questions
> that change minute to minute; the effort level changes when someone changes
> it, and a row already competing with Claude Code's own output does not spend
> cells on a constant by default.
>
> **Absence is the ordinary state.** Claude Code sends the `effort` object only
> when the current model supports the parameter (§3.1), so a session on a model
> without one renders nothing here and takes its separator with it — §4.3, not a
> degraded state. There is no level to fall back to: "this model has no effort
> setting" and "the effort is low" are different facts, and rendering the second
> for the first would be inventing data.
>
> **The level is printed as it arrived.** §3.1 names five — `low`, `medium`,
> `high`, `xhigh`, `max` — and this segment checks against none of them. A sixth
> is exactly the schema change §3.1.2 calls the likeliest failure of the next
> twelve months, and printing it is how anyone finds out; an allow-list would
> blank a segment that was holding the right answer, silently, which is the
> failure mode §3.1.2 exists to keep visible.
>
> **It is the one segment whose default carries its own label**, inverting the
> axis §10.4's ring is built on. Every other compact form identifies itself with
> a unit or a glyph — `%`, `$`, `+`/`-`, the bar, `◆`, `⎇` — and `high` sitting
> on a line of numbers identifies nothing. §6.2's table has no glyph that reads
> as "reasoning effort", and adding one means choosing three codepoints against a
> terminal §9.4's gate has not looked at, so the word does that work instead.
> `tab` offers the bare `{level}` as the alternative, and a user who wants a
> glyph types one into `format`, which needs nothing from the icon set.
>
> **No threshold, no bands.** `max` is not a danger state and `low` is not a
> warning; the level is a setting, not a measurement, so §5.4 does not apply and
> the segment paints flat in `[colors] effort`.

**Why `bar` and `percent` are one segment.** The reference states join them with
a single space while every other junction uses ` │ `, and §7.2 has exactly one
global separator. As two segments the default preset could not produce §5.1 and
§9.3's first criterion would be unsatisfiable. They are also one visual unit: the
`⚠` and size markers straddle what used to be the boundary, and dropping the bar
while keeping the percent — which is what the fitter should do — is an internal
concern, not a segment-level one.

**Internal spacing is explicit.** `{warn}` renders `" ⚠"` in the danger band and
`""` otherwise. `{size}` renders `" 1M"` when it applies and `""` otherwise. The
leading spaces belong to the placeholders, which is what makes `42%` and
`92% ⚠ 1M` both correct from one format string.

**The percent has one definition, used everywhere.** M0 measured that Claude
Code's `used_percentage` is `round(total_input_tokens / context_window_size ×
100)` — an integer. We have the operands, so we compute the exact value and let
Claude Code's rounding go:

```
p_exact = 100 × total_input_tokens / context_window_size    (both present, size > 0)
        = used_percentage                                   (fallback)
        = 0                                                 (neither present)
p_shown = clamp(round(p_exact), 0, 100)
```

`p_exact` drives the bar fill and the gradient ramp (§5.5). `p_shown` is the
number on screen and the value the bands compare against (§5.4). Bar fill from
`p_exact` differs from fill computed off the rounded integer by at most one cell,
and only near a cell boundary — a small gain, taken because the better operands
cost nothing. The ramp benefits more, being continuous.

Note `total_input_tokens`, not the summed `current_usage` fields: M0 measured
them equal in 33/33 payloads, and one field cannot disagree with itself.

**Null percent renders as zero.** With both operands absent and no
`used_percentage`, `p_exact` is `0` for both the bar and the number. The segment
is empty only when `context_window` is absent entirely. This is what makes the
startup state render `░░░░░░░░░░ 0%`.

**The context size marker** is governed by `[context] show_size`:
`"non_default"` (default) renders only when `context_window_size != 200000`, and
then in every state; `"always"` renders always; `"never"` suppresses. Under the
default, presence carries information instead of flickering on at a threshold.

**Duration's one-minute floor.** `total_duration_ms` is wall-clock since session
start and is never truly `0` at first render. Below one minute the segment is
empty, which is what the clean startup mockup depicts.

### 5.4 Thresholds

| Band | Range | Color key |
|---|---|---|
| normal | `0 ≤ p < thresholds.warning` | `normal` |
| warning | `thresholds.warning ≤ p < thresholds.danger` | `warning` |
| danger | `thresholds.danger ≤ p ≤ 100` | `danger` |

Defaults `warning = 70`, `danger = 85`, compared against `p_shown` (§5.3) — the
**rounded integer** percent — so the displayed number and its color never
disagree. `p_exact` drives only the bar's fill and ramp.

> **Still provisional, and now for a known reason.** M0 answered the question
> §3.1.1 posed: the percent measures the **raw** context window, so it does not
> account for the autocompact threshold. That means 100 is not the ceiling that
> matters, and `danger = 85` may sit *after* the point compaction fires. These
> defaults cannot be locked until the compaction point is measured — C-4 in
> `TODOS.md`.

Rate limits use `thresholds.ratelimit_warn = 80` and have two states only:
below it `colors.ratelimit`, at or above `colors.warning`.

### 5.5 The bar

**Fill.** `filled = clamp(int(math.Round(p_exact × width / 100)), 0, width)`,
with `p_exact` as defined in §5.3 — never the rounded integer.
Verified: 42 → 4, 75 → 8, 92 → 9, 0 → 0.

**Ramp.** `gradient_stops` is an ordered list spaced **evenly** across `[0,1]`.
`ramp(t)` interpolates **linearly in sRGB** between bracketing stops, `t` clamped
to `[0,1]`. sRGB is named explicitly because goldens are byte-identical and the
color space cannot be left to a library default.

**Fill-relative coloring**, when truecolor is available and `gradient = true`:

```
color(i) = ramp( (p_exact/100) × (i+1)/filled )   for i ∈ [0, filled)
```

`filled == 0` lights no cells and never evaluates the ramp. `filled == 1`
(including `bar.width == 1`) gives that cell `ramp(p_exact/100)`. The visible ramp
compresses and expands with the fill level: green through yellow at 42%, the full
green through red at 92%, which is what the reference mockups depict.

**Otherwise** — `gradient = false`, or no truecolor — every filled cell takes the
band color. Shape identical, coloring degrades. Empty cells always take
`colors.bar_empty`.

### 5.6 Width handling and fitting

```
available = max(20, COLUMNS - (2 × padding) - width_reserve)
```

`COLUMNS` from the environment if set and parseable, else `general.max_width` if
non-zero, else `80`. **No ioctl:** Claude Code captures stdout, so `TIOCGWINSZ`
on fds 0/1/2 fails and `tput cols` cannot work. `width_reserve` defaults to `12`
because Claude Code renders system notifications on the right of the same row.

All width math runs on `Rendered.Plain` via `github.com/mattn/go-runewidth`,
never `len()` or `utf8.RuneCountInString`. ANSI-stripping at measure time is
prohibited.

> **Ambiguous-width glyphs — corrected at M3.** Rev 6 asserted that `▓` `░` `◆`
> `⚠` are East Asian Ambiguous. Measured against Unicode's `EastAsianWidth.txt`,
> the list was wrong in both directions:
>
> | glyph | | class | | glyph | | class |
> |---|---|---|---|---|---|---|
> | `▓` | U+2593 dark shade | **A** | | `⚠` | U+26A0 warning sign | **N** ← claimed A |
> | `◆` | U+25C6 black diamond | **A** | | `░` | U+2591 light shade | **N** ← claimed A |
> | `│` | U+2502 box drawings | **A** ← omitted | | `⎇` | U+2387 alternative key | **N** |
> | `…` | U+2026 ellipsis | **A** ← omitted | | | | |
>
> U+2591 is the odd one out among the shade blocks — U+2592 and U+2593 are both
> Ambiguous — and that single discrepancy is load-bearing. **The bar's filled and
> empty cells would fall in different width classes**, so under a CJK locale a
> ten-cell bar is ten columns at 0% and twenty at 100%: line 1 grows by up to ten
> columns as the session fills, and the fitter drops the cost and then the
> duration for a reason that has nothing to do with either.
>
> **Rule.** The bar's two cells must share a display width. When they do not and
> neither was set explicitly, the empty cell is substituted — `▒` U+2592, which
> is Ambiguous and is `░`'s nearest visual relative. Under `ambiguous_width = 1`
> the substitution never fires and the default appearance is unchanged. An
> explicitly configured `[bar].filled` or `[bar].empty` is left alone, per §6.2's
> override rule; silently replacing a glyph a user named is a worse surprise than
> the wobble.
>
> The Nerd Font column is entirely Private Use Area, which is uniformly
> Ambiguous, so it is already consistent.
>
> `runewidth.EastAsianWidth` is set from `LC_ALL` / `LC_CTYPE` / `LANG` on a
> per-`Style` `runewidth.Condition`, never the package global — §6.4's purity
> requirement, and what lets M7's wizard preview a locale it is not running in.
> `[general] ambiguous_width = "auto" | 1 | 2` is the explicit escape hatch. See
> §9.4: goldens measure with the same library the code does, so they can confirm
> the arithmetic is self-consistent and never that a terminal agrees with it.

**Fitting** runs per line, independently, in three escalating stages:

```
    line exceeds available?
             │
      ┌──────┴──────┐
      │   STAGE 1   │  drop highest `drop` value remaining on THIS line
      │    DROP     │  ties break rightmost-first; 99 = never drop
      └──────┬──────┘
             │ still over?
      ┌──────┴──────┐
      │   STAGE 2   │  ask each Truncatable, ascending drop order
      │  TRUNCATE   │  branch floors at 8 cells, model at 10, bar at 3
      └──────┬──────┘
             │ still over?
      ┌──────┴──────┐
      │   STAGE 3   │  hard clip to `available` at a rune boundary,
      │    CLIP     │  append ANSI reset
      └─────────────┘
```

Stage 3 is what makes never-wrap a guarantee rather than an aspiration.

**Drop priorities reflect the information hierarchy in §1**, not visual weight:

| Line 1 | `model` | `context` | `ratelimits` | `cost` | `duration` |
|---|---|---|---|---|---|
| drop | 99 | 99 | 3 | 4 | 5 |

| Line 2 | `branch` | `diffstat` | `project` |
|---|---|---|---|
| drop | 99 | 2 | 1 |

`duration` dies first and `ratelimits` last among the droppable, because rate
limits are §1's question 3 and the duration is the least actionable thing on the
line. The bar no longer competes for a drop slot at all: it lives inside
`context` and shrinks there.

**Two rules M3 had to add, because the diagram above underspecified stage 2.**

*Ties in stage 2 break rightmost-first*, the same rule §5.6 already states for
stage 1. It is not only consistency: on the default preset `model` and `context`
are both marked never-drop, so the tie-break is what decides which gives way.
Rightmost-first asks `context` first, so the bar sheds cells before the model
name loses characters — and the bar's information survives intact in the `42%`
beside it, while the model name's does not survive anywhere.

*The bar floors at 3 cells and is dropped outright below that.* §5.6 gave floors
for the branch and the model and none for the bar, and the omission shows: a
two-cell bar quantises every percentage into halves and reads as a rendering
artefact. Dropping it also frees more room than shrinking it, because `{bar} `
takes its trailing space with it. The percentage is never touched — `92%` already
says everything the bar says, and the bar's value is peripheral-vision *shape*,
which is exactly what does not survive being two cells wide.

**Stage 2 may refuse.** A segment at its floor returns unchanged, which is
legitimate and is why stage 3 exists. A `Truncatable` takes the render context
and re-renders at a target width rather than being handed its own output to cut
down: cutting a string that holds escape sequences loses either a colour or a
reset, and leaves nowhere to put the ellipsis in the right colour.

> **The flex marker — added after M8.** Every stage above answers "the line is
> too wide"; nothing answered "the line is too narrow", and the width a row does
> not use is width the user cannot spend. `{name="flex"}` in a `segments` list is
> a position rather than a segment: it takes the leftover, so what follows it
> lands on the right edge of `available`.
>
> ```
>   segments = [{name="model", drop=99}, {name="flex"}, {name="cost", drop=4}]
>
>   ◆ Claude Opus 4.6                                              $15.30
>   └────────────────────────── available ─────────────────────────────┘
> ```
>
> Two markers split the leftover evenly — three groups, remainder to the left —
> which is how a segment gets centred. A leading marker right-aligns the row.
>
> **It is not in §5.3's segment list, and must not be.** That list is what
> `line.New` builds a renderer for; a marker renders no information and reads no
> payload. It is a second, one-word vocabulary the validator accepts, which is
> also what keeps it out of the wizard's disabled pool — there is no "off" for a
> thing that may appear any number of times.
>
> **Four rules, each of which the stages above would otherwise get wrong.**
>
> *The stages measure it at its floor.* Drop and truncate run against the line's
> own content, and only what survives is widened. Measured expanded, a marker
> would drop a segment to make room for whitespace.
>
> *Its floor is one cell, and zero at the head of a line.* A marker stands where
> a separator would, so it owes the cell that keeps two segments from touching —
> and ` │ ` is not drawn beside it, because it *is* the separator there. At the
> head there is nothing on the left to separate from, and the cell would read as
> an indent nobody configured.
>
> *A marker with nothing to its right is dropped.* It would be trailing
> whitespace. This is not only about what the file writes: §4.3 removes empty
> segments and stage 1 drops surviving ones, so `cost │ flex │ duration` ends in
> a marker on every session too young to have a duration. A row left holding
> nothing but markers is therefore empty, and §5.2 omits it.
>
> *It carries no `drop`.* There is no width at which removing it helps —
> one cell at the floor, and it gives back everything above that. Validation
> pins it at 99 and the file is written without a priority, rather than
> honouring a number the fitter has no rule for.
>
> **A flexed row is exactly `available` cells**, where every other row is at
> most that. `available` is already net of padding and `width_reserve`, whose
> default of 12 is why this does not put the cursor on the terminal's own wrap
> column; a user who sets `width_reserve = 0` and a flex is asking for the last
> column and will get it.

### 5.7 Formatting rules

**Percent.** `p_shown = int(math.Round(p_exact))`, half away from zero, clamped
`[0,100]`. `p_exact` is defined once in §5.3; no other section computes it.

**Zero window.** `context_window_size` of `0` or absent renders an all-empty bar
and `0%`. Never divide by it.

**Cost.** `$` + `strconv.FormatFloat(v, 'f', 2, 64)`. No separator, no locale.

**Duration.** Minute granularity, selected by magnitude:

| Range | Key | Default | Example |
|---|---|---|---|
| `< 3600s` | `under_hour` | `{m}m` | `3m`, `45m` |
| `< 86400s` | `over_hour` | `{h}h{m}m` | `1h5m` |
| `≥ 86400s` | `over_day` | `{d}d{h}h` | `2d3h` |

`duration.pad = true` zero-pads the minor unit (`1h05m`). Default `false`.

**Rate limits.** Two independent templates plus a joiner, so a single present
window renders cleanly. An absent window contributes nothing and its joiner
drops.

> **`reset_format` — added after M8.** A second kind of format string, and the
> only one whose mistakes are invisible. `time.Format` copies through anything it
> does not recognise, so `reset_format = "HH:mm"` is a legal Go layout that
> renders the literal text `HH:mm` on the status line forever, with no error
> anywhere.
>
> Validation is therefore not a placeholder table but a probe: a layout that
> formats to *itself* substituted nothing, and a layout that substituted nothing
> is a literal string the user believed was a pattern. That rejects `HH:mm` and
> `%H:%M` — the two things a person reaches for first — while accepting anything
> that does substitute, literal text around it included. The table lives in
> `config.TimeKeys`, beside `FormatKeys` and for the same reason.
>
> Accepting literal text is not laxity, it is the feature: `reset_format =
> "resets 15:04"` renders `resets 12:30`, and the word disappears with the time
> it introduces. That is why §10.4's labelled formats put the word there rather
> than in `format`, where a payload carrying no `resets_at` would leave it
> stranded — a sentence with its object missing, which is the failure `{icon}`
> exists to prevent.

**Context size label.** `1M` for exactly `1000000`; `{n}k` where
`n = round(size/1000)` for sizes ≥ 1000; the raw integer otherwise.

**Format-string grammar — deliberately minimal.** `{name}` substitutes. That is
the entire grammar. No padding syntax, no escapes, no literal-token fallback.
Zero-padding is a per-segment boolean where it applies. An unknown placeholder is
a validation error in `config` / `init` / `doctor`, and in `render` the segment
falls back to its embedded default format.

Valid placeholders:

| Config key | Placeholders |
|---|---|
| `segments.duration.{under_hour,over_hour,over_day}` | `{d}` `{h}` `{m}` |
| `segments.ratelimits.{five_format,seven_format}` | `{n}` |
| `segments.ratelimit_{5h,7d}.format` | `{n}` `{icon}` `{reset}` |
| `segments.diffstat.format` | `{added}` `{removed}` |
| `segments.cost.format` | `{n}` |
| `segments.context.format` | `{bar}` `{n}` `{warn}` `{size}` |
| `segments.model.format` | `{name}` `{marker}` |
| `segments.effort.format` | `{level}` |
| `segments.branch.format` | `{name}` |
| `segments.project.format` | `{name}` |

This table is defined **once**, with a getter and setter attached to each row.
The validator walks it to repair; `internal/line` walks it in a test to prove
every listed placeholder actually renders. Duplicating it is the one repetition
that would silently drift: validation would pass while render emitted literal
`{braces}`.

> **Restructured after M8.** `config.FormatKeys` is still the table the
> validator walks, but it is a view now rather than a source: the rows are read
> out of `config.SegmentDefs` in `internal/config/schema.go`, where each segment
> declares its keys once alongside its defaults, its colours and its `tab` ring.
> The placeholders in the table above are that declaration's `Fields`, and each
> field carries a `Kind` — the type of the value it stands for — which is what
> decides the formatting rules stated further up this section.
>
> `TimeKeys` was a second hand-written table beside `FormatKeys`, and the only
> way to discover it existed was to add a time layout to the first and watch
> nothing validate it. Both are now the same list, split apart by a `Syntax`
> field on each key.

Two consequences M3 made explicit:

- **The grammar itself is also defined once** — `config.Tokenize`, used by the
  validator and by the segment expander. Two scanners would become two grammars
  the moment one of them handled an unterminated `{` differently, and the symptom
  would be a format that validates cleanly and renders as braces. An unterminated
  `{` is literal text.
- **The duration segment supplies all three of `{d}` `{h}` `{m}` in every
  branch**, not just the ones its default format for that branch uses. The table
  gives all three to all three keys, so `under_hour = "{h}h{m}m"` must render
  `0h45m` — supplying only the subset would let that config validate and then
  print `{h}` on the status line. Only which unit `pad` zero-fills varies by
  branch.

The same treatment applies to `[colors]`: `config.ColorKeys` enumerates the
sixteen scalar keys with accessors — read out of `config.ColorDefs`, which holds
each key's default beside it — and `internal/style` resolves through it rather
than through a switch of its own. `Paint` returns text unstyled for an
unknown key — a colour is decoration, and a render path that can fail on
decoration can blank the line — which is precisely why a key missing from one
side must be impossible to introduce rather than tolerated at runtime.

### 5.8 Git discovery

Runs once, before the segment loop. **No subprocess.** All path resolution uses
`workspace.current_dir` from the payload; `os.Getwd()` is never called, because it
returns `ENOENT` when the directory has been deleted mid-session.

1. Start at `workspace.current_dir`. Walk up looking for `.git`. Stop at any
   `GIT_CEILING_DIRECTORIES` component. Cap at 64 levels. If `GIT_DIR` is set,
   use it and skip the walk.
2. `.git` a **directory** → that is the git dir.
3. `.git` a **file** → read `gitdir: <path>`, resolve relative to the containing
   directory when not absolute (submodules write relative paths). Follow at most
   one level of indirection.
4. Read `HEAD`. `ref: refs/heads/<name>` → `<name>`. A 40-hex line → first 7
   characters.
5. If `<gitdir>/rebase-merge/head-name` exists, prefer its branch name.

Resolving only the branch **name** means `packed-refs` is never read. Bare repos
yield no branch and the segment renders empty. `git.branch_max_len` (default 24)
applies unconditionally at render; §5.6's fitter may reduce further to 8.

---

## 6. Terminal capability degradation

### 6.1 Environment contract

| Variable | Default | Effect |
|---|---|---|
| `CC_STATUSLINE_ASCII` | `0` | `1` selects the ASCII column of §6.2 |
| `CC_STATUSLINE_NERDFONT` | `0` | `1` selects the Nerd Font column |
| `CC_STATUSLINE_POWERLINE` | follows `NERDFONT` | `1` uses Powerline arrow separators |
| `CC_STATUSLINE_COLOR` | unset | `none` \| `16` \| `256` \| `truecolor` |
| `CC_STATUSLINE_CONFIG` | unset | Config file path, replacing the XDG lookup |
| `CC_STATUSLINE_NO_GIT` | `0` | `1` disables the branch segment |
| `NO_COLOR` | unset | Any value forces no color |
| `COLORTERM` | (system) | `truecolor` / `24bit` informs `auto` detection |

Authoritative table; §7.3 references it. `POWERLINE` follows `NERDFONT` because
Powerline separators need a patched font. `ASCII` beats `NERDFONT` when both are
`1`, since ASCII is the compatibility floor.

> **Naming note.** Renamed from `CLAUDE_STATUSLINE_*` so the variables do not read
> as official Anthropic configuration.

### 6.2 Glyph table

Defaults per icon set. An explicitly-set `[bar].filled`, `[bar].empty`, or
`[general].separator` overrides the table for all icon sets; `ASCII=1` selects the
ASCII column but does **not** rewrite explicit values. Shipped presets use the
sentinel `"auto"` so the icon set stays effective.

The Nerd Font column is written as codepoints, not as literals. They live in the
Private Use Area, so a literal renders as a replacement box in any editor without
a patched font and survives copy-paste badly — which it did not, here: three of
these cells were empty in this document until the `reset` row was added beside
them. `internal/style/glyph.go` writes them the same way, for the same reason.

| Role | ASCII | Unicode | Nerd Font |
|---|---|---|---|
| model marker | `*` | `◆` | `U+F06A9` nf-md-robot |
| bar filled | `#` | `▓` | `▓` |
| bar empty | `-` | `░` | `░` |
| separator | `\|` | `│` | `│` |
| powerline sep | n/a | n/a | `U+E0B0` nf-pl-left_hard_divider |
| branch | `>` | `⎇` | `U+E725` nf-dev-git_branch |
| danger | `!` | `⚠` | `U+F071` nf-fa-warning |
| ellipsis | `.` | `…` | `…` |
| reset | `@` | `↻` | `U+F021` nf-fa-refresh |

> **`reset` was added after M8**, for §5.3's rate-limit reset segments. `@` reads
> as "at" in `5h:15% @ 12:30`, where the obvious `~` would read as
> "approximately" beside a number. `↻` is U+21BB rather than a clock face:
> every clock glyph in Unicode is either an emoji presentation two cells wide or
> a codepoint no terminal font carries, and U+21BB is Neutral width, so it stays
> one cell under a CJK locale — which the fixed-width time beside it depends on.

ASCII normal state:
```
* Claude Opus 4.6 | ####------ 42% | $0.85 | 3m | 5h:15% 7d:8%
> main | +150/-30 | my-project
```

### 6.3 Color profile resolution

First match wins:

1. `NO_COLOR` set → `none`
2. `TERM` is `dumb`, empty, or unset → `none`
3. `CC_STATUSLINE_COLOR` set to a valid value → use it
4. `general.color` from config, if not `"auto"` → use it
5. `COLORTERM` is `truecolor` or `24bit` → `truecolor`
6. `TERM` contains `256color` → `256`
7. otherwise → `16`

`TERM=dumb` is checked before `COLORTERM` because many shells export `COLORTERM`
globally from a profile.

### 6.4 Capability resolution is a pure function

```go
type Capabilities struct {
    Icons      IconSet   // ascii | unicode | nerdfont
    Powerline  bool
    Profile    termenv.Profile
    Ambiguous  int       // 1 or 2
    Columns    int
}

func Detect(env map[string]string, cfg *config.Config) Capabilities
func NewStyle(c Capabilities, cfg *config.Config) *Style
```

**Environment is read exactly once, in `main`, and passed as a map.** Everything
downstream takes `Capabilities` explicitly.

This is not stylistic. §10.3's wizard must flip the live preview across three icon
sets, two separator styles, four color profiles, and arbitrary widths. If
resolution reads `os.Getenv` directly, the wizard can only do that by mutating its
own process environment, and the M7 milestone turns into a refactor of M2 and M3
code. Writing it as a pure function now costs nothing.

### 6.5 Forced-TTY rendering — required

> **The highest-severity implementation trap in the project.**

Claude Code **captures** stdout rather than connecting it to the terminal, so
stdout is always a pipe. `termenv`'s default `ColorProfile()` calls
`isatty(stdout)`, sees a pipe, and returns `Ascii` — so a faithful implementation
of §6.3 resolves `truecolor`, hands hex values to Lipgloss, and emits **zero
escape sequences**. It presents as "colors just don't work" with no error.

**The fix below is not the one this section originally specified.** Rev 5 said:

```go
r := lipgloss.NewRenderer(os.Stdout,
    termenv.WithProfile(caps.Profile),   // ← silently ignored
    termenv.WithTTY(true),
)
```

Measured against lipgloss v1.1.0 at M2, `WithProfile` has **no effect**.
It configures the termenv `Output`; Lipgloss keeps its *own* profile field,
initialised lazily, and never reads the one on the `Output` it was handed. Every
profile therefore emitted 24-bit colour:

| profile passed | emitted | correct |
|---|---|---|
| `termenv.Ascii` | `\x1b[38;2;203;166;247m` | *(nothing)* |
| `termenv.ANSI` | `\x1b[38;2;203;166;247m` | `\x1b[95m` |
| `termenv.ANSI256` | `\x1b[38;2;203;166;247m` | `\x1b[38;5;183m` |
| `termenv.TrueColor` | `\x1b[38;2;203;166;247m` | correct, by accident |

That is worse than the bug this section exists to prevent. §6.3's entire
resolution would have been computed and discarded: `NO_COLOR=1` and `TERM=dumb`
would both have emitted colour, and a 16-colour terminal would have received
sequences it cannot render. The original wording passed three adversarial
reviews because it is the construction the termenv documentation implies.

The working form sets Lipgloss's own field explicitly:

```go
r := lipgloss.NewRenderer(w, termenv.WithTTY(true))
r.SetColorProfile(caps.Profile)          // ← the actual control
```

`SetColorProfile` is necessary and sufficient. `WithTTY(true)` is retained
because it disables the lazy `isatty()` detection rather than overriding its
result — it is the guard that stops a future refactor, or a Lipgloss release
that restores lazy initialisation, from silently reintroducing the original
trap on a pipe.

`lipgloss.ColorProfile()` and the package-level default renderer remain
**prohibited codebase-wide**. Two tests hold this in place, because the failure
is invisible in both directions: `TestColorSurvivesAPipe` (escapes are emitted
when stdout is not a terminal) and `TestProfileIsHonoured` (the emitted escapes
match the resolved profile, and `none` emits nothing).

---

## 7. Configuration

### 7.1 Resolution

**File selection:** `CC_STATUSLINE_CONFIG` if set, else
`$XDG_CONFIG_HOME/cc-statusline/config.toml`, defaulting to
`~/.config/cc-statusline/config.toml`.

**Overlay order** (later wins): embedded defaults → selected file →
`CC_STATUSLINE_*` env.

The repo's `config/` folder ships presets and the commented reference. It is not
read at runtime; `init` copies a preset into the XDG location, which keeps
`go install` working.

**The presets are embedded, which is what makes that sentence true.**
`go install …@latest` leaves no repository on disk, so `init` has nowhere to copy
a preset *from* unless the bytes are in the binary. `config/embed.go` declares
`package presets` beside the `.toml` files — `//go:embed` cannot reach across a
parent directory, which is why the Go file lives there rather than the presets
living under `internal/`. Nothing opens a file at runtime.

`default.toml` is documentation that happens to be executable: every value in it
is the embedded default, so installing it verbatim changes nothing. A test loads
it through the ordinary path and asserts it decodes to exactly `config.Defaults()`
with zero repairs — without which the file users read and the behaviour they get
would drift apart the first time a default moved.

**Invalid config behaves identically everywhere: silently default.** `render`,
`config`, `init`, and `doctor` all fall back to the embedded default for any
invalid key, and all record what was defaulted. `doctor` reports it and `render`
writes it to `last-error.txt`. Divergent behavior — erroring in one command and
defaulting in another — would let the wizard preview and the real status line
disagree, which breaks §10.3's central promise.

> **Corrected at M5: `render` rewrites that file, it does not append to it.**
> `render` runs on every refresh — every sixty seconds, in every session, for as
> long as Claude Code is open — and a config with one typo'd key produces the
> same note every time. Appending would write about fourteen hundred identical
> lines a day to diagnose a single misspelling. The file holds the current set
> and is deleted once the config loads clean, so a corrected typo stops being
> reported instead of accumulating into a history of solved problems.
>
> The write is best-effort in the strictest sense: every error from it is
> discarded, because §3.3's contract outranks any diagnostic. A file where the
> cache directory should be must not cost the user their status line.

**`doctor` exits 1 only for a config that could not be parsed at all.** An
unknown key or an out-of-range value is repaired and reported at exit 0 — those
are states the user can be told about and fix. A file that is not TOML is the one
case where what `doctor` reports and what `render` does could genuinely diverge.

### 7.2 Schema

```toml
# ~/.config/cc-statusline/config.toml

[general]
separator        = "auto"     # "auto" uses the icon set's glyph (§6.2)
powerline        = "auto"     # "auto" | true | false
icons            = "unicode"  # "ascii" | "unicode" | "nerdfont"
color            = "auto"     # "auto" | "none" | "16" | "256" | "truecolor"
max_width        = 0          # 0 = read COLUMNS
width_reserve    = 12         # cells reserved for Claude Code notifications
padding          = 0          # mirrors statusLine.padding, recorded by `init`
refresh_interval = 60         # seconds; written into settings.json by `init`
ambiguous_width  = "auto"     # "auto" | 1 | 2

[thresholds]
warning          = 70
danger           = 85
ratelimit_warn   = 80

[bar]
enabled          = true
width            = 10
filled           = "auto"
empty            = "auto"
gradient         = true

[git]
enabled          = true
branch_max_len   = 24

[context]
show_size        = "non_default"   # "non_default" | "always" | "never"

[colors]
model_marker     = "#cba6f7"
model_name       = "#89dceb"
normal           = "#4ade80"
warning          = "#facc15"
danger           = "#ef4444"
cost             = "#4ade80"
duration         = "#6c7086"
ratelimit        = "#6c7086"
effort           = "#94e2d5"
branch           = "#cba6f7"
added            = "#4ade80"
removed          = "#ef4444"
project          = "#89b4fa"
separator        = "#45475a"   # the line separator glyph
diffstat_delim   = "#45475a"   # the "/" inside +150/-30
bar_empty        = "#45475a"
gradient_stops   = ["#4ade80", "#facc15", "#fb923c", "#ef4444"]

# One entry per line. Inline tables are self-describing, so inserting a segment
# cannot misalign anything, and each line stays readable at a glance.
# `drop`: higher drops first when the line overflows; 99 never drops;
# omitted defaults to 50. Priorities are scoped to their own line.
[[line]]
segments = [
  {name="model", drop=99}, {name="context", drop=99}, {name="cost", drop=4},
  {name="duration", drop=5}, {name="ratelimits", drop=3},
]

[[line]]
segments = [
  {name="branch", drop=99}, {name="diffstat", drop=2}, {name="project", drop=1},
]

[segments.duration]
under_hour = "{m}m"
over_hour  = "{h}h{m}m"
over_day   = "{d}d{h}h"
pad        = false

[segments.ratelimits]
five_format  = "5h:{n}%"
seven_format = "7d:{n}%"
join         = " "

# Added after M8; see §5.3. `reset_format` is a Go time layout, not a {name}
# format — the two are separate keys because they are separate grammars. A
# `format` naming {reset} shows the clock; one that does not omits it, and no
# second key says so.
[segments.ratelimit_5h]
format       = "5h:{n}%"
reset_format = "15:04"

[segments.ratelimit_7d]
format       = "7d:{n}%"
reset_format = "2 Jan 15:04"

[segments.context]
format = "{bar} {n}%{warn}{size}"

[segments.diffstat]
format = "+{added}/-{removed}"

[segments.cost]
format = "${n}"

[segments.model]
format = "{marker} {name}"

# Added after M8; see §5.3. On neither shipped line — add it to a [[line]] to
# use it. The default spells its label out, which no other segment's does.
[segments.effort]
format = "Effort: {level}"

[segments.branch]
format = "{name}"

[segments.project]
format = "{name}"
```

The number of `[[line]]` blocks is the line count; `minimal.toml` ships one.
Order, presence, drop priority, glyphs, colors, thresholds, widths, and format
strings are reachable for every segment.

### 7.3 Environment overrides

See §6.1. Mapping: `ASCII`/`NERDFONT` → `general.icons`; `POWERLINE` →
`general.powerline`; `COLOR` → `general.color`; `NO_GIT` → `git.enabled`;
`CONFIG` → file selection; `NO_COLOR` → `general.color = "none"`.

**Where each is applied, and why it is not all one place.** `CONFIG` resolves in
`config.Path`. `NO_GIT` folds into `git.enabled` in `config.Load`, so nothing
downstream reads the variable — one switch, checked once, which is what keeps
M7's preview and the real render from disagreeing about it.

The other four resolve in `internal/style`, not in the config overlay, because
§6.3 defines an order that **interleaves** environment and configuration:
`NO_COLOR` and `TERM` outrank `CC_STATUSLINE_COLOR`, which outranks
`general.color`, which outranks `COLORTERM`. Folding the environment into the
config first would flatten that order — `general.color` would beat the
`COLORTERM` check specified to follow it. They resolve at the one point where the
whole order is visible in a single function.

---

## 8. Performance

### 8.1 Budget

The binding constraint is **"render never blocks and never forks."** As of rev 4
that is structural: there are no subprocesses on any path.

| Stage | Expectation |
|---|---|
| Go runtime init + package inits | ~2–4ms (floor, not controllable) |
| stdin read + JSON decode (~2KB) | < 0.2ms |
| Config load (TOML parse) | < 0.3ms |
| Git discovery (upward walk + `HEAD` read) | < 0.2ms |
| Segment render + fit | < 0.3ms |
| **p99 total** | **< 20ms** |

Measured from `execve` to process exit via `hyperfine --shell=none`; the shell
wrapper Claude Code interposes is excluded.

There is no cache, no lock, no GC, and no timeout context anywhere in the
project. Deferring the dirty flag (§3.2) removed all of them.

---

## 9. Testing

### 9.1 Payload fixtures

Git state is **injected**, not discovered, so goldens are hermetic. Each payload
fixture may have a sibling `<name>.git.json` loaded directly into `Context.Git`:

```json
{ "is_repo": true, "branch": "feat/auth" }
```

Without this, goldens would depend on the CI checkout's real branch.

They live in `internal/refstate/payloads` and are embedded rather than read from
disk, because §9.4's `preview` and §10.4's wizard consume them outside a test
binary. §4.2 has the reasoning. **Complete at M8.**

| Fixture | Exercises | Status |
|---|---|---|
| `startup.json` | zero cost, null percent, no rate limits, `is_repo: false` | M3 |
| `normal-42.json` / `warning-75.json` / `danger-92.json` | the three reference states | M3 |
| `long-model.json` | forces truncate and clip at width 40 | M3 |
| `empty.json` / `malformed.json` / `nulljson.json` / `zerobytes.json` | §3.3 degenerate inputs | M8 — **as a fuzz seed corpus, not as fixtures**; see below |
| `null-context.json` | a live session whose percentage went away; the discriminator against `startup`, which is 0% for the opposite reason | M8 |
| `no-ratelimits.json` / `seven-only.json` | subscriber-absence and independent-window absence | M8 |
| `no-git.json` / `detached.json` / `long-branch.json` | branch segment states | M8 |
| `500k-context.json` | the third size label. 200k is `normal-42`, 1M is `danger-92`; a fixture for each would be three copies of one rule | M8 |
| `fractional-pct.json` | tokens giving `p_exact = 69.6` → `p_shown = 70` → warning band, while fill rounds to 7 cells. The one fixture where §5.3's two percents diverge; built from tokens, since M0 measured `used_percentage` is never fractional. | M8 |
| `wide-cost.json` | `total_cost_usd = 107.43094200000006` — an observed value. Three digits widen line 1 past the reference states, and `FormatFloat(v,'f',2,64)` must absorb the float noise. | M8 |
| `sub-minute.json` | `59999` — one millisecond below §5.3's duration floor | M8 |
| ~~`1m-context.json`~~ / ~~`over-day.json`~~ | covered by `danger-92` and `long-model` respectively | not added |

> **Three of the degenerate four are not payloads.** `malformed`, `zerobytes`
> and `nulljson` describe *stdin*, not a Claude Code payload, and this package
> is embedded into the shipped binary so that `preview --state <name>` can put a
> fixture on a screen. Shipping invalid JSON inside the binary to satisfy the
> letter of this table would be the wrong reading of it. They are `f.Add` seeds
> on `FuzzRender` and rows in cmd's failure-contract table, which is where an
> assertion about stdin belongs.
>
> **A fixture is not coverage until something says what it is for.** A golden
> records what a fixture rendered. Edit the fixture, regenerate the golden, and
> the coverage evaporates with nothing red anywhere. `TestFixturesStillIsolate
> WhatTheyWereBuiltFor` names the rule each one exists for and fails citing it.

**The reference states are still four.** The M8 fixtures are deliberately not
`Reference`: §5.1 states acceptance criteria for four states and §9.4 asks a
human to compare four blocks against four mockups. Seventeen blocks is not a
longer gate, it is an unrun one. They are reachable individually with
`preview --state <name>`.

`gitinfo` is tested separately against synthetic `.git` trees in `t.TempDir()`:
directory form, file form with absolute `gitdir:`, file form with relative
`gitdir:`, detached HEAD, `rebase-merge/head-name`, bare repo, nested subdirectory
requiring the upward walk, and `GIT_CEILING_DIRECTORIES`.

### 9.2 Goldens — two tiers

Layout and color are stored separately, because layout does not change when a
color does. A one-hex theme edit previously rewrote ~300 golden files, producing
a diff nobody could review.

**Tier 1 — plain goldens (layout).**
`fixtures × {ascii, unicode, nerdfont} × {plain, powerline} × {40, 80, 120, 200}`,
storing `Rendered.Plain` only. These are the axes that actually vary in unstyled
output; the color axis is invisible here and is deliberately not part of this
tier. One `LANG=ja_JP.UTF-8` row validates the ambiguous-width path. One
Powerline row at width 40 covers **clip-mid-arrow**, the one case where stage-3
clipping is visually dangerous because it can leave a background active.

**Tier 2 — exact-escape table (color).** A table-driven test asserting the
**exact** escape sequence emitted for each `(profile, color)` pair:

```
truecolor  #4ade80  →  \x1b[38;2;73;222;128m     ← corrected at M8, twice over
256        #4ade80  →  \x1b[38;5;78m             ← corrected at M8; 114 was wrong
16         #4ade80  →  \x1b[92m                  ← as originally written
none       #4ade80  →  (empty)
```

A presence check (`contains \x1b[38;2;`) is insufficient: it cannot detect a
*wrong* downsample, which is exactly the §6.5 bug family. Editing a hex value
touches this table only.

> **Restructured after M8.** The table is keyed by colour *value* rather than by
> colour name. Ten of its twenty rows were duplicates — `cost`, `normal`,
> `added` and `gradient_stops[0]` are all `#4ade80` and each carried its own
> copy of the same three sequences. `escapeByHex` freezes what each distinct
> value emits; `defaultColors` freezes which value each key holds. The same
> eighty assertions run, a new segment reusing an existing colour needs no new
> row at all, and "is this frozen sequence still reachable" became a question
> with one answer.

> **Both corrections were found by writing the table, and neither by any test
> that existed before it.**
>
> The 256 row was arithmetic: palette 114 is `#87d787` (135,215,135), palette 78
> is `#5fd787` (95,215,135), and against (74,222,128) the squared distances are
> 3819 and 539. The document was wrong and the code was right.
>
> The truecolor row is the interesting one. `0x4a` is 74 and the terminal
> receives 73, because termenv's `RGBColor.Sequence` truncates `f.R*255` where
> go-colorful's `RGB255()` rounds — two components of the same dependency tree
> disagreeing by one unit in 255. It is pinned rather than fixed; `TODOS.md`
> has the reasoning. What matters here is that the table is the only instrument
> in the project that could have seen it.
>
> **The table is a freeze, not a derivation.** The values came from this code and
> were read back. Only the 16-colour row is corroborated by an independent
> source — this document, written before the code. What the freeze buys is that
> a termenv upgrade, a lipgloss upgrade re-breaking §6.5, or an edit to §7.2's
> defaults has to be looked at by a person instead of passing quietly.

**Tier 3 — a handful of styled end-to-end goldens**, one per reference state at
truecolor, proving the two tiers compose. Tier 1 removes the colour axis and
tier 2 removes the layout; between them sits what neither can see — *which
colour lands on which run of text*. A band on the wrong segment, a dropped
reset, the gradient painted over the empty cells: every one leaves both tiers
green. Stored Go-quoted, one line per line, because a changed colour in a raw
file is an invisible diff on a line the terminal repaints in the new colour.

`make golden` regenerates tiers 1 and 3; CI fails on drift.
`TestEveryStyledLineIsBalanced` additionally asserts what a golden cannot assert
about itself: that every line closes every colour it opens and ends reset. An
unterminated SGR is not cosmetic — the terminal has no concept of "end of status
line", so it bleeds into whatever Claude Code prints next and stays there.

### 9.3 Acceptance criteria

Every box below is ticked as of M8, with the test that ticks it named. A
criterion with no test named against it is a criterion nobody has checked, which
is the state this list was in until M8.

**Render**
- [x] All four reference states render byte-identical to their goldens.
      — `line.TestReferenceStates`, `line.TestPlainGoldens`
- [x] `{}`, `null`, malformed JSON, and zero bytes each render ≥1 line, exit 0.
      — `cmd.TestRenderHoldsTheFailureContract` (12 hostile inputs)
- [x] Every optional field, removed individually, renders without panic.
      — `cmd.TestEveryOptionalFieldRemovedIndividually`, generated across all
      17 fixtures rather than enumerated by hand
- [x] Fuzzed stdin (`go test -fuzz`) never produces a non-zero exit or empty output.
      — `cmd.FuzzRender`. 13.5M executions clean; seeds run every `make check`,
      the search runs in CI for 60s and on request via `make fuzz`
- [x] A panic injected mid-assembly yields the fallback line alone, with no
      partial output (proves `recover()` resets the buffer).
      — `cmd.TestRenderRecoversFromAPanic` (M1)
- [x] No line exceeds `available` at `COLUMNS` of 10 / 40 / 80 / 120 / 200,
      including `ambiguous_width = 2` with a 60-character model name.
      — `line.TestNoLineExceedsAvailable` (widths 10–200 × 3 icon sets × both
      ambiguous settings), and `line.TestSixtyCharacterModelNameUnderAmbiguousTwo`
      for the clause no fixture satisfies
- [x] `COLORTERM=truecolor ./cc-statusline render < fixture | cat` emits the
      exact expected escapes. The `| cat` is the point.
      — `cmd.TestExactEscapesSurviveARealPipe`, which runs the built binary with
      its stdout captured. `exec.Command` with a captured Stdout *is* the pipe;
      adding a literal `| cat` would test the shell
- [x] `NO_COLOR=1` emits zero ANSI escapes.
      — same test, plus `cmd.TestGateNoColorRendersLikePlain` and a CI step
- [x] p99 < 20ms over 100 consecutive renders on a 50k-file repo.
      — `cmd.TestRenderProcessBudget`: **p50 1.17ms, p99 1.30ms** idle on an
      i5-10400, and 4.16 / 9.07ms with all twelve threads saturated. The 50k
      half is `cmd.TestFileCountDoesNotMatter`, which asserts the stronger and
      truer claim
- [x] Render succeeds with `git` absent from `PATH`.
      — `cmd.TestRenderSucceedsWithGitAbsentFromPath`. Passes by construction
      today (§3.2 reads `.git/HEAD`, never execs), which is why it is written
      down: it is the tripwire on the dirty flag §13 defers
- [x] Render succeeds when `workspace.current_dir` no longer exists.
      — `cmd.TestRenderSucceedsWhenCurrentDirIsGone`

**Install** — integration tests with `t.TempDir()` as `HOME`, against a corpus in
`testdata/settings/`: with `//` comments, with `/* */`, with `//` inside a string
literal, a 17-digit integer, non-alphabetical key order, symlinked, read-only,
absent, empty, and with a pre-existing `statusLine`.
- [x] The command `init` writes executes under `env -i sh -c '<command>' < fixture`.
      — `cmd.TestTheInstalledCommandActuallyRuns`, added at M8. M5 asserted the
      *string* and never ran it; a quoting bug produces a string that compares
      equal to the expected value and fails the first time a user's path has a
      space in it. This builds under `…/Application Support/it's here/`, installs,
      reads the command back out of settings.json, and executes it
- [x] Non-alphabetical key order and a 17-digit integer survive byte-identical.
      — `settings.TestEveryOtherByteSurvives`, `cmd.TestInitPreservesEveryOtherByte`
- [x] A file with comments causes `init` to decline and print manual instructions.
      — `cmd.TestInitDeclinesOnComments`, `cmd.TestInitDeclinesRatherThanWriteIntoAComment`
- [x] `//` inside a string literal is not treated as a comment.
      — `settings.TestSlashesInStringsAreNotComments`, `cmd.TestInitLeavesSlashesInStringsAlone`
- [x] `init` run twice produces no second backup and no file modification.
      — `cmd.TestInitIsIdempotent`
- [x] A user-set `padding` is recorded into `[general] padding`, not clobbered.
      — `settings.TestPaddingIsCarriedNotClobbered`, `cmd.TestInitRecordsPaddingWithoutClobberingIt`
- [x] `uninstall` removes only the `statusLine` key and leaves every other byte,
      and every later user edit, intact.
      — `cmd.TestUninstallRemovesOnlyTheKey`, `cmd.TestUninstallRoundTripsToTheOriginal`

### 9.4 Manual visual gate

Goldens are byte-identical text, and the width harness measures with the same
go-runewidth the code uses. **The tests therefore validate the implementation
against its own assumption, and can never detect a real display-width violation
or a missing Nerd Font glyph.**

The procedure is in the README ("Does it look right in *your* terminal?"); the
instrument is `cc-statusline preview`. Screenshots go in `docs/gate/`. This also
closes §12 open question 1 (glyph availability) and 2 (gradient stops), which
cannot be settled any other way.

**The instrument, and why the gate needed one.** `preview` renders the §5.1
reference states — from `internal/refstate`, the same payloads the goldens
assert against — against capability sets the terminal running it does not have,
and draws under every line a **width rule**: `|---- 62 ----|`, exactly 62 cells
of pure ASCII. ASCII is East Asian Narrow, so the rule occupies 62 columns in
every terminal, every locale, and under every ambiguous-width setting. It is the
one line on screen this program cannot be wrong about, which turns "does it look
right" into "do these two lines end in the same column" — a question a
screenshot answers permanently.

Without it the gate was: hand-assemble twenty-odd `render` invocations with a
fixture on stdin and four environment variables in front of each, and judge
alignment by eye against nothing. That gate gets skipped, and when it is not
skipped it records an opinion.

> **The terminal list in rev 7 was wrong, and is corrected here (rev 8).** It
> named Kitty, iTerm2, Terminal.app, and **Windows Terminal** — and §13 defers
> Windows support entirely. Gating v1 on a platform v1 does not support is a
> spec error, not a missing screenshot. §10.1 ships `linux` and `darwin`, so the
> gate is:
>
> | | |
> |---|---|
> | **Linux** | two of {ghostty, alacritty, kitty} — whichever are installed |
> | **darwin** | iTerm2 and Terminal.app, before v0.1 is tagged |
> | ~~Windows Terminal~~ | out of scope until §13's Windows item lands |
>
> at 40, 120, and 200 columns, across ASCII / Unicode / Nerd Font / Powerline.
>
> **The locale axis was also overstated.** §6.4 resolves the ambiguous width by
> prefix-matching `LC_ALL` / `LC_CTYPE` / `LANG` against `zh` `ja` `ko`; it never
> calls `setlocale`, so `LANG=ja_JP.UTF-8` takes the CJK path whether or not the
> locale is generated. Setting the variable therefore proves nothing about the
> terminal, which decides ambiguous width from its own configuration in most
> emulators. The real question — *which behaviour does this terminal have* — is
> answered by rendering the same state under `--ambiguous 1` and `--ambiguous 2`
> and seeing which one lands on its rule.

---

## 10. Installation

### 10.1 Distribution

| Channel | Requires Go toolchain |
|---|---|
| `curl -fsSL .../install.sh \| sh` — fetches a release asset | **no** |
| GitHub Releases — static `linux/darwin × amd64/arm64` | **no** |
| `go install github.com/xqsit94/cc-statusline@latest` | yes |
| `./install.sh` from a clone | yes |

**Install prefix:** `$XDG_BIN_HOME`, else `~/.local/bin`, overridable with
`PREFIX`.

**Integrity verification.** `install.sh` downloads GoReleaser's `checksums.txt`
alongside the binary, verifies with `sha256sum -c`, and aborts loudly on
mismatch. The release tag is pinned rather than resolved as `latest` at install
time.

> **This is integrity, not authenticity.** `checksums.txt` ships from the same
> release as the binary, so it defends against a truncated or corrupted download,
> not against a compromised release. Signing (cosign or GitHub attestations) is
> the authenticity answer and is deferred to §13. Do not describe the current
> state as a security guarantee it is not.

### 10.2 `cc-statusline init`

1. Detect capabilities; set `general.icons` / `general.powerline` accordingly.
2. Write `~/.config/cc-statusline/config.toml` if absent. Never overwrite without
   `--force`.
3. Compute the desired value from the **fully resolved** config (§7.1), so a
   user-edited `general.refresh_interval` propagates on a later `init`:
   ```json
   { "type": "command", "command": "<absolute path> render", "refreshInterval": 60 }
   ```
   The path is the **absolute** `os.Executable()` result, never a bare name.

   > `~/.local/bin` is not on the default PATH on macOS or most Linux
   > distributions, and Claude Code runs the command through a **non-interactive**
   > shell that does not source `.bashrc` / `.zshrc`. A bare name would fail to
   > resolve, exit non-zero, and blank the status line **with no error message**.

   `padding` is not written; if present it is recorded into `[general] padding`.
4. **Plain-JSON check.** If `gjson.ValidBytes` rejects the file — comments, a
   trailing comma, anything JSON5 — print the exact block to add manually and
   exit 0 **without writing**. Refusing is the only non-destructive option.

   > **This step and the next were originally the other way round, and the order
   > was the bug.** Checking equality first means reading `statusLine` out of a
   > file that may not be plain JSON, and gjson finds keys inside comments (C-1).
   > A commented-out `statusLine` matching what we would write would be read as
   > an existing installation, and `init` would report success forever without
   > ever reaching this refusal. Corrected at M5; `TestInitDeclinesRatherThan`
   > `WriteIntoAComment` is what holds it.

5. If `statusLine` already equals the desired value, skip to step 7. Equality is
   compared on the decoded value, not the bytes: whitespace and key order in the
   user's file are not disagreements, and a byte comparison would make `init`
   non-idempotent the moment someone reformatted their settings.json. This is
   what §9.3's "run twice, no second backup" rests on.
6. Back up to `settings.json.bak-<timestamp>`, then write the key. Preserve file
   mode, resolve symlinks, write via temp+rename in the same directory.

   > **Adding the key is not `sjson`'s job; replacing it is.** sjson replaces an
   > existing value in place and preserves the layout around it. Appending a new
   > one, it writes `,"statusLine":{…}` immediately before the closing brace with
   > no indentation, so a hand-formatted settings.json comes back with a comma at
   > column zero and a doubled brace on the last line. That is valid JSON that
   > reads as damage, in a file people open by hand. M5 does the insert at the
   > indentation the file already uses and keeps sjson as the fallback for any
   > shape it does not recognise. Everything else in the document is still
   > untouched — §9.3's 17-digit integer is the assertion that keeps it so.
   >
   > sjson also drops trailing whitespace, so the file's final newline is
   > restored explicitly. One byte, and it is the byte that makes `git diff`
   > print "\ No newline at end of file" against a file we only added a key to.

7. Print a rendered preview and the `uninstall` command.

> **Why `refreshInterval: 60`.** Without a timer the duration segment freezes
> while you read code. At 10 seconds it would tick a visible digit in peripheral
> vision, which contradicts §1, and it would fire 8,640 times a day per session.
> Sixty seconds matches the duration segment's own minute granularity exactly, so
> every tick either changes the display or does nothing observable.

### 10.3 `cc-statusline uninstall`

Symmetric with install: `sjson.DeleteBytes(raw, "statusLine")`.

> **It does not restore a backup.** Backups are timestamped per `init`, so
> restoring the newest would revert *every* settings.json edit the user made since
> installing — permissions, hooks, env — not just the status line. Backups remain
> on disk as a manual escape hatch.

The config file is left in place.

### 10.4 `cc-statusline config`

The Bubble Tea wizard.

- Left pane: segment list, enable/disable, reorder with `j`/`k` and `J`/`K`, and
  per-line drop-priority editing (labeled per-line, since priorities are not
  comparable across lines).
- Right pane: **live preview**, re-rendered on every keystroke against the real
  captured payload *and captured environment* from
  `$XDG_CACHE_HOME/cc-statusline/last-payload.json`, falling back to a bundled
  fixture.
- Capability toggle row flipping the preview across icon sets, Powerline, and all
  four color profiles — the one control here that other status lines do not have.
- Width slider showing the drop → truncate → clip cascade live.
- A `Presets ›` button at the foot of the sidebar, opening §7.2's shipped
  presets as a list that previews each one before it is applied.
- `s` writes, `q` exits without saving, `r` resets.

The preview constructs a `Capabilities` struct (§6.4) and calls the same render
path, so what the wizard shows is what the status line prints.

> **Corrected at M7 — it does not construct a `Capabilities` struct.** It
> synthesises an *environment* and runs `style.Detect` on it (`style.Overlay`).
> Setting the resolved struct directly is shorter and forks §6.1's precedence —
> ASCII beating NERDFONT, Powerline refusing to turn on under ASCII, NO_COLOR
> beating everything — into a second implementation nothing tests. Going through
> the environment is what makes the sentence after it true rather than merely
> intended, and it is the same mechanism §9.4's `preview` already used.
>
> Three further things this section did not say, all settled when the wizard was
> built:
>
> - **What a save does to the file.** It patches it textually — the three
>   `[general]` keys through `ApplyOverrides`, the rows through `ReplaceLines` —
>   so every comment survives. §7.1 calls the config "documentation that happens
>   to be executable"; a wizard that marshalled a `Config` back out would strip
>   all of it, silently, on every use. Where it cannot regenerate the `[[line]]`
>   region without deleting something, it **refuses and writes nothing**, on the
>   same principle as §10.2 step 5.
> - **What it does not edit**: `[colors]`, `[segments.*]` formats, and
>   `[thresholds]`. A TUI picks colours worse than an editor does, the format
>   DSL is `{name}` substitution §13 already defers extending, and the
>   thresholds are the numbers C-4 exists to measure — a slider for an
>   uncalibrated guess invites tuning against it.
> - **The width slider is a lens, not a setting.** `s` never writes
>   `max_width`: it would pin the width for a terminal the user resizes, and
>   the question the slider answers is about drop priorities rather than width.

> **Added after M8 — the preset picker.** The sidebar's `Presets ›` button
> opens the shipped presets as a list, previewed through the same right pane and
> the same render path as an edit. Two decisions are load-bearing:
>
> - **A preset is a `Result`, not a `Config`.** It is previewed as, and applied
>   as, exactly the four things a save writes: the rows and the three `[general]`
>   capability keys. `minimal.toml` also sets `bar.width`, `git.branch_max_len`
>   and `context.show_size`; previewing those would show a status line that
>   pressing `enter` cannot produce, because §10.4's save has no way to write
>   them. Applying only what can be written keeps the sentence above — what the
>   wizard shows is what the status line prints — true of the picker too.
> - **Applying is an edit, not a save.** `enter` marks the model dirty and
>   writes nothing; `s` writes and `r` restores what was loaded. A preset that
>   wrote itself to disk on selection would be the only key in the wizard that
>   cannot be taken back, and it would be the destructive one.
>
> The presets reach the wizard already decoded, through `config.Decode` and
> `config.Validate` — the two steps a user's own file takes — because §10.4's
> package performs no I/O and knows no paths.

> **Added after M8 — `ctrl+s`, save and apply.** `s` writes config.toml, which
> the render path reads on every invocation. That is the whole of it *if the
> status line is installed*, and this section never asked what happens when it is
> not: someone who runs `cc-statusline config` before `cc-statusline init` builds
> a line, presses `s`, is told it was saved, and sees nothing — with no error
> anywhere, because nothing failed. `ctrl+s` closes that by running §10.2 steps
> 3-6 from inside the wizard.
>
> - **It is one function with `init`, not a second copy of it.** `cmd.install`
>   is what both call. "Installed" has to mean one thing in both places or they
>   differ in whichever detail the second copy forgot — the shell-quoted absolute
>   path, the `refreshInterval` read back from the *reloaded* config, the
>   `padding` carried through from settings.json, the backup taken before the
>   write rather than after. Every one of those is invisible in a struct
>   comparison and visible in the user's `git diff`, and none of them shows up as
>   anything but a status line that does not appear.
> - **It asks first, and `s` does not.** settings.json is not ours; it holds
>   permissions, hooks, environment and MCP servers, and the fact that exactly
>   one key in it is touched is a promise §10.2 makes rather than something the
>   user can see. config.toml is this program's own file and `r` puts back what
>   was loaded, so a prompt there would be a keystroke tax paid on every save to
>   warn about nothing.
> - **The order is save, then apply, and a failed save stops it.** Applying
>   installs a command that reads config.toml on every render, so applying after
>   a save that did not land would put the *previous* file into service under a
>   screen that says the new one is live.
> - **The prompt takes every key.** `n` is the preview payload's cycler
>   everywhere else in this wizard, and a y/n prompt is the one place `n` may not
>   mean something clever. Everything that is not `y` cancels.
>
> The refusal in §10.2 step 5 applies unchanged, and for the reason that section
> gives: on a file whose *comment* says exactly what we would have written, the
> idempotence check reads the comment, concludes the work is done and reports
> success forever. So the plain-JSON gate runs before the value is read here too.

> **Added after M8 — the hint block.** Heading the preview, four lines about the
> sidebar row the cursor is on: what it is, where it ends up on the line below,
> which keys apply to it, and which `[table] key`s in the file tune the parts
> the wizard does not edit.
>
> This section's bullets describe a pane that shows *what* the status line looks
> like. They do not answer the question a user actually arrives with, which is
> **why does my segment not appear**. That has two answers which are
> indistinguishable on screen and opposite in what they imply:
>
> - §4.3 removed it, because the payload carried nothing for it. No edit in this
>   wizard changes that; `n` shows it on a payload that does.
> - §5.6 stage 1 dropped it, because the row would not fit. That is the `drop`
>   the user chose, and `-` on this very row keeps it longer.
>
> Watching `duration` vanish as `<` narrows the preview, and watching it vanish
> as `n` cycles to the startup state, are the same experience without the block.
> The `here` line is the sentence that separates them, and it is the reason this
> exists at all — the rest is a description someone could have read in the
> README.
>
> - **It reads the render, it does not re-derive it.** `line.Trace` returns what
>   became of every configured entry, indexed the way the configuration is, by
>   reporting the items the fitter kept rather than by re-reading its three
>   stages. A second answer to "does this survive at this width" would eventually
>   disagree with the first, and a hint saying *dropped* about a segment visible
>   two lines below it is worse than no hint.
> - **One `line.Context` per frame.** The rendered rows, the cell counts beside
>   them, and the hint are all built from it, so they cannot come to describe
>   different renders.
> - **It has a frame of its own, drawn to fill the pane.** Everything else in
>   that pane is output; this is the one thing addressed to the reader, and a
>   box with a `>` in it is the shape they already read as such. A frame is also
>   the one thing here that cannot be approximately the right width — too wide
>   and it wraps into the next row — so `view` settles the pane's width before
>   it builds the pane's contents.
> - **It is pinned to one height, which is what lets it head the pane.** Its
>   rows wrap, and how many times depends on the segment — five lines for
>   `cost` at a hundred columns, seven for `context`. Unpinned and above the
>   preview, it would shove the rendered line, the cell counts and the rule two
>   rows up and down on every `j`, and those are precisely the numbers the eye
>   is comparing between keystrokes.
> - **It is the first thing dropped on a short or narrow terminal.** The help
>   line names every binding there is; the block explains one row. Where the
>   sidebar is the taller pane, which is most terminals, the block costs no rows
>   at all — it fills what was blank.
>
> The **`file`** line is the answer to a gap this section's own bullet leaves:
> §10.4 deliberately does not edit `[colors]` or `[thresholds]`, which leaves the
> wizard quietly implying those knobs do not exist. Naming the keys points at the
> file, where §7.1 wants the user to end up.

> **Added after M8 — the format ring, and `tab`.** Every segment ships a short
> list of alternative formats, and `tab` walks it with the result on screen.
> Mostly one axis: the same thing said compactly or said in words — `$0.85`
> against `Cost: $0.85`, `◆ Claude Opus 4.6` against `Model: Claude Opus 4.6`.
> The rate-limit windows cross that with a second, the clock, giving four:
>
> ```
> 5h:15%      5h:15% ↻ 23:10      Session: 15%      Session: 15% resets 23:10
> ```
>
> - **A variant is a value, not a setting.** Each entry is an assignment to
>   format keys §7.2 already has. Cycling writes `format = "Model: {name}"` into
>   the file, so the TOML still says what it does, `doctor` still validates it,
>   and someone who outgrows the ring is editing the same key the wizard was.
>   A `variant = "labelled"` key stored beside the format would have made §7.1's
>   "documentation that happens to be executable" into documentation of a lookup
>   table living in the binary. The render path learns nothing: `internal/line`
>   never reads `config.Variants`.
> - **Which is why this bullet list, and not four more segments.** The first
>   attempt made `5h:15% ↻ 23:10` a segment — see §5.3 for why being one buys
>   nothing that two windows do not already have.
> - **A hand-written format is kept in the ring, at the front.** Otherwise the
>   first press overwrites something the user typed, with quitting unsaved as the
>   only way back — and `tab` is exactly the key someone presses to find out what
>   it does. `shift+tab` reverses, because undoing one press of a four-entry ring
>   by making three more is not a cycle.
> - **A save writes only the keys that moved.** `minimal.toml` names no
>   `[segments.*]` table at all; writing every format key would append eleven
>   tables of defaults to a file whose whole point is that it does not name them.
>   A table that genuinely is absent gets appended rather than dropped in
>   silence — a save that reports success and does not write is the failure
>   §7.1's textual patching exists to avoid.
> - **`branch` has one entry and no ring.** What labels a branch is the glyph in
>   front of it, which the renderer prepends from the icon set and `i` cycles, so
>   `⎇ Branch: main` would be saying it twice. `tab` there says so rather than
>   doing nothing, which is the same argument the hint block's `keys` row is
>   built on.
> - **`effort` runs the axis backwards**, and is the only one that does: its
>   ring is `Effort: high` then `high`, labelled first. §5.3 gives the reason —
>   every other compact form carries a unit or a glyph that says what it is, and
>   a bare `high` carries neither.

---

## 12. Open questions

1. **Nerd Font glyph selection** — settled by the M4 visual gate, not on paper.
2. **Gradient stops** — same; needs a real terminal background.
3. **Optional segments** — which of `effort.level`, `fast_mode`,
   `thinking.enabled`, `pr.number`, `session_name` earn a segment post-v1?
   **`effort.level` does**, added after M8 and specified in §5.3; it is on
   neither shipped line, which is the shape the rest of this question should
   take too — a segment nobody has to accept is much cheaper to be wrong about
   than a row of the default. The other four are still open.
4. **Windows** — git discovery and XDG paths need a story if published broadly.
5. **Single-line default?** `minimal.toml` ships but is not the default. Two lines
   costs two terminal rows on every prompt forever, and some users reject that on
   sight. Revisit at M6 with real feedback.

---

## 13. Deferred (explicitly out of scope for v1)

- **The git dirty flag** and everything it required: subprocess spawning, the
  cache, atomic writes, GC, timeout contexts, and the lock protocol. Cut because
  it answers none of §1's four questions and cost the largest subsystem in the
  document to render one asterisk. Revisit in v0.2 if a week of real use says the
  asterisk was load-bearing.
- Release signing (cosign / GitHub attestations) for authenticity beyond §10.1's
  integrity check.
- A Starship-style format-string DSL.
- Segments for the §3.1 "available but not rendered" fields.
- Transcript parsing and `/api/oauth/usage` querying.
- Windows support.
- ~~`-tags minimal` build excluding Bubble Tea. Only if measurement shows package
  init is material against §8.1.~~ **Measured at M7: it is not.** Bubble Tea
  costs 0.07ms of package init and 160KB of binary — under half a percent of
  §8.1's 20ms p99, and `render` never touches it. Stays deferred, now on
  evidence.
- A `ccstatusline` config importer, if adoption ever justifies it.
