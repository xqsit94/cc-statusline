# Changelog

## Unreleased — v0.1.0 candidate

The first release-shaped state. Not yet tagged; `docs/M6-release.md` lists what
is still blocking.

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
- `README.md`.

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

### Known gaps

- No release signing. `checksums.txt` is integrity, not authenticity — PRD §13.
- Windows is unsupported — PRD §13.
- The M4 visual gate has not been run by a human; C-2, C-6 and C-7 are open.
- §5.4's thresholds are guesses until C-4 is measured against a real compaction.

---

## M0–M4 (pre-release)

Milestone history lives in `docs/PRD.md` §11 and §14. Each milestone corrected
something the document had asserted without measuring:

- **M0** — the payload contract. Corrected four factual claims in §3.
- **M1** — skeleton and the §3.3 failure contract.
- **M2** — segments, capabilities, git discovery. Corrected §6.5.
- **M3** — config, fitting, gradient. Three corrections to §5.
- **M4** — the visual gate harness (`preview`, `--probe`, `internal/refstate`).
  Corrected §9.4's terminal and locale axes; found the reference payloads
  existing in three copies, two agreeing by coincidence.
