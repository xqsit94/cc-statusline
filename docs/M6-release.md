# M6 — releasing v0.1

PRD §11 gives M6 two exit criteria: the tag is published, and **one non-you user
has it installed**. Only the first is code. This document is the rest.

> **M6 is a real gate, not a formality** (§11). The wizard at M7 is the largest
> remaining investment and its value is unproven. Shipping v0.1 first makes M7 a
> decision informed by use rather than a bet placed before it. Tagging before
> the questions below are answered would forfeit exactly that.

---

## 0. What is blocking, and who has to unblock it

| # | Blocker | Whose call |
|---|---|---|
| ~~B-1~~ | ~~No `LICENSE` file.~~ **Done** — MIT, copyright `xqsit94`. If you want your legal name on it rather than the handle, change it before the tag; after publication the holder is on record in every copy anyone has. | — |
| B-2 | There is no git remote. `git remote -v` is empty, so there is nothing to push a tag to and no Releases page for `install.sh` to fetch from. | **Yours** |
| B-3 | The M4 visual gate has not been run by a human. `docs/M4-visual-gate.md` is the checklist. | **Yours** — see §1 |
| B-4 | C-4 and C-5 are unmeasured, so §5.4's thresholds are guesses. | Time — see §2 |

B-2 is five minutes. B-3 is about twenty. B-4 is a week of use, which is what §11
was asking for when it wrote "use it yourself for a week".

---

## 1. Run the visual gate first (B-3)

Not because a process says so, but because two of its findings can change what
ships:

- **C-2** — if the fill-relative gradient reads as "same rainbow, different
  length", §5.5 switches to a solid `ramp(pct/100)`. That is a change to the
  single most visible thing in the product, and it is much cheaper before a tag
  than after one.
- **C-6** — if the bare Powerline arrow reads as broken rather than intentional,
  `powerline = "auto"` should not be defaulting it on for Nerd Font users.

`docs/M4-visual-gate.md` §3 and §5. Twenty minutes, two terminals.

---

## 2. Then use it, for a week (B-4)

```sh
make install
cc-statusline init
```

This replaces the M0 spike passthrough currently in your `settings.json`. Keep
the spike's captures if C-4 and C-5 are still open — `cc-statusline capture` is
the drop-in for `render` that keeps recording payloads while showing the real
line.

What a week is actually for:

| Question | Where it lands |
|---|---|
| **C-4** — what `used_percentage` does compaction fire at? | §5.4's `warning = 70` / `danger = 85` are guesses against a scale the number may never reach. One session run into a compaction answers it. |
| **C-5** — the 200k window and the null-percentage startup state | Every M0 payload came from a 1M session already in progress. One fresh session on a 200k model closes both. |
| **Q5 / C-3** — one line or two? | Two lines costs two terminal rows on every prompt forever. §12 Q5 explicitly says revisit at M6 *with real feedback*, and a week is the feedback. If the second line has not earned its row, `minimal.toml` becomes the default. |
| **C-7** — is `width_reserve = 12` right? | `docs/M4-visual-gate.md` §4. Needs a `settings.json` swap, so it is yours to run. |

Record each answer in PRD §14.1 with a reason, not just a verdict.

---

## 3. Tagging

Once B-1 through B-3 are closed:

```sh
make check                     # gofmt, vet, tests
make release-dry               # builds every artefact locally, publishes nothing
git tag -a v0.1.0 -m "…"
git push origin v0.1.0
```

`.github/workflows/release.yml` takes it from there: GoReleaser runs the test
suite again as a `before` hook, builds `linux/darwin × amd64/arm64`, publishes
the archives and `checksums.txt`, and then **installs the release it just made
via `install.sh` and renders a line with it**. That last step is deliberate:
the curl-pipe-sh path is the channel §10.1 puts first and the one nobody
exercises until a stranger tries it.

If the release job fails after publishing, delete the release and the tag rather
than patching over it. A `v0.1.0` that means two different things is worse than
no `v0.1.0`.

### Version numbers

`0.1.0`, not `1.0.0`. The interface most likely to change is §7.2's config
schema, and pre-1.0 is the honest signal that it might. The payload contract is
Claude Code's, not ours, and §3.1.2's drift detection exists because it can move
under us at any time.

---

## 4. The second exit criterion

> One non-you user has it installed.

Not a formality either. Everything in this repository has been read by its
author, and the failure modes that matter most — the install on a machine
configured differently, the terminal with a different font, the `settings.json`
with something unexpected in it — are precisely the ones a single machine cannot
produce. The install corpus in `internal/settings/testdata/` was written by
imagining those files. One real user finds the case nobody imagined.

Ask for one thing back: the output of `cc-statusline doctor`. It reports the
resolved capabilities and the environment variable that decided each one, which
is the difference between an actionable bug report and "the icons look wrong".

---

## 5. After the tag

M7 (the Bubble Tea wizard) is the next milestone, and §11 is explicit that
shipping first makes it a decision rather than a bet. Before starting it, answer:
after a week of real use, did you want to change the config, and did editing the
TOML actually get in the way? If it did not, M8 (hardening) is the better next
milestone and the wizard can wait for someone to ask for it.
