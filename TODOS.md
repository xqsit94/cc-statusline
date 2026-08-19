# TODOS

What is still open. Everything here is unresolved — completed work is in
`CHANGELOG.md`, and the reasoning behind the design is in `docs/PRD.md`.

## Open concerns

These are cited by name from the source, so the labels are stable.

- [ ] **C-2: is the fill-relative gradient legible?** — PRD §5.5.
      Settle at the visual gate. Compare `normal-42` (4 filled cells) against
      `danger-92` (9 filled). If it reads as "same rainbow, different length"
      rather than "one early, one nearly full", switch to solid `ramp(pct/100)`
      and §5.5 changes.
- [ ] **C-3: should the default be one line, not two?** — PRD §12 Q5.
      Two lines costs two terminal rows on every prompt forever. `minimal.toml`
      ships but is not the default. Needs real feedback.
- [ ] **C-4: what `used_percentage` does compaction fire at?** — PRD §3.1.1.
      M0 proved the metric divides by the raw window, so 100% is not the ceiling
      that matters. Until this is measured, §5.4's warning=70 / danger=85 are
      guesses against a scale the number may never reach. Needs a real session
      run into a compaction, then `cc-statusline spike report`.
- [ ] **C-5: the 200k window and the null-percentage startup state are
      unobserved.** — PRD §3.1.1. Every M0 payload came from a 1M session already
      in progress. One fresh session on a 200k model closes both.
- [ ] **C-6: Powerline ships without background fills.** — PRD §6.2.
      `CC_STATUSLINE_POWERLINE=1` swaps the separator glyph for the arrow and
      nothing else, because §7.2's `[colors]` has no per-segment background.
      Decide at the gate whether the filled variant earns a sixteen-colour
      palette. Note that a flex gap is unpainted spaces, which is right for the
      shape-only variant and wrong for a filled one: a fill either has to stop
      at the gap or run through it, and that is a look, not an implementation
      detail. Settle it with the palette or not at all.
- [ ] **C-7: is `width_reserve = 12` the right number?** — PRD §5.6.
      Never measured, and load-bearing: it is the two cells that stop §5.1's
      danger state from fitting at 80 columns. Run `make probe` — the README
      explains how to read it. Needs a `settings.json` swap, so it is yours to
      run.

## The visual gate — the human half

The harness landed; the looking has not. The decidable half is `make gate-check`
(and runs under `make check`), but nothing in it renders to a screen. The
procedure is in the README under "Does it look right in *your* terminal?".

- [ ] ghostty and alacritty, at 40 / 120 / 200, across the four capability sets
- [ ] the C-7 probe, with and without a notification on screen
- [ ] iTerm2 and Terminal.app — **needs a Mac**
- [ ] screenshots into `docs/gate/`, and C-2 / C-6 / C-7 resolved with reasons

PRD §12 Q1 (Nerd Font glyph availability) and Q2 (gradient stops) close here or
not at all.

## Release

- [ ] **B-5: the repository is private**, so every release asset and
      `raw.githubusercontent.com/…/install.sh` returns 404 to anyone
      unauthenticated — GitHub answers 404, not 403, for private resources, so a
      correct release looks like a missing one. The `v0.1.0` Release job is red
      for exactly this and no other reason: GoReleaser succeeded and all five
      assets are uploaded. **Going public fixes it with no re-tag.** Decision of
      2026-08-05: stay private for now.
- [ ] **One non-you user has it installed.** Blocked on B-5.

## Unproven

- [ ] **Nobody has used the wizard.** `cc-statusline config` is built and tested,
      but there is no evidence that editing the TOML was ever in anybody's way.
      Three things would show it was not needed: nobody opens it twice; the save
      is distrusted; the drop editor is not why people open it. Answer them
      before building on it.
- [ ] **The wizard has never been looked at in a terminal.** Its layouts are
      asserted at 80 and 200 columns by test. Whether the panes read well,
      whether the faint text is legible, and whether the cursor is findable are
      questions for the same session as the visual gate.

## Known defects, not fixed

- [ ] **The truecolor escape is one unit off the configured colour**, on two of
      nine default colours, one channel each. termenv truncates where go-colorful
      rounds. Pinned in the tier-2 escape table rather than fixed — fixing it
      means not using lipgloss to paint a foreground. Revisit only if termenv
      fixes it upstream, in which case the table goes red and this is why.
- [ ] **Keep watching the p99 gate in the uninstrumented CI step.** 20ms is
      wall-clock on a shared runner, and the test already skips under `-race`
      because the instrument, not the threshold, was at fault. If the
      uninstrumented step starts flaking, the finding is about §8.1's
      measurability — record that rather than raising the threshold.

## Deferred from v1

- [ ] **Git dirty flag** (`main*`) and its subsystem — PRD §3.2, §13.
      Revisit in v0.2 only if a week of real use says the asterisk was
      load-bearing.
- [ ] **Release signing** (cosign or GitHub attestations) — PRD §10.1, §13.
      §10.1 currently gives integrity, not authenticity. Do not describe it as
      a security guarantee until this lands.
- [ ] **Format-string DSL** beyond bare `{name}` — PRD §5.7, §13.
- [ ] **Segments for the 20 unrendered payload fields** — PRD §3.1, §13.
      Gated on someone actually asking.
- [ ] **Windows support** — PRD §12 Q4, §13.
      Git discovery, XDG paths, and install prefix each need a separate answer.
- [ ] **`ccstatusline` config importer** — PRD §13. Only if adoption justifies it.

## Unanswered strategic question

- [ ] **Why not fork `felipeelias/claude-statusline` and add a gradient bar?**
      Raised by the outside voice, never answered. Not a blocker, but it is the
      baseline this project is measured against.

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
