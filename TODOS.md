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

- [ ] **C-1: does sjson handle `//` comments?** — PRD §14.1, task T19.
      Ten-line test. Write it before building the refusal path in §10.2 step 5;
      if sjson handles them, delete that path.
- [ ] **C-2: is the fill-relative gradient legible?** — PRD §14.1, §5.5.
      Settle at the M4 visual gate. If it reads as "same rainbow, different
      length," switch to solid `ramp(pct/100)`.
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

## Unanswered strategic question

- [ ] **Why not fork `felipeelias/claude-statusline` and add a gradient bar?**
      Raised by the outside voice, never answered. Not a blocker, but it is the
      baseline this project is measured against. Worth a paragraph in the README.

## M0 spike — installed, leave it running

`~/.local/bin/cc-statusline-spike` is wired into `~/.claude/settings.json` as a
passthrough in front of `~/.claude/statusline.sh`, so the existing status line is
unchanged while payloads accumulate in `~/.cache/cc-statusline/spike`. It stays
until C-4 and C-5 close.

- Read the findings: `cc-statusline-spike report`
- Rebuild after edits: `go build -o ~/.local/bin/cc-statusline-spike ./cmd/cc-statusline`
- Remove it: restore `~/.claude/settings.json.pre-m0.bak`
- Delete at M1: `internal/spike/` and its `cmd` cases go when `internal/payload`
  lands. Carry `capture_test.go`'s §3.3 assertions forward; they are the only
  part of the spike that is not throwaway.

## Implementation tasks

19 tasks from the engineering review live at
`~/.gstack/projects/cc-statusline/tasks-eng-review-20260805-175046.jsonl`
(readable by `/autoplan`). Priorities: 6× P1, 9× P2, 4× P3.
