# Changelog

## v0.2.1 — 2026-08-19

### Added

- **`cc-statusline update`.** Checks GitHub for a newer release; `--force`
  downloads the platform archive, verifies its SHA-256 against
  `checksums.txt` exactly as `install.sh` does, and atomically replaces the
  running binary. The one command that leaves the render path's no-network
  guarantee — README and CLAUDE.md now scope that claim accordingly.

## v0.2.0 — 2026-08-19

### Added

- **`ctrl+s` in `cc-statusline config`: save and apply.** `s` writes your config
  file, which the render path reads every time — which is the whole story *if
  the status line is installed*. If it is not, `s` reports a successful save and
  nothing appears, with no error anywhere, because nothing failed. `ctrl+s`
  writes the config and then installs it into Claude Code's `settings.json`,
  which is exactly what `init` does.

  It asks first, in a box that names the file, and `s` deliberately does not:
  `settings.json` holds the user's permissions, hooks and MCP servers, and that
  we touch one key in it is a promise rather than something visible from the
  wizard. `config.toml` is ours and `r` puts back what was loaded.

  `init` and `ctrl+s` install through one function. Two implementations would
  differ in whichever detail the second forgot — the shell-quoted absolute path,
  the `refreshInterval` read back from the reloaded config, the `padding` carried
  through from `settings.json`, the backup taken before the write rather than
  after — and none of those shows up as anything but a status line that does not
  appear. A test installs by both routes onto a machine where none of those is
  at its default and compares the files byte for byte.

  The help block went from two lines to three on the way: `ctrl+s save & apply`
  put the second line past ninety cells, and `helpStyle` wraps, which moves both
  panes on a terminal the user cannot widen.

- **A segment is declared once, in a vocabulary with types in it.** Adding
  `effort` touched twelve places and seven of them held nothing but the word
  "effort" again — the name list, two accessor tables, two default blocks, the
  variant ring and the wizard's help map. `internal/config/schema.go` is now the
  one declaration, and `SegmentNames`, `FormatKeys`, `TimeKeys`, `ColorKeys`,
  `Variants`, the `[colors]` and `[segments]` halves of `Defaults()` and the
  wizard's hint are all read back out of it. Nothing outside `internal/config`
  knows it exists, which is what made it possible to prove the change changed
  nothing: six scaffolding tests asserted the derivations reproduced the
  hand-written tables — writing a probe through one accessor and reading it back
  through the other, since comparing Go func values is not possible — and were
  deleted once they compared a table with itself. No golden moved.

  It declares a segment's *interface*, not its implementation. `Render` stays
  hand-written Go: the bar shrinks, the branch carries a glyph its format never
  sees, duration picks one of three formats by elapsed time, and each decides
  absence its own way. A table that could express that would be a worse-read
  programming language than the one beside it.

  Two type axes, each earning its place by deleting something. **`Syntax`** is
  why `FormatKeys` and `TimeKeys` were two hand-written lists — one validates
  `{placeholder}` grammar, the other a Go time layout, and the only way to learn
  the second existed was to add a time key to the first and watch nothing
  validate it. **`Kind`** is how a value becomes text, and it replaced three
  inline roundings and two implementations of §5.4's threshold rule.

  The `Segments` struct keeps its typed fields. Flattening it to a map would
  turn `Segments.Duration.OverHour` into a string literal and a typo from a
  compile error into a silently empty format, which is the opposite of the point.

- **`Kind` is checked rather than asserted.** It shipped in the commit above as
  a field nothing read — a type that decides nothing is a comment in a struct,
  free to drift from the code the moment either moves.
  `TestEveryFieldRendersItsDeclaredKind` renders each field alone and matches
  the output against its declaration: `KindMoney` is two decimal places,
  `KindPercent` and `KindCount` are bare integers. Nothing dispatches on `Kind`
  — a renderer calls `money` or `percent` directly, because what a segment does
  with its values is hand-written Go — so the test is the only thing making the
  declaration true. `KindText`, `KindGlyph`, `KindGauge` and `KindClock` claim
  no shape and are not checked: text is printed exactly as it arrived, and a
  clock renders through a layout the user chose.

  It catches the shape, not the arithmetic. A percentage reaching the screen as
  `85.0` fails; one rounded the wrong way does not, since both are digits —
  that direction is `TestBandBoundaries`' job, and the message says so rather
  than implying more.

- **`CONTRIBUTING.md`**, because the above is only worth anything if someone can
  find it. Adding a segment is three files, or six if it needs a new colour, new
  payload data and a documented default — verified by adding a throwaway segment
  end to end and taking it out again, not by counting from memory.

- **An `effort` segment.** `Effort: high` — the reasoning effort the session is
  set to, from the payload's `effort.level`, and it follows a mid-session
  `/effort`. PRD §12 Q3 asked which of `effort.level`, `fast_mode`,
  `thinking.enabled`, `pr.number` and `session_name` earn a segment; this
  settles the first and leaves the other four open.

  It is on neither shipped preset. `context` and `ratelimits` answer questions
  that change minute to minute; the effort level changes when you change it, and
  a row already competing with Claude Code's own output does not spend cells on
  a constant by default.

  Absence is the ordinary state, not a degraded one. Claude Code sends the
  `effort` object only on a model that has the parameter, so a session on one
  without it renders nothing here and takes its separator with it. There is no
  level to fall back to: "this model has no effort setting" and "the effort is
  low" are different facts, and showing the second for the first would be
  inventing data.

  The level is printed as it arrives, checked against no list. `low`, `medium`,
  `high`, `xhigh` and `max` are what exists today, and a sixth is exactly the
  payload schema change PRD §3.1.2 calls the likeliest failure of the next
  twelve months — printing it is how anyone finds out, where an allow-list would
  blank a segment that was holding the right answer.

  It is the one segment whose default spells its own label out, which inverts
  the axis `tab`'s ring is built on. Every other compact form identifies itself
  with a unit or a glyph — `%`, `$`, `+`/`-`, the bar, `◆`, `⎇` — and a bare
  `high` on a line of numbers identifies nothing. `tab` offers `{level}` as the
  alternative rather than as the default, and no glyph was invented for it:
  that would mean choosing three codepoints (ASCII, Unicode, Nerd Font) against
  a terminal §9.4's visual gate has never looked at. `format = "⚡ {level}"` is
  one edit in the file and needs nothing from the icon set.

- **Two single-window rate-limit segments.** `ratelimits` renders both windows
  as one unit — `5h:15% 7d:8%` — which is the right default and the wrong
  primitive. `ratelimit_5h` and `ratelimit_7d` are the halves on their own, each
  able to show the wall-clock time the window comes back.

  They are segments rather than placeholders on `ratelimits` because everything
  anyone wants from the pair is a *layout* decision — one window per row,
  different `drop` priorities, a `{name="flex"}` between them — and a segment is
  the unit the fitter drops, truncates and positions. Placeholders would have
  made the text configurable and left all three impossible. `ratelimits` is
  unchanged and stays in both shipped presets.

  Whether the clock appears is decided by the format naming `{reset}`, and by
  nothing else — there is no second key to disagree with it. `{icon}` and
  `{reset}` carry their own leading space and are one decision rather than two
  lookups, so a payload with `used_percentage` and no `resets_at` renders a bare
  `5h:15%` rather than a lone `↻`. Showing the clock makes these the widest
  segments in the schema, so the fitter takes it off at stage 2 before it drops
  anything — all of it or none of it, because `↻ 12:3` and `↻ 10 A` are not less
  precise, they are wrong.

  `reset_format` is a Go time layout, which is a second kind of format string
  and the only one whose mistakes are invisible: `time.Format` copies through
  what it does not recognise, so `reset_format = "HH:mm"` would print `HH:mm` on
  the status line forever with no error anywhere. Validation is a probe rather
  than a table — a layout that formats to *itself* substituted nothing — which
  rejects `HH:mm` and `%H:%M` and accepts anything that does substitute.

  The reset time is rendered in a zone `line.Context` carries, set by `cmd` to
  the local one. A segment reading `time.Local` for itself would be an input no
  test and no preview could vary.

- **`tab` in the wizard cycles a segment through its formats.** Mostly one axis
  — the same thing said compactly or said in words, `$0.85` against
  `Cost: $0.85`, `◆ Claude Opus 4.6` against `Model: Claude Opus 4.6`. The
  rate-limit windows cross that with a second, the clock, giving four:

  ```
  5h:15%   5h:15% ↻ 23:10   Session: 15%   Session: 15% resets 23:10
  ```

  Nothing new is stored. Each entry is an assignment to `format` keys the schema
  already had, so cycling writes `format = "Model: {name}"` into the file and the
  TOML still says what it does — a `variant = "labelled"` key beside the format
  would have turned a config that documents itself into documentation of a table
  living in the binary. `internal/line` never reads the ring.

  It is also why there are two rate-limit segments and not four. The first cut of
  this made `5h:15% ↻ 23:10` its own segment, which bought four names, four
  config tables and four sidebar rows for no expressiveness at all: two
  presentations of one window differ in nothing that being a segment buys.

  A format you wrote by hand stays in the ring, at the front, so `tab` never
  overwrites it — one full cycle brings it back, and `shift+tab` reverses, since
  undoing one press of a four-entry ring by making three more is not a cycle.
  A save writes only the keys that moved, because `minimal.toml` names no
  `[segments.*]` table and writing all fourteen would append fourteen tables of
  defaults to a file whose whole point is that it does not name them.

  `branch` is the one segment without a ring: what labels it is the glyph in
  front of it, which comes from the icon set and changes with `i`. `tab` there
  says so rather than doing nothing.

- **A hint box heading the wizard's preview.** Four lines about the sidebar row
  the cursor is on: what it is, where it ends up on the line directly below it,
  which keys apply to it, and which `[table] key`s in the file tune the parts
  the wizard does not edit.

  It gets a frame and a `>` of its own, drawn to fill the pane exactly and
  pinned to one height whichever row is selected — the rows in it wrap, and how
  many times depends on the segment, so an unpinned box at the head of the pane
  would shove the rendered line and the rule two rows up and down on every `j`.
  Everything else in that pane is output — the status line, its width, the
  budget it was fitted against — and this is the one thing addressed to the
  reader; borrowing the shape they already read as "this is talking to you"
  says so before a word of it is read. Its rows wrap with a hanging indent, so
  a sentence too long for the box continues under itself rather than under the
  label column: at a hundred columns, which is an ordinary terminal, every row
  in it wraps.

  The middle line is the reason it exists. A segment that is not on the status
  line looks the same whatever took it off, and the two causes are opposites:
  §4.3 removes one the payload had nothing to say for, and §5.6 stage 1 drops
  one the row could not afford. The first is a fact about your session and no
  edit in the wizard changes it; the second is the `drop` you chose, and `-`
  undoes it. Until now, narrowing the preview with `<` until `duration`
  disappeared and cycling to the startup payload with `n` until `duration`
  disappeared were the same experience.

  It reads the render it is heading rather than reaching its own verdict:
  `line.Trace` reports what the fitter kept, indexed the way the configuration
  is. A hint that said *dropped* about a segment visible two lines below it
  would be worse than none, and a second reading of the three stages is all it
  would take. On a terminal too short for both, the block goes and the help line
  stays.

- **Flex gaps: `{name = "flex"}` in a `segments` list.** Not a segment but a
  position — it takes whatever width the row did not use, so everything after it
  lands on the right edge of `available`. Two split the leftover evenly, giving
  three groups; one at the head right-aligns the row. `f` adds one in the
  wizard, `space` removes it.

  Every existing stage of PRD §5.6's fitter answers "the line is too wide";
  nothing answered "the line is too narrow", and the width a row did not use was
  width nobody could spend.

  It carries no `drop`. There is no width at which removing it helps: it costs
  one cell where two segments would otherwise touch, and nothing at all once the
  segment to its right has gone — a gap with nothing to push is trailing
  whitespace, so it goes with it. The fitter measures it at that floor while
  deciding what to drop and truncate, so a gap can never squeeze a segment off
  the line, and only what survives is then widened.

  A flexed row is exactly `available` cells where every other row is at most
  that. `available` is already net of `width_reserve`, so the default 12-column
  reserve is what keeps this off the terminal's own wrap column.

- **A `Presets ›` button in the wizard's sidebar.** Walk to the foot of the
  segment list and press it, and the left pane becomes the shipped presets;
  moving through them renders each one in the right pane, at whatever width and
  payload you were already looking at. `enter` applies the highlighted one,
  `esc` backs out.

  Applying is an edit like any other — unwritten until `s`, undone by `r` — and
  it brings across exactly what a save can write: the rows and the `icons` /
  `powerline` / `color` keys. That is also exactly what the preview shows, which
  is the point; a preset carries a `Result` rather than a `Config` so the two
  cannot come apart. `minimal.toml`'s bar width and branch length are therefore
  neither previewed nor applied.

  Each preset is labelled with its own first comment line, read out of the file
  rather than repeated in Go, and with how many terminal rows it costs — PRD
  §12 Q5's question, and the reason to open the list at all.

### Fixed

- **`ApplyOverrides` dropped an override whose table the file did not have.**
  It looked for the key, then for the table header to insert it under, and did
  nothing at all when there was neither. `minimal.toml` names no `[segments.*]`
  table, so a format cycled against a config based on it was reported as saved
  and was not in the file — the wizard and the file disagreeing about a change
  the user had watched themselves make, which is the one failure §7.1's textual
  patching exists to avoid. A missing table is now appended with the key under
  it.

- **§9.2's tier-2 escape table undercounted its own known defect.** The table
  freezes the exact escape emitted per (colour, profile) pair, and a note above
  it records that termenv truncates rather than rounds the truecolor channels —
  "two of the nine default colours are affected", naming `#4ade80` and
  `#45475a`. Re-running the scan while adding `[colors] effort` found four of
  ten: `#89b4fa` had been affected all along, and its row had the truncated
  value frozen in it correctly. Only the sentence was wrong, which is the
  failure mode a table of read-back values invites — the rows are generated and
  the prose above them is not.

- **The README's hint-box example was missing a row.** It showed the four lines
  the box had before `tab` existed; the box has had a `format` row since, and
  its `keys` line names `tab`. Regenerated from the code rather than retyped.

- **`QuoteTOML` wrapped a string in quotes without escaping it.** Safe by
  construction while it only ever carried enum values (`"ascii"`, `"auto"`), and
  not once formats became writable: a format containing a `"` produced a file
  that would not parse, which §7.1 turns into every key defaulting at once — the
  status line replaced wholesale over a character that had been working.

- **The wizard's sidebar had its name column written down as `11`, and the hint
  block its label column as `8`.** Each was the longest string of its kind at
  the time it was typed, with nothing saying so and nothing enforcing it. The
  rate-limit segments' longer names broke three things at once: their own `drop`
  values fell out of the column, the pane widened under the segment list but not
  under the preset picker, and the preview therefore jumped five columns sideways
  when the picker closed. A `format` label six characters long in a column sized
  for four would have put that row's value two cells right of every other while
  its wrapped continuations stayed where the constant said.

  Both columns are now computed from what goes in them, and every sidebar row —
  including the flex marker and the disabled pool, which write spacing rather
  than a priority — is measured in cells rather than hand-counted. The disabled
  pool's `—` had been a cell to the left of every priority above it.

- **The wizard previewed an 80-column terminal whatever size yours was.** Most
  shells do not export `COLUMNS`, so `style.Detect` handed the wizard §5.6's
  fallback of 80 — the frame was drawn to the real terminal and the line inside
  it for someone else's, leaving half the preview pane blank at 160 columns.

  The previewed width now follows the terminal, fitted to what the pane can
  show beside the segment list, so the rule ends where the frame does. `<` and
  `>` pin it, after which a resize leaves it alone: they ask about a width other
  than this terminal's, and resizing is not an answer. Below a 60-cell rule
  there is no two-pane layout worth having, so a narrow terminal previews itself
  and the panes stack.

- **The two wizard panes could close on different rows.** The shared height was
  measured from the pane text, but `lipgloss.Height` counts newlines and the
  wrapping happens inside `Render` — so a pane holding a line too long for its
  box was taller than the string it was measured from. Visible whenever the
  capability row did not fit, which is any terminal near 100 columns.

- **M8 hardening: every PRD §9.3 acceptance criterion now has a named test.**
  The list had twenty boxes and no way to tell which were checked; eighteen were
  already satisfied by M1–M5 and had never been written down against it.

  New coverage: `FuzzRender` over stdin (13.5M executions clean; the seed corpus
  runs every `make check`, a 60s search runs in CI, `make fuzz` runs longer);
  every optional payload field removed one at a time across all fifteen
  fixtures, generated rather than enumerated; render with `git` absent from
  `PATH`; render when `workspace.current_dir` has been deleted underneath the
  session; a 60-character model name under `ambiguous_width = 2`; and the exact
  escape sequences surviving a real pipe out of the real binary.

- **PRD §9.2's tiers 2 and 3.** Tier 2 is the exact escape emitted for each of
  nineteen colours across all four profiles — the whole sequence, because a
  `contains "\x1b[38;2;"` check passes for every row of the §6.5 bug. Tier 3 is
  four styled end-to-end goldens, plus an assertion no golden can make about
  itself: every line closes every colour it opens and ends reset, because an
  unterminated SGR bleeds into whatever Claude Code prints next and stays there.

- **§9.1's fixture backlog, complete.** Ten new payloads, each isolating one
  rule the four reference states leave to a unit test: `null-context`,
  `no-ratelimits`, `seven-only`, `no-git`, `detached`, `long-branch`,
  `500k-context`, `fractional-pct`, `wide-cost`, `sub-minute`. They are not
  reference states — §9.4's human gate still shows four blocks, not fifteen.
  `TestFixturesStillIsolateWhatTheyWereBuiltFor` names the rule each exists for,
  so a fixture cannot be edited into decoration while its golden regenerates.

- **§8.1's budget is measured for the first time.** `TestRenderProcessBudget`
  forks the real binary 100 times and times execve to exit: **p50 1.17ms, p99
  1.30ms** idle on an i5-10400, and **4.16 / 9.07ms** with all twelve threads
  saturated, against a 20ms budget. Every earlier number in this repository
  measured only what happens after process start, which §8.1's own table calls
  the smaller half. The loaded row is kept because a status line renders while
  you are working, and working often means a compile is using every core.

- **`make golden`, `make fuzz`, `make p99`.** §9.2 specified the first and it
  did not exist. `make p99` also builds §9.3's 50,000-file repository and uses
  `hyperfine --shell=none` when it is installed.

- **`cc-statusline config`** — PRD §10.4's wizard. Two panes: the segment list
  on the left, the status line on the right, re-rendered on every keystroke.
  Enable and disable segments, reorder them within a row or between rows, set
  drop priorities, and flip the preview across icon sets, Powerline and all four
  colour profiles — including ones the terminal does not have. `<`/`>` move the
  previewed width, `n` cycles the payload between the captured session and §5.1's
  reference states, `s` saves, `r` resets, `q` discards.

  It saves by **patching the file rather than regenerating it**, so every comment
  survives; §7.1 calls the config "documentation that happens to be executable"
  and a wizard that stripped it would make the file worse on every use. Where the
  `[[line]]` region holds something it cannot regenerate — a comment between two
  rows — it refuses and writes nothing, on the same principle as `init`'s refusal
  to edit a `settings.json` with comments. The refusal fires *before* the editor
  opens rather than after the edits are made.

  `internal/wizard` performs no I/O at all, which is what lets a TUI be tested as
  a state machine: every binding, every reorder edge, the reset, and the
  save/discard promise are asserted without a terminal.

### Removed

- **`line.Names`**, unreachable: its doc said it was "for `doctor` and the M7
  wizard" and both had long since read `config.SegmentNames` directly.

- **Three unused parameters** — the `*style.Style` every `text()` call passed
  and none used, `writeText`'s `env`, and a test helper's temp-dir return. The
  first was the one worth removing: nothing in `text` paints, so a `Style`
  argument implied the colouring happened there rather than in `expand`, and
  twenty call sites carried the implication.

### Changed

- **Adding a preset is adding a file.** `config/` is embedded whole rather than
  one `//go:embed` variable per file, so a `.toml` dropped in appears in
  `Names()`, in `init --preset`'s help, in the installer and in the wizard's
  picker with the summary read from its own first comment line. The tests were
  the worse half of the old shape: they asserted `default` and `minimal` load
  cleanly by naming each one, so a third preset would have shipped with nothing
  checking that it even parsed — under §7.1, which requires a broken config to
  default silently rather than complain. `TestEveryPresetLoadsCleanly` walks the
  directory.

- **§9.2's tier-2 escape freeze is keyed by colour value, not colour name.** Ten
  of its twenty rows were duplicates — `cost`, `normal`, `added` and
  `gradient_stops[0]` are all `#4ade80` and each held its own copy of the same
  three sequences. `escapeByHex` freezes what each distinct value emits and
  `defaultColors` freezes which value each key holds; the same eighty assertions
  run, and a segment reusing an existing colour now needs nothing added at all.
  Deduplicating also made "is this frozen sequence still reachable" a question
  with one answer, which `TestNoFrozenEscapeIsUnused` now asks.

- **`line.New`'s switch became a map filled beside each renderer**, so a
  segment's name sits with its code. It also made the reverse contract exact for
  the first time: `TestRegistryMatchesConfig` used to walk the names the default
  config happened to use, because nothing can enumerate a switch statement, so a
  renderer registered under a name `config` had never heard of was invisible
  unless a shipped preset used it.

- **The wizard's hint derives its key list.** The prose is still written by hand;
  the `[segments.*]` and `[colors]` halves are not, and that fixed a drift the
  block existed to prevent — `duration` named its four keys and none of its
  colours, so the pane told anyone looking that the segment had no colour to
  configure. `context` and `ratelimits` named none of theirs either, and the two
  put their thresholds on opposite ends of the line.

- **The wizard's preview no longer prints `N cells` under each rendered row.**
  It doubled the pane's height to answer a question the width rule two lines
  below already answers better — what matters is not how wide a row is but how
  much of the budget is left, and the rule shows that by running out. It also
  put a line on screen that the status line never prints, in the one pane whose
  whole claim is that it shows what the status line prints. The rule keeps its
  own number; that one is the budget the `drop` priorities are fought over.

  Nothing about the measurement changed, only its display: the fitter reads the
  same width under the same environment, which is what the test that used to
  cover the counts now asserts directly.

- **The decidable half of the M4 visual gate is Go, not Python.**
  `scripts/gate-check.py` is gone. It made a project whose toolchain is one `go`
  binary depend on python3, and it put the checks where `go test ./...` could not
  see them. Half of it also duplicated existing assertions, worse — re-deriving
  by regex over printed output what `internal/line`'s `TestNoLineExceedsAvailable`
  already asserts directly. What was genuinely new is now `cmd/gate_test.go`, and
  `make gate-check` selects it.
- `applyTOMLOverrides` moved from `cmd/init.go` to `internal/config/edit.go` and
  is shared with the wizard. A second implementation of "patch a TOML key in
  place" would have drifted until `init` and `config` disagreed about the same
  file.
- `previewEnv` and `widthRule` became `style.Overlay` and `style.Rule` for the
  same reason: the wizard needed both, and the alternative was two of each.

### Fixed

- **The wizard model shared slice backing arrays across frames.** Bubble Tea
  passes the model by value and takes a new one back, which reads as value
  semantics; a slice field carries a pointer, so an edit reached backwards into
  the model that had been passed in. Invisible while running — the previous
  frame is discarded — and caught by the test that holds a baseline and presses
  one key against it.

- **`make bench` measured nothing and reported PASS.** It had run
  `go test ./internal/line -bench .` since M3 against a package with no
  `Benchmark` function, and `go test` calls an empty `-bench` run a pass — so
  the target answered "is it fast enough" with a green line. Four benchmarks
  added, and the target now fails if none of them run. `FitNarrow/40` at 0.23ms
  against §8.1's 0.3ms line item is the row with the least room in it.

- **PRD §9.2's own worked example was wrong at 256 colours.** It gave
  `#4ade80 → \x1b[38;5;114m`. Palette 114 is `#87d787` (135,215,135); palette 78
  is `#5fd787` (95,215,135). Against (74,222,128) the squared distances are 3819
  and 539. The code has always picked 78; the document is corrected.

- **PRD §9.3's install criterion "the command `init` writes executes" was
  asserted as a string and never executed.** M5 checked the shell quoting
  thoroughly and never handed the result to a shell — the failure mode being a
  string that compares equal to the expected value and breaks the first time a
  user's path has a space in it. The test now builds under
  `…/Application Support/it's here/`, installs, reads the command back out of
  `settings.json`, and runs it with `env -i sh -c`.

### Measured

- **Bubble Tea costs 0.07ms of package init and 160KB of binary.** `render`
  never touches it, so that is the whole cost, and it is under half a percent of
  §8.1's 20ms p99. §13's `-tags minimal` build stays deferred, now on evidence
  rather than assumption.

- **§8.1's p99, from execve to exit: p50 1.17ms, p99 1.30ms** against a 20ms
  budget, over 100 consecutive renders on an idle i5-10400 — and 4.16 / 9.07ms
  on the same machine with all twelve threads busy.

- **A 50,000-file repository costs no more than a three-file one** — 1.177ms
  against 1.199ms, which is to say the large one measured marginally faster and
  the difference is noise. §5.8 walks upward and reads one file, so file count
  cannot appear in the cost; the test asserts that claim rather than §9.3's
  threshold, because the threshold would go on passing while a future dirty flag
  made it slower.

- **The first p99 figure published here was wrong by 9×**, and the correction is
  worth more than the number. It read p50 5.5ms / p99 12.2ms, and every sample
  had been taken while this milestone's own fuzzer saturated all twelve threads.
  A wall-clock measurement records the machine as much as the program.

### Known gaps

- **Nobody has used the wizard.** It was to be decided by a week of real use and
  that week has not happened; the milestone is code, not evidence. `TODOS.md`
  records in advance what would show it was not needed.
- It has never been looked at in a terminal. The layouts are asserted by test at
  80 and 200 columns; whether they *read* well is B-3's kind of question.

- **The truecolor escape is not quite the configured colour, and this is
  accepted rather than fixed.** `#4ade80` reaches the terminal as
  `38;2;73;222;128`; `0x4a` is 74. termenv's `RGBColor.Sequence` truncates
  `f.R*255` where go-colorful's `RGB255()` rounds, and the two live in the same
  dependency tree. Two of the nine default colours are affected, on one channel
  each, by one unit in 255 — imperceptible. Fixing it means not using lipgloss
  to paint a foreground at all (its `TerminalColor` interface has an unexported
  method), and rebuilding §6.5's render path to move one unit of blue is the
  wrong trade. Pinned in the tier-2 table so it is known rather than latent.

- **Every §9.3 box being ticked is a claim about what a machine can decide.**
  The M4 visual gate's human half is still unrun, and C-2, C-4, C-5, C-6 and C-7
  are all still open — see `TODOS.md`, so the two cannot be confused.

---

## v0.1.0 — 2026-08-05

The first release. `0.1.0` rather than `1.0.0` deliberately: the interface most
likely to move is §7.2's config schema, and pre-1.0 is the honest signal that it
might. The payload contract is Claude Code's, not ours, which is why §3.1.2's
drift detection exists at all.

Tagged with the second exit criterion still open — one non-you user has it
installed — because that one cannot be satisfied before a release exists. The
M4 visual gate's human half is also outstanding; see Known gaps.

### Added

- **`cc-statusline init`** — writes a commented `config.toml` from a preset and
  points Claude Code's `statusLine` at this binary by absolute path. Idempotent,
  backs the file up first, writes atomically, follows symlinks to their target,
  preserves permissions, and matches the file's own indentation. `--dry-run`,
  `--preset`, `--icons`, `--force`.
- **`cc-statusline uninstall`** — removes the `statusLine` key and nothing else.
  Declines to remove a status line belonging to another program without
  `--force`. Never restores a backup, because backups are per-`init` and
  restoring one would revert every unrelated edit made since.
- **`cc-statusline doctor [--json]`** — what this build actually saw: the
  install, the config and what it could not use, the resolved capabilities *and
  the variable that decided each one*, the last payload and how it differs from
  what this build models, and whatever the last render had to repair.
- Distribution: `.goreleaser.yaml`, a release workflow that installs its own
  release through `install.sh` before finishing, `install.sh` with SHA-256
  verification against a pinned tag, a `Makefile`, and CI on Linux and macOS.
- `README.md` and `LICENSE` (MIT).

### Fixed / corrected from the design

- **PRD §10.2's step order was wrong.** It compared the existing `statusLine` to
  the desired value *before* checking whether the file was plain JSON. Because
  gjson finds keys inside comments, a commented-out `statusLine` would have been
  read as an existing installation and `init` would have reported success
  forever without ever writing anything. The plain-JSON check now runs first.
- **C-1 settled.** sjson does not "mis-locate the edit" on a commented file as
  §10.2 predicted — it appends correctly and the comments survive. The real
  failure is narrower and worse (above), and trailing commas corrupt the file
  outright. The refusal path stays; the gate is `gjson.ValidBytes` rather than a
  hand-written comment scanner, which also gets `//` inside a string literal
  right for free.
- **§7.1 said `render` *appends* to `last-error.txt`.** It must not: `render`
  runs every sixty seconds in every session, so a single typo'd config key would
  write about fourteen hundred identical lines a day. The file is rewritten with
  the current set instead, and removed once the problem is fixed.
- `init` shell-quotes the binary path. Claude Code runs the command through a
  shell, and a `go install` on macOS can land it under a path with a space in it.
- **`.goreleaser.yaml` had never been parsed.** The checksum block was spelled
  `checksums:`; GoReleaser rejects unknown fields, so no release could have been
  built from the config at all. Caught by `goreleaser check` on the first CI run
  against the published remote — which is the run before the tag, not after it.
- **Two `internal/settings` tests only passed on Linux.** They compared paths
  against a raw `t.TempDir()`, but on macOS that sits under `/var/folders` and
  `/var` is a symlink to `/private/var`, so `EvalSymlinks` resolved further than
  the assertion expected. The behaviour was correct — resolving the last link is
  what keeps `Write` from replacing a dotfiles symlink with a regular file — so
  the fix was to the tests, which now resolve up front.
- Five dependencies this code imports directly were marked `// indirect` in
  `go.mod`. `go mod tidy` runs as a release `before` hook, so leaving it stale
  would have dirtied the tree mid-release, and GoReleaser refuses to build from
  a dirty tree.

### Known gaps

- No release signing. `checksums.txt` is integrity, not authenticity — PRD §13.
- Windows is unsupported — PRD §13.
- **The M4 visual gate's human half has not been run.** Its decidable half is
  `make gate-check` and passes in CI, but nothing in it renders to a screen.
  Three questions are therefore open at v0.1.0 rather than closed before it:
  whether the Nerd Font glyphs draw as glyphs (§12 Q1), whether the
  fill-relative gradient is legible and reads as a level rather than a rainbow
  (C-2), and whether the bare Powerline arrow reads as intentional (C-6). If
  C-2 resolves the other way, §5.5 switches to a solid `ramp(pct/100)` and the
  most visible thing in the product changes in a later release.
- `width_reserve = 12` is unmeasured (C-7). It can only be read off Claude Code's
  own rendering — `make probe`, and the README has the procedure.
- §5.4's thresholds are guesses until C-4 is measured against a real compaction,
  and the 200k window and null-percentage startup state are unexercised (C-5).

---

## M0–M4 (pre-release)

Each milestone corrected something the document had asserted without measuring:

- **M0** — the payload contract. Corrected four factual claims in §3.
- **M1** — skeleton and the §3.3 failure contract.
- **M2** — segments, capabilities, git discovery. Corrected §6.5.
- **M3** — config, fitting, gradient. Three corrections to §5.
- **M4** — the visual gate harness (`preview`, `--probe`, `internal/refstate`).
  Corrected §9.4's terminal and locale axes; found the reference payloads
  existing in three copies, two agreeing by coincidence.
