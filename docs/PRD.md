# cc-statusline — Product Requirements Document

| | |
|---|---|
| **Status** | DRAFT (rev 6, post two adversarial rounds + one engineering review + outside voice + M0 measurement + M2 correction) |
| **Version** | 0.1.0 |
| **Date** | 2026-08-05 |
| **Owner** | xqsit94 |
| **Module** | `github.com/xqsit94/cc-statusline` |
| **Language** | Go 1.26+ |
| **Requires** | Claude Code ≥ 2.1.153 |
| **Origin** | `/office-hours` → `/plan-eng-review`, 2026-08-05 |

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

## 2. Background and competitive context

There are 80+ published Claude Code status line projects. The field is mature and
the differentiator is not feature count.

| Project | Stack | Position |
|---|---|---|
| ccstatusline | TypeScript | De-facto framework, broadest widget system, TUI config |
| claude-powerline | TypeScript | Best visual taste, plugin-native install |
| CCometixLine | Rust | Speed, built-in themes, TUI config |
| cship | Rust | Reuses Starship's TOML mental model |
| claudeline | Go | Intentionally constrained |
| felipeelias/claude-statusline | Go + TOML | Preset engine, Starship-inspired |

Recurring ecosystem failure modes, each mapped to a requirement:

| Ecosystem failure mode | Requirement |
|---|---|
| `npx @latest` in the hot render path | §4.1 — single static binary, no runtime resolution |
| Transcript-parsing brittleness | §3.2 — payload-first, transcript parsing prohibited |
| Multiline collapse on terminal resize | §5.6 — width-aware fitting with a hard terminal clip |
| Nerd Font inconsistency across emulators | §6 — capability matrix, plus a manual visual gate (§9.4) |
| Missing payload fields forcing heuristics | §3.1 — every displayed value has a named payload source |

### 2.1 Positioning — honest version

The wizard (§10.3) is **not** unique; ccstatusline and CCometixLine both ship TUI
config. What is unique is the **degradation preview**: flipping the live preview
across ASCII / Nerd Font / Powerline and all four color profiles before you
commit to a theme.

That is a narrow differentiator, and it is a control a user touches once. The
honest v1 bet is therefore not the wizard. It is:

> **Works correctly with zero configuration, installs without a toolchain, and
> never blocks the terminal.**

The wizard is sequenced last (M7) precisely so that it remains a decision made
*after* someone has used v1, not a bet placed before.

### 2.2 Design thesis

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

**Path resolution input** (read, never displayed): `workspace.current_dir` drives
git discovery. See §5.8.

**Available but not rendered.** Fields with no v1 segment; adding them is post-v1
work tracked in §13: `cwd`, `workspace.added_dirs`, `workspace.repo.*`,
`workspace.git_worktree`, `worktree.*`, `session_name`, `version`,
`output_style.name`, `effort.level`, `thinking.enabled`, `fast_mode`, `vim.mode`,
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
   number, not 100, is the scale §5.4 must derive from. Tracked in §14.1 as C-4.
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
spike stays installed until §14.1 C-4 closes.

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

**`capture` records the environment, not just the payload.** It writes the raw
stdin JSON plus `COLUMNS`, `LINES`, `TERM`, `COLORTERM`, `LANG`, `LC_CTYPE`, and
`NO_COLOR` into a sidecar. Without those, §6's capability model is unverifiable
and §10.3's wizard cannot reproduce the real render environment. It defaults to
`$XDG_CACHE_HOME/cc-statusline/last-payload.json` and always writes that path in
addition to any explicit file. A `capture` write failure never affects the render
or the exit code.

### 4.2 Package layout

Five internal packages. Packages that always change together are one package.

```
cc-statusline/
├── main.go                     # arg dispatch + top-level recover, no logic
├── go.mod
├── cmd/
│   ├── render.go   config.go   init.go   uninstall.go
│   ├── capture.go  doctor.go   version.go
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
│   └── gitinfo/
│       └── discover.go         # upward walk, gitdir indirection, HEAD parse
├── config/
│   ├── config.example.toml
│   └── presets/{default,minimal}.toml
├── testdata/
│   ├── payloads/*.json         # + *.git.json sidecars, see §9.1
│   ├── settings/*.json         # settings.json corpus, see §9.3
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

Byte-identical acceptance criteria for the default preset.

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
> defaults cannot be locked until the compaction point is measured — C-4, §14.1.

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

> **Ambiguous-width glyphs.** `▓` `░` `◆` `⚠` are East Asian Ambiguous and render
> double-width in many terminals under a CJK locale. `runewidth.EastAsianWidth` is
> set from `LC_CTYPE` / `LANG`, and `[general] ambiguous_width = "auto" | 1 | 2`
> is an explicit escape hatch. See §9.4 — this cannot be validated by goldens.

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
      │  TRUNCATE   │  branch floors at 8 cells, model at 10
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
| `segments.diffstat.format` | `{added}` `{removed}` |
| `segments.cost.format` | `{n}` |
| `segments.context.format` | `{bar}` `{n}` `{warn}` `{size}` |
| `segments.model.format` | `{name}` `{marker}` |
| `segments.branch.format` | `{name}` |
| `segments.project.format` | `{name}` |

This table is defined **once**, in `config/validate.go`, and consumed by both the
validator and every segment's renderer. Duplicating it is the one repetition that
would silently drift: validation would pass while render emitted nothing. A test
asserts every key in the table is reachable from its segment.

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

| Role | ASCII | Unicode | Nerd Font |
|---|---|---|---|
| model marker | `*` | `◆` | `󰚩` |
| bar filled | `#` | `▓` | `▓` |
| bar empty | `-` | `░` | `░` |
| separator | `\|` | `│` | `│` |
| powerline sep | n/a | n/a | `` |
| branch | `>` | `⎇` | `` |
| danger | `!` | `⚠` | `` |
| ellipsis | `.` | `…` | `…` |

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

**Invalid config behaves identically everywhere: silently default.** `render`,
`config`, `init`, and `doctor` all fall back to the embedded default for any
invalid key, and all record what was defaulted. `doctor` reports it and `render`
appends it to `last-error.txt`. Divergent behavior — erroring in one command and
defaulting in another — would let the wizard preview and the real status line
disagree, which breaks §10.3's central promise.

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

[segments.context]
format = "{bar} {n}%{warn}{size}"

[segments.diffstat]
format = "+{added}/-{removed}"

[segments.cost]
format = "${n}"

[segments.model]
format = "{marker} {name}"

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

| Fixture | Exercises |
|---|---|
| `startup.json` | zero cost, null percent, no rate limits, `is_repo: false` |
| `normal-42.json` / `warning-75.json` / `danger-92.json` | the three reference states |
| `empty.json` / `malformed.json` / `nulljson.json` / `zerobytes.json` | §3.3 degenerate inputs |
| `null-context.json` | `current_usage: null` (post-`/compact`) |
| `no-ratelimits.json` / `seven-only.json` | subscriber-absence and independent-window absence |
| `no-git.json` / `detached.json` / `long-branch.json` | branch segment states |
| `long-model.json` | forces truncate and clip at width 40 |
| `500k-context.json` / `1m-context.json` | size label rules |
| `fractional-pct.json` | tokens giving `p_exact = 69.6` → `p_shown = 70` → warning band, while fill rounds to 7 cells. The one fixture where §5.3's two percents diverge; built from tokens, since M0 measured `used_percentage` is never fractional. |
| `wide-cost.json` | `total_cost_usd = 107.43094200000006` — an observed value. Three digits widen line 1 past the reference states, and `FormatFloat(v,'f',2,64)` must absorb the float noise. |
| `sub-minute.json` / `over-day.json` | duration boundaries |

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
truecolor  #4ade80  →  \x1b[38;2;74;222;128m
256        #4ade80  →  \x1b[38;5;114m
16         #4ade80  →  \x1b[92m
none       #4ade80  →  (empty)
```

A presence check (`contains \x1b[38;2;`) is insufficient: it cannot detect a
*wrong* downsample, which is exactly the §6.5 bug family. Editing a hex value
touches this table only.

**Tier 3 — a handful of styled end-to-end goldens**, one per reference state at
truecolor, proving the two tiers compose.

`make golden` regenerates tiers 1 and 3; CI fails on drift.

### 9.3 Acceptance criteria

**Render**
- [ ] All four reference states render byte-identical to their goldens.
- [ ] `{}`, `null`, malformed JSON, and zero bytes each render ≥1 line, exit 0.
- [ ] Every optional field, removed individually, renders without panic.
- [ ] Fuzzed stdin (`go test -fuzz`) never produces a non-zero exit or empty output.
- [ ] A panic injected mid-assembly yields the fallback line alone, with no
      partial output (proves `recover()` resets the buffer).
- [ ] No line exceeds `available` at `COLUMNS` of 10 / 40 / 80 / 120 / 200,
      including `ambiguous_width = 2` with a 60-character model name.
- [ ] `COLORTERM=truecolor ./cc-statusline render < fixture | cat` emits the
      exact expected escapes. The `| cat` is the point.
- [ ] `NO_COLOR=1` emits zero ANSI escapes.
- [ ] p99 < 20ms over 100 consecutive renders on a 50k-file repo.
- [ ] Render succeeds with `git` absent from `PATH`.
- [ ] Render succeeds when `workspace.current_dir` no longer exists.

**Install** — integration tests with `t.TempDir()` as `HOME`, against a corpus in
`testdata/settings/`: with `//` comments, with `/* */`, with `//` inside a string
literal, a 17-digit integer, non-alphabetical key order, symlinked, read-only,
absent, empty, and with a pre-existing `statusLine`.
- [ ] The command `init` writes executes under `env -i sh -c '<command>' < fixture`.
- [ ] Non-alphabetical key order and a 17-digit integer survive byte-identical.
- [ ] A file with comments causes `init` to decline and print manual instructions.
- [ ] `//` inside a string literal is not treated as a comment.
- [ ] `init` run twice produces no second backup and no file modification.
- [ ] A user-set `padding` is recorded into `[general] padding`, not clobbered.
- [ ] `uninstall` removes only the `statusLine` key and leaves every other byte,
      and every later user edit, intact.

### 9.4 Manual visual gate — before M3 freezes goldens

Goldens are byte-identical text, and the width harness measures with the same
go-runewidth the code uses. **The tests therefore validate the implementation
against its own assumption, and can never detect a real display-width violation
or a missing Nerd Font glyph.**

One human gate, recorded in the PR:

- Kitty, iTerm2, Terminal.app, and Windows Terminal
- at 40 and 200 columns
- with `LANG=en_US.UTF-8` and `LANG=ja_JP.UTF-8`
- across ASCII, Unicode, Nerd Font, and Powerline

Screenshots attached. This also closes §12 open question 1 (glyph availability)
and 2 (gradient stops), which cannot be settled any other way.

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
4. Read `~/.claude/settings.json`. If `statusLine` already equals the desired
   value, skip to step 7. This is what makes `init` idempotent.
5. **Comment check.** If the file contains `//` or `/* */` outside a string
   literal, print the exact JSON block to add manually and exit 0 **without
   writing**. Neither `gjson` nor `sjson` understands comments; their scanners
   walk raw bytes and would mis-locate the edit. Refusing is the only
   non-destructive option. **Verify empirically before M6.**
6. Back up to `settings.json.bak-<timestamp>`, then
   `sjson.SetBytes(raw, "statusLine", desired)`, validated first with `gjson`.
   Preserve file mode, resolve symlinks, write via temp+rename in the same
   directory.
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
  four color profiles — the differentiator per §2.1.
- Width slider showing the drop → truncate → clip cascade live.
- `s` writes, `q` exits without saving, `r` resets.

The preview constructs a `Capabilities` struct (§6.4) and calls the same render
path, so what the wizard shows is what the status line prints.

---

## 11. Milestones

Re-sequenced in rev 4: git discovery moved into M2 (three of four reference states
show a branch, so M3's goldens depended on it), config moved into M3 (the fitter
is driven by config concepts), and the README moved to M6 with the first release.

| Phase | Scope | Exit criterion |
|---|---|---|
| ~~**M0 Spike**~~ **DONE** | `capture` + `report` in `internal/spike`. 35 payloads. | ✅ §3.1.1 answered. Residual: C-4 (compaction point), C-5 (200k + startup) |
| ~~**M1 Skeleton**~~ **DONE** | Module, payload structs, `(value, ok)` accessors, key diff, buffered output + recover contract, `render` / `capture` / `version` | ✅ 12 hostile inputs render a fallback line and exit 0; the recover is exercised by an injected panic |
| ~~**M2 Render core**~~ **DONE** | Segment interface, 8 segments, `gitinfo` HEAD reader, plain joining, `Capabilities` struct, **§6.5 forced-TTY renderer** | ✅ All four §5.1 states byte-identical; colour survives a pipe at every profile |
| **M3 Config + polish** | TOML schema, XDG resolution, env overlay, validation, 2 presets, gradient, glyph sets, powerline, go-runewidth, drop→truncate→clip | All four reference states match plain goldens |
| **M4 Visual gate** | §9.4 manual pass across 4 terminals × 2 locales × 4 capability sets | Screenshots in the PR; glyphs and stops locked |
| **M5 Install** | `init`, `uninstall`, `doctor`, `version`, sjson + comment refusal, GoReleaser, release workflow, `install.sh` + checksums, **README** | Clean machine, no Go, installs via curl and passes `env -i` |
| **M6 Release v0.1** | Tag, publish, use it yourself for a week | One non-you user has it installed |
| **M7 Wizard** | Bubble Tea `config`, live preview, capability toggles, width slider | Full configuration without editing TOML |
| **M8 Hardening** | Full golden tiers, escape table, install integration corpus, fuzz | All §9.3 criteria pass |

**M0 is done** (2026-08-05). It took longer than ten minutes and was worth every
one: it corrected four factual claims in §3, and the analysis method itself had
to be rewritten once. Two residuals (C-4, C-5) need real sessions rather than
more code, so the spike stays installed while M1 proceeds.

**M6 is a real gate, not a formality.** The wizard at M7 is the largest remaining
investment and its value is unproven. Shipping v0.1 first makes M7 a decision
informed by use rather than a bet placed before it.

---

## 12. Open questions

1. **Nerd Font glyph selection** — settled by the M4 visual gate, not on paper.
2. **Gradient stops** — same; needs a real terminal background.
3. **Optional segments** — which of `effort.level`, `fast_mode`,
   `thinking.enabled`, `pr.number`, `session_name` earn a segment post-v1?
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
- `-tags minimal` build excluding Bubble Tea. Only if measurement shows package
  init is material against §8.1.
- A `ccstatusline` config importer, if adoption ever justifies it.

---

## 14. Review history

Two adversarial rounds (7/10, then 8/10), one engineering review, one outside
voice. Rev 4 folds all of it. Rev 5 folds M0's measurements.

**M0 (rev 5)** replaced §3.1.1's questions with answers. The headline finding is
that `used_percentage` measures the **raw** window, which leaves §5.4's
thresholds miscalibrated until the compaction point is measured (C-4).

Two document changes followed: `resets_at` is an epoch number rather than a
string (§3.1), and the percent now has a single definition in §5.3 — `p_exact`
for fill and ramp, `p_shown` for the number and the bands — because §5.5 and
§5.7 each said `pct` without either saying where it came from. Two further
"corrections" were claimed and then withdrawn on checking; §3.1.1 records which
and why.

**M2 (rev 6)** found that §6.5's prescribed fix does not work. `termenv.WithProfile`
is silently ignored by Lipgloss, so the profile §6.3 resolves was discarded and
every terminal received 24-bit colour — meaning `NO_COLOR` and `TERM=dumb` would
both have emitted colour. §6.5 now specifies `SetColorProfile` and shows the
measured table. The section had survived three adversarial reviews because the
broken form is the construction termenv's own documentation implies; only
running it against all four profiles exposed it.

Two smaller corrections came with it. Powerline is now suppressed under ASCII
regardless of configuration, since honouring it would emit U+E0B0 into a
terminal the user has just declared incapable of Unicode. And `git.branch_max_len`
truncation moved out of `gitinfo` into the branch segment, because the ellipsis
is a glyph — `.` under ASCII — and a package that reads files has no business
knowing that.

The analysis itself had to be rewritten mid-flight. An implied-denominator
method that assumed an exact percentage read wild inconsistency into data that
was in fact perfectly regular, because the value is rounded. Rounded metrics
constrain a range, not a point — and a method that reports noise as a finding is
worse than no method, because it looks like an answer.

**Rounds 1–2** fixed: the forced-TTY color trap (§6.5), the unsatisfiable bar-fill
triple, cross-line drop ordering, non-hermetic git fixtures, the debounce
misreading, the exit-code contract, `TIOCGWINSZ` under output capture, East Asian
ambiguous widths, the missing `.git` upward walk, `encoding/json` reformatting
settings.json, EXDEV on cache rename, a position-relative gradient carrying no
information, and the absolute-path install failure.

**Engineering review (rev 4)** decided:

| Finding | Resolution |
|---|---|
| 9 packages for a two-line program | Collapsed to 5 (§4.2) |
| Hand-rolled `O_EXCL` lock reinventing `flock` | Moot — the whole subsystem is deferred |
| `install.sh` executes an unverified binary | `checksums.txt` verification (§10.1) |
| Idle sessions forked `git status` twice a minute | Moot — no subprocesses remain |
| `recover()` cannot meet the exit-0 contract | Buffered write, no goroutines, buffer reset (§3.3) |
| 30 lines of TOML for 9 segments | Inline tables (§7.2) |
| `init` mutates settings.json with no tests | Integration corpus (§9.3) |
| One hex edit rewrote 300 goldens | Two-tier goldens (§9.2) |

**Outside voice (rev 4)** found what three passes missed:

| Finding | Resolution |
|---|---|
| **The default preset could not render §5.1** — bar/percent joined by a space, but only one global separator exists | Fused into one `context` segment (§5.3) |
| The dirty flag's subsystem served none of §1's four questions | Deferred to v0.2 (§3.2, §13) |
| "Concurrent refreshes are idempotent" was time-inverted and wrong | Moot, but the reasoning error is recorded here |
| Plain goldens cannot cover the color axis | Two-tier split with an exact-escape table (§9.2) |
| Drop priorities killed rate limits before the redundant bar | Reordered to match §1 (§5.6) |
| The data contract had never been run once | M0 spike, now gating everything (§3.1.1) |
| `used_percentage` may measure the raw window, not net of autocompact | Named as a measurement task; §5.4 defaults marked provisional |
| Width tests validate the implementation against itself | Manual visual gate (§9.4) |
| Capability resolution read env, but the wizard needs it parameterized | `Capabilities` struct (§6.4) |
| `uninstall` restoring a backup destroys later user edits | `sjson.DeleteBytes` (§10.3) |
| `render` had no diagnostic channel at all | `last-error.txt` + `doctor` (§3.3) |
| `refreshInterval: 10` put a ticking digit in peripheral vision | 60s + minute granularity (§10.2, §5.7) |
| Stdin sniffing misfires under cron, CI, and `env -i` | `render` is required (§4.1) |
| A template engine for zero demand | Bare `{name}` only (§5.7) |
| Powerline × clip was untested by construction | Added to tier 1 (§9.2) |
| README shipped after the product | Moved to M5 (§11) |
| The wizard is not actually unique | §2.1 restates the positioning honestly |

**Deliberately rejected.** Replacing the fill-relative gradient with a solid
`ramp(p_exact/100)` — simpler and edge-case-free, but it discards the multi-hue bar
the reference mockups specify. §5.5 records the trade.

### 14.1 Reviewer Concerns (open)

**C-1 — sjson comment handling is unverified (§10.2 step 5, M5).** The claim that
neither `gjson` nor `sjson` tolerates comments is consistent with how their
scanners work but has not been tested. Write the ten-line test before writing the
installer; if they handle it, delete the refusal path.

**C-2 — the fill-relative gradient's information density (§5.5).** The leftmost
lit cell stays near the ramp start at every level, so much of the bar's ink does
not move as the session fills. Accepted to match the mockups. Settle it at the M4
visual gate: if it reads as "same rainbow, different length," switch to solid
`ramp(p_exact/100)`.

**C-3 — two lines may be the wrong default (§12 Q5).** Not resolved. Revisit at
M6 with real feedback.

**C-4 — the compaction point is unmeasured (§3.1.1, §5.4).** M0 answered what
`used_percentage` divides by; it did not answer what value compaction fires at.
Until it does, `warning = 70` and `danger = 85` are numbers chosen against a
100% ceiling the metric may never reach. The spike stays installed: run a
session into a real compaction, then `cc-statusline report`. Blocks locking
§5.4, not M1 or M2.

**C-5 — the 200k window and the null-percentage startup state are unobserved
(§3.1.1, §5.1).** Every M0 payload came from a 1M-token session already in
progress. The startup reference state and the `[1M]` size marker rest on the
docs, not on measurement. Cheapest fix: one fresh session on a 200k model with
the spike installed.

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | CLEAR | 9 issues, 0 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | — |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |
| Outside Voice | fallback subagent | Independent plan challenge | 1 | issues_found | 30 findings, 3 cross-model tensions |

**CROSS-MODEL:** The outside voice overturned two decisions this review had
already made — the lock-deletion rationale (time-inverted writes, not idempotent)
and the golden split (color is invisible in unstyled output). Both were corrected.
It also found the one defect three prior passes missed: the default preset could
not render its own reference states, because bar and percent are joined by a space
while only one global separator exists. Its strongest structural argument, cutting
the git dirty flag, removed the largest subsystem in the document.

**VERDICT:** ENG CLEARED — ready to implement. Start at M0; it takes ten minutes
and everything downstream depends on it.

NO UNRESOLVED DECISIONS
