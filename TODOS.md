# TODOS

Pointers only. The reasoning lives in `docs/PRD.md` — this file exists so `/ship`
and `/review` can see what was deferred without reading a 1,300-line spec.

## Deferred from v1

- [ ] **Git dirty flag** (`main*`) and its subsystem — PRD §3.2, §13.
      Revisit in v0.2 only if a week of real use says the asterisk was load-bearing.
- [ ] **Release signing** (cosign or GitHub attestations) — PRD §10.1, §13.
      §10.1 currently gives integrity, not authenticity. Do not describe it as
      a security guarantee until this lands.
- [ ] **Format-string DSL** beyond bare `{name}` — PRD §5.7, §13.
- [ ] **Segments for the 20 unrendered payload fields** — PRD §3.1, §13.
      Gated on someone actually asking.
- [ ] **Windows support** — PRD §12 Q4, §13.
      Git discovery, XDG paths, and install prefix each need a separate answer.
- [ ] **`-tags minimal` build** excluding Bubble Tea — PRD §13.
      Only if measurement shows package init is material against §8.1.
- [ ] **`ccstatusline` config importer** — PRD §13. Only if adoption justifies it.

## Open concerns to settle during implementation

- [x] **C-1: does sjson handle `//` comments?** — CLOSED at M5. It does, and the
      refusal path stays anyway: gjson finds keys *inside* comments, so a
      commented-out `statusLine` would be rewritten in place and `init` would
      report success forever. §10.2's step order was inverted as a result.
      See PRD §14.1 and `internal/settings`.
- [ ] **C-2: is the fill-relative gradient legible?** — PRD §14.1, §5.5.
      Settle at the M4 visual gate. If it reads as "same rainbow, different
      length," switch to solid `ramp(pct/100)`.
      Run: `docs/M4-visual-gate.md` §3.3.
- [ ] **C-3: should the default be one line, not two?** — PRD §12 Q5, §14.1.
      Two lines costs two terminal rows on every prompt forever. Revisit at M6
      with real feedback.
- [ ] **C-4: what used_percentage does compaction fire at?** — PRD §3.1.1, §14.1.
      M0 proved the metric divides by the raw window, so 100% is not the ceiling
      that matters. Until this is measured, §5.4's warning=70 / danger=85 are
      guesses against a scale the number may never reach. Needs a real session
      run into a compaction, then `cc-statusline-spike report`.
- [ ] **C-5: 200k window and the null-percentage startup state are unobserved.**
      — PRD §3.1.1, §14.1. Every M0 payload came from a 1M session already in
      progress. One fresh session on a 200k model closes both.
- [ ] **C-6: Powerline ships without background fills.** — PRD §14.1, §6.2.
      `CC_STATUSLINE_POWERLINE=1` swaps the separator glyph for the arrow and
      nothing else, because §7.2's `[colors]` has no per-segment background.
      Decide at M4 whether the filled variant earns a sixteen-colour palette.
      Run: `docs/M4-visual-gate.md` §5.
- [ ] **C-7: is `width_reserve = 12` the right number?** — PRD §14.1, §5.6.
      Never measured, and now load-bearing: it is the two cells that stop §5.1's
      danger state from fitting at 80 columns.
      Run: `docs/M4-visual-gate.md` §4 — install `preview --probe` as the
      statusLine command and read the number off Claude Code's own rendering.
      Needs a settings.json swap, so it is yours to run, not mine.

## M4 — the human half

The harness landed; the looking has not. `docs/M4-visual-gate.md` is the
checklist. Outstanding:

- [ ] ghostty and alacritty, at 40 / 120 / 200, across the four capability sets
- [ ] the C-7 probe, with and without a notification on screen
- [ ] iTerm2 and Terminal.app — **needs a Mac**; due before v0.1 is tagged (M6)
- [ ] screenshots into `docs/gate/`, findings table filled in, §14 updated

`preview` renders the same payloads the goldens assert against, so a signature
on its output is a signature on the shipped bytes — `TestPreviewShowsWhatThe`
`GoldensAssert` is what keeps that true.

## M6 — what blocks the tag

`docs/M6-release.md` is the checklist and the reasoning. The four blockers:

- [x] **`LICENSE`** — MIT, copyright `xqsit94`. Change the holder to your legal
      name before the tag if you want it there; afterwards it is on record in
      every copy anyone has.
- [ ] **No git remote.** `git remote -v` is empty — nowhere to push a tag, and
      no Releases page for `install.sh` to fetch from.
- [ ] **The M4 visual gate is unrun.** Two of its findings (C-2, C-6) can change
      what ships, and both are much cheaper before a tag than after one.
- [ ] **C-4 and C-5 unmeasured**, so §5.4's thresholds are still guesses. This is
      the week of real use §11 asked for; §12 Q5 (one line or two) resolves in
      the same week.

Also unset: the repository has no git identity, so every commit so far needed
`git -c user.name=… -c user.email=…`. Worth configuring before a tag carries
your name.

## Unanswered strategic question

- [ ] **Why not fork `felipeelias/claude-statusline` and add a gradient bar?**
      Raised by the outside voice, never answered. Not a blocker, but it is the
      baseline this project is measured against. Worth a paragraph in the README.

## M0 spike — installed, leave it running

`~/.local/bin/cc-statusline` is wired into `~/.claude/settings.json` as
`spike capture -- bash "$HOME/.claude/statusline.sh"`: a passthrough in front of
the existing status line, so nothing on screen changes while payloads accumulate
in `~/.cache/cc-statusline/spike`. It stays until C-4 and C-5 close.

- Read the findings: `cc-statusline spike report`
- Rebuild after edits: `go build -o ~/.local/bin/cc-statusline .`
- Remove it: restore `~/.claude/settings.json.pre-m0.bak`
- Delete when C-4 and C-5 close: `internal/spike/`, `cmd/spike.go`, and one case
  in `cmd.Main`. Nothing else depends on it — the report already runs through
  `payload.FlattenKeys`, and `capture_test.go`'s §3.3 assertions are carried
  forward in `cmd/render_test.go`.

> `spike capture` and `capture` are deliberately different. The first passes the
> payload through to another status line command and forwards its output; the
> second renders our own. Swapping one for the other silently changes what is on
> screen, which is why they are not the same subcommand.

## Implementation tasks

19 tasks from the engineering review live at
`~/.gstack/projects/cc-statusline/tasks-eng-review-20260805-175046.jsonl`
(readable by `/autoplan`). Priorities: 6× P1, 9× P2, 4× P3.
