# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Claude Code status line: one static Go binary that reads a JSON payload on
stdin and prints one or two lines. No Node, no Python, no subprocesses, no
network — in the render path. `update` is the one command that leaves it, to
fetch a release; every other subcommand touches only the filesystem. Go 1.26+.

## Commands

```sh
make check          # gofmt -l, go vet, go test ./... — run before every commit
make build          # -> bin/cc-statusline, with the same ldflags as a release
make install        # -> $PREFIX (default ~/.local/bin)
```

Running one test, or one package:

```sh
go test ./internal/line/ -run TestNoLineExceedsAvailable -count=1 -v
go test ./internal/wizard/ -count=1
```

`-count=1` matters: without it Go serves cached results and a "passing" run may
have executed nothing.

The specialised targets:

| target | what it does |
|---|---|
| `make gate` | PRD §9.4's **manual** visual gate — renders the matrix to a screen for a human to look at. No test substitutes for it: goldens measure with the same go-runewidth the renderer uses, so they prove self-consistency, never that your terminal agrees. Checklist in the README. |
| `make gate-check` | The half a machine can answer (ASCII purity, `NO_COLOR` ≡ `--plain`, the ambiguous-width flag). Ordinary tests, so `make check` already runs them. |
| `make golden` | Regenerates §9.2 tiers 1 and 3. **Read the diff before committing it** — a golden regenerated to make a red test green is a bug written down and agreed to. Tier 2 (the escape table) is hand-written on purpose. |
| `make fuzz` | `FuzzRender` over stdin. The seed corpus runs on every `make check`; this is the search. `FUZZTIME=10m` for an overnight run. |
| `make bench` | §8.1's in-process budget. Fails when `-bench` matches nothing, because `go test -bench` reports PASS on an empty run. |
| `make p99` | §8.1's real criterion (p99 < 20ms execve→exit), plus the 50,000-file repo case. Uses hyperfine when installed. |
| `make probe` | C-7: measure `width_reserve` against Claude Code's own rendering. Needs a `settings.json` swap; procedure in the README. |

## Architecture

`main` is at the module root, not under `cmd/`, because
`go install github.com/xqsit94/cc-statusline@latest` only resolves for a main
package at the module root. Arg dispatch and the outermost recover live in
`main.go`; `cmd.Main` routes to one function per subcommand (`render`,
`capture`, `preview`, `init`, `config`, `uninstall`, `doctor`, `version`,
`update`, `spike`).

### The render path

```
stdin JSON → payload.Parse → style.Detect(env, cfg) → line.Render(Context) → fitter → stdout
```

- **`internal/payload`** — the stdin contract (§3.1). Every field is optional;
  absent is normal, not an error.
- **`internal/style`** — resolves what the terminal can do. Pure functions of an
  environment map plus a `*config.Config`; nothing here reads the real
  environment or a terminal.
- **`internal/line`** — segments, the joiner, and the fitter. `Render` returns
  one string per non-empty row. `segments.go` holds §5.3's original eight plus
  `effort`; `ratewindow.go` holds the two single-window rate-limit ones added
  after M8, which are one parameterised type rather than two near-identical
  renderers. All three post-M8 segments are on no shipped line: a segment nobody
  has to accept is cheaper to be wrong about than a row of the default.
- **`internal/config`** — the TOML schema, the loader, and the textual editor.
  `schema.go` is the segment registry: every segment declared once, with its
  keys, its placeholders and their types, its colours and its `tab` ring.
  `registry.go` is the vocabulary that is written in and `derive.go` reads it
  back out into `SegmentNames`, `FormatKeys`, `TimeKeys`, `ColorKeys`,
  `Variants` and half of `Defaults()`. **Adding a segment is `CONTRIBUTING.md`**,
  not a hunt: three files, or six if it needs a new colour, new payload data
  and a documented default.
- **`internal/wizard`** — the Bubble Tea TUI behind `cc-statusline config`.
  `hint.go` is the box heading the preview: what the cursor is on, and what
  became of it in the render directly below (`line.Trace`). Three modes: the
  segment list, the preset picker, and the `ctrl+s` confirmation — the first two
  are which of two things the left pane is, the third leaves both panes alone
  and takes the keyboard.
- **`config/`** (package `presets`) — the shipped `.toml` files, `//go:embed`-ed
  so `go install` (which leaves no repo on disk) can still write one.

### Invariants that are easy to break

These are load-bearing and each has tests defending it. Breaking one usually
still compiles.

- **Segments perform no I/O, and resolve none of their own inputs.** Everything
  arrives already resolved in `line.Context` — including `Zone`, which the
  rate-limit reset segments format in. A segment that could read a file could
  block the render; a segment that reads `time.Local` for itself is an input no
  test and no preview can vary, and a golden that read the machine's zone would
  pass on one laptop and fail in CI with nothing in the diff to say why.
- **`internal/wizard` performs no I/O** — no files, no paths, no environment.
  It is handed a `State` and returns a `Result`; `cmd/config.go` owns every
  side effect, including the save and `ctrl+s`'s install, both of which arrive
  as injected functions. That is what lets a Bubble Tea `Update` be driven by
  tests at full speed with no terminal attached. `Apply` carries a `Target`
  string beside its `Do` because the confirmation has to name the file before it
  is touched, and this package builds no paths.

- **`init` and the wizard's `ctrl+s` install through one function** (`cmd.install`,
  §10.2 steps 3-6). A second copy would differ in whichever detail it forgot —
  the shell-quoted absolute path, the `refreshInterval` read back from the
  *reloaded* config, the `padding` carried through from settings.json, the backup
  taken before the write rather than after — and every one of those is invisible
  in a struct comparison and shows up only as a status line that does not appear.
  `TestApplyInstallsExactlyWhatInitInstalls` compares the two routes' output byte
  for byte on a machine where none of those inputs is at its default.
- **Width is measured from `Rendered.Plain`, never from `Styled`.** Stripping
  ANSI at measure time makes every width calculation depend on a regex keeping
  pace with every escape a terminal understands.
- **The wizard builds one `line.Context` per frame** (`previewContext`). The
  rendered rows, the width budget the rule under them draws, and the hint
  block's account of what became of each segment all read it. A second Overlay
  is all it takes to make the pane describe two different renders at once, and
  the copy that used to sit behind the per-row `N cells` counts had already
  drifted onto the wrong environment for a source carrying its own. Those counts
  are gone; the rule that produced them is why there is one Context here.
- **`line.Trace` reports the fitter's decisions; it does not reproduce them.**
  `fit` hands back the items it kept and `Trace` reads the difference against
  the ones it was offered. Re-deriving "does this segment survive at this width"
  gives a second answer that will eventually differ from the screen — most
  likely where a segment *refuses* to truncate, which is the segment's own
  decision and not one a caller can reproduce.
- **Capabilities are resolved by synthesising an environment and running
  `style.Detect` on it** (`style.Overlay`), never by constructing a
  `Capabilities` struct directly. Setting the struct forks §6.1's precedence —
  ASCII beating NERDFONT, Powerline refusing to turn on under ASCII, `NO_COLOR`
  beating everything — into a second implementation nothing tests. This is why
  the wizard's preview really is what the status line prints.
- **`config.Load` returns no error.** Every failure — unreadable file, syntax
  error, unknown key, out-of-range value — becomes a `Defaulted` note against a
  configuration that is still complete and renderable. §7.1: a broken config
  never blanks the status line.
- **`render` cannot fail.** §3.3: it recovers and writes a fallback line, and
  the fallback must be the only thing on stdout. No goroutines in that path — a
  panic in one that does not recover in the same goroutine takes the process
  down past every recover here.
- **A segment is declared in `schema.go` and nowhere else.** `SegmentNames`,
  `FormatKeys`, `TimeKeys`, `ColorKeys`, `Variants`, the `[colors]` and
  `[segments]` halves of `Defaults()` and the wizard's hint are views over
  `SegmentDefs`. Writing any of them out by hand again reintroduces exactly the
  drift that made adding `effort` a twelve-file change — and the failure is
  silent, because a table that disagrees with the registry still compiles.
- **The registry holds a segment's interface, never its implementation.** The
  keys a user can type, the placeholders each accepts, the type behind each, the
  colour that paints it. `Render` stays hand-written Go, because the bar shrinks,
  the branch carries a glyph the format never sees, duration picks one of three
  formats by elapsed time, and each decides absence its own way. The line to hold
  is: if the answer is a string a user can type, it belongs in the registry.
- **`Presentations` are positional over a segment's non-opaque keys, and must
  assign all of them.** A short one leaves the keys it does not name holding
  whatever the last variant set — a presentation nobody chose — and makes "which
  variant is this config on" unanswerable when two variants agree on the key they
  both mention. `TestEveryPresentationAssignsEveryKey` catches both directions;
  `derivedVariants` truncates rather than pads, so a too-long one would otherwise
  vanish silently.
- **Presentation is a `format` value, never a new key.** `config.Variants` is a
  ring of shipped format strings per segment, and the wizard's `tab` writes the
  one it lands on into `[segments.*] format`. Adding a `variant = "labelled"`
  key beside the format would fork one decision into two that can disagree, and
  turn a config that documents itself into documentation of a table in the
  binary. Nothing in `internal/line` outside a test reads it: the render path
  reads `format` and knows nothing about rings. The corollary is the reason there are two
  rate-limit segments and not four — two presentations of one window differ in
  nothing that being a separate segment buys, which is layout.

- **Every variant assigns every format key its segment has.** A partial one
  makes "which variant is this config on" unanswerable and leaves the unnamed
  keys holding whatever set them last. `TestEveryVariantIsACompleteAssignment`
  is the enforcement; `TestTheFirstVariantIsWhatTheDefaultsSay` is the other
  half, because a fresh config that does not sit at position 0 makes every ring
  in the wizard one entry too long.

- **Saves patch the TOML textually** (`config.ApplyOverrides` for scalar keys,
  `config.ReplaceLines` for the `[[line]]` blocks), never by marshalling a
  `Config` back out. §7.1 calls the config "documentation that happens to be
  executable"; regenerating it would strip every comment, silently, on every
  use. Where the `[[line]]` region cannot be rewritten without deleting
  something, the save **refuses and writes nothing**.
- **Empty means absent.** A zero `Rendered` omits the segment *and* its adjacent
  separator, rather than leaving a gap.
- **A window's percentage decides its presence, and a rate-limit reset is
  optional on top of it.** `used_percentage` and `resets_at` are separate
  pointers, so `{icon}` and `{reset}` are one decision and never two lookups —
  two would put a lone `↻` on the line for a payload that carried no time. The
  same rule is why the labelled variants say `resets` from inside `reset_format`
  rather than as literal text in `format`: a Go layout copies through what it
  does not recognise, so the word travels with the time and goes when it goes.

- **Anything measured in terminal columns is measured with `lipgloss.Width` or
  `style.Width`, never with `len` and never with a printf width.** `%-11s` pads
  to a *rune* count, which agrees with cells until one rune is not one column;
  `len` counts bytes and agrees with neither. The wizard's sidebar is where this
  went unnoticed, because being wrong there costs a column of alignment rather
  than a wrapped status line.

- **`flex` is not a segment**, and must stay out of `config.SegmentNames` and
  `line.New`. Those two are the vocabulary and the registry, and a test asserts
  they agree in both directions; putting a marker in either makes that test
  claim the registry can build a gap. Validation accepts it as a separate
  one-word vocabulary, which is also what keeps it out of the wizard's disabled
  pool.

### The fitter

`internal/line/fit.go` — three escalating stages, §5.6: **drop** (by descending
`drop` priority, ties rightmost-first, 99 never drops), then **truncate** (ask
`Truncatable` segments in ascending drop order; a segment may refuse and return
something wider than asked), then **clip**. Stage 3 is what makes never-wrap a
guarantee rather than an aspiration.

`Truncatable.Truncate` takes the `Context` and re-renders rather than cutting
the already-rendered string — cutting a string holding escapes loses either a
colour or a reset, and leaves nowhere to put the ellipsis in the right colour.

A fourth thing happens on the way out: `flexed` spends whatever the line did not
use on its `{name="flex"}` markers, so a row that declares one comes back
exactly `available` wide. **All three stages measure with the markers at their
floor** — a marker measured expanded would make stage 1 drop a segment to make
room for whitespace. `trimFlex` is what stops a marker becoming trailing
whitespace when the segment to its right is empty or dropped, and it is also the
only thing keeping a marker-only row off the screen; there is deliberately no
second count in `renderLine` that could come to disagree with it.

## Conventions

### PRD citations

`docs/PRD.md` is the design reference. `§` citations live in string literals —
CLI usage text, error messages, test failure messages — not in comments, so
**section numbers are load-bearing — never renumber them**. When you change
behaviour the PRD specifies, update the PRD; when the implementation corrected
the PRD, the convention is a `> **Corrected at M7 — …**` blockquote under the
section saying what it did not say and why, rather than a silent rewrite.

`TODOS.md` holds what is still open. Its `C-2`…`C-7` labels are cited by name
from the same kind of string literal (e.g. `cmd/cmd.go`'s and `cmd/preview.go`'s
usage text), so those are stable too.

### Comments

Write no comments. Identifiers and structure carry the explanation; if a name
can't carry it, restructure until it can, or put the reasoning in the PR
description instead. This applies to doc comments, section banners, and inline
notes alike — do not add any of them when writing or editing code.

The only comments in this codebase are load-bearing, not stylistic, and both
are exceptions to be preserved, not a precedent for adding more:

- Go compiler directives — `//go:embed`, `//go:build` — because a tool reads
  them, not a person.
- `config/default.toml`, which ships as the commented reference configuration
  described in §7.1 and is the one file this rule does not apply to.

One near-miss: a preset TOML's first `#` line (see `config/minimal.toml`) is
not documentation either — `presets.Summary` in `config/embed.go` parses it as
the config wizard's picker description. Removing it blanks the picker, which
is why it survives despite the rule above.

### Tests

The house rule: **verify a new test fails against the unfixed code before
accepting it.** Revert the fix, run the test, confirm it reports the symptom you
set out to catch, then restore. A test written after the fix and never seen red
is a test of nothing.

Prefer tests that assert the property rather than the rendering. Where a test
must assert layout, say what it means (`m.seek(slot{parkedRow, n-1})`) rather
than counting from an end that later work will move.
