# cc-statusline

A status line for [Claude Code](https://claude.com/claude-code) that answers four
questions at a glance and then gets out of the way.

```
◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42% │ $0.85 │ 3m │ 5h:15% 7d:8%
⎇ main │ +150/-30 │ my-project
```

- **How much context is left?** — a gradient bar that fills as the window does
- **What is this costing?** — session spend, and how long you have been at it
- **Am I near a rate limit?** — both windows, shown only when they matter
- **Where am I?** — branch, diff, project

One static binary. No Node, no Python, no subprocesses, no network. It reads a
JSON payload on stdin and prints one or two lines.

---

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/xqsit94/cc-statusline/main/install.sh | sh
cc-statusline init
```

Or, with a Go toolchain:

```sh
go install github.com/xqsit94/cc-statusline@latest
cc-statusline init
```

`install.sh` downloads a release archive and its `checksums.txt`, verifies the
SHA-256 before unpacking, and installs into `$XDG_BIN_HOME` or `~/.local/bin`
(override with `PREFIX`). It does **not** touch your `settings.json` — that is
what `init` is for, and it tells you what it is about to do.

> **Integrity, not authenticity.** `checksums.txt` ships from the same release
> as the binary it attests to, so it catches a truncated or corrupted download
> and nothing else. Release signing is not implemented yet. Please do not treat
> the checksum as a supply-chain guarantee it is not.

### What `init` does

1. Writes `~/.config/cc-statusline/config.toml` from a commented preset — never
   overwriting an existing one without `--force`.
2. Points Claude Code's `statusLine` at this binary, by absolute path.
3. Prints a preview of what you will see.

It is idempotent: running it twice makes no second backup and changes no bytes.
It backs `settings.json` up before touching it, writes atomically, follows a
symlink to its target rather than replacing the link, and preserves the file's
permissions and formatting. If your `settings.json` has comments or a trailing
comma in it, `init` **declines to edit it** and prints the block to paste
yourself — see [Why it refuses](#why-init-refuses-a-settingsjson-with-comments).

```sh
cc-statusline init --dry-run       # show what would change, touch nothing
cc-statusline init --preset minimal # one line instead of two
cc-statusline uninstall            # remove the statusLine key, nothing else
```

---

## When something looks wrong

```sh
cc-statusline doctor
```

It reports what this build actually saw: whether the status line is wired up and
to which binary, which config file was read and what it could not use, the
resolved capabilities *and the environment variable that decided each one*, the
last payload Claude Code sent and how it differs from what this build models,
and anything the last render had to repair. `--json` for scripting.

---

## Configuration

Everything is configurable, in `~/.config/cc-statusline/config.toml`. The
installed file is fully commented; the two shipped presets are
[`config/default.toml`](config/default.toml) and
[`config/minimal.toml`](config/minimal.toml).

```toml
[general]
icons            = "unicode"   # "ascii" | "unicode" | "nerdfont"
powerline        = "auto"      # arrow separators, if your font has them
color            = "auto"      # "none" | "16" | "256" | "truecolor"
width_reserve    = 12          # cells kept clear for Claude Code's notifications

[thresholds]
warning = 70                   # the bar turns amber here
danger  = 85                   # …and red here

[[line]]
segments = [
  {name = "model",   drop = 99},   # 99 never drops
  {name = "context", drop = 99},
  {name = "cost",    drop = 60},
  {name = "duration", drop = 70},  # higher drops first when the line overflows
]
```

Invalid values never break the status line. Every one of them is repaired to its
default, the render proceeds, and `doctor` tells you what was repaired — a
config typo should cost you a segment, not your prompt.

### Environment overrides

For a single invocation, without editing anything:

| Variable | Effect |
|---|---|
| `CC_STATUSLINE_ASCII=1` | force the ASCII glyph set |
| `CC_STATUSLINE_NERDFONT=1` | force the Nerd Font glyph set |
| `CC_STATUSLINE_POWERLINE=1` | force Powerline separators |
| `CC_STATUSLINE_COLOR=…` | `none` \| `16` \| `256` \| `truecolor` |
| `CC_STATUSLINE_NO_GIT=1` | skip git discovery |
| `CC_STATUSLINE_CONFIG=…` | read a different config file |
| `NO_COLOR=1` | no escapes at all; always wins |

`ASCII` beats `NERDFONT` when both are set: ASCII is the compatibility floor, and
someone who asked for it has a terminal that cannot show the alternative.

---

## Does it look right in *your* terminal?

No test can answer that. The goldens measure width with the same library the
renderer uses, so they prove self-consistency and never that your terminal
agrees — and nothing in Go can tell a rendered Nerd Font glyph from a
replacement box. So there is an instrument instead:

```sh
cc-statusline preview --matrix
```

Every capability set, with a **width rule** drawn under each line in pure ASCII:

```
◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42% │ $0.85 │ 3m │ 5h:15% 7d:8%
|---------------------------- 62 ----------------------------|
⎇ main │ +150/-30 │ my-project
|------------ 30 ------------|
```

ASCII is East Asian Narrow everywhere, under every locale and every ambiguous-
width setting, so the rule is the one line on screen the program cannot be wrong
about. If the two lines end in the same column, the width model and your terminal
agree. If they do not, they disagree by exactly that many cells, and you have a
bug report with a number in it.

---

## Why `init` refuses a `settings.json` with comments

Claude Code's `settings.json` is JSON, but people put `//` comments in it anyway
and most tooling tolerates that. This one will not edit such a file, for a
specific reason that is worth stating.

The JSON scanners that let us edit one key while preserving every other byte do
not know that a comment is not data. Given

```jsonc
{
  // "statusLine": {"type": "command", "command": "disabled-on-purpose"},
  "theme": "dark"
}
```

a scanner reports `statusLine` as **present**, and an automatic edit rewrites the
value *inside the comment*. You would end up with a commented-out status line
that mentions this program, no live `statusLine` key at all, and an `init` that
reports success — and keeps reporting success on every later run, because it
reads back the value it wrote into the comment. A visibly broken file would be
better than that. A silent no-op is undiagnosable.

Trailing commas fail the same way and worse. So `init` checks whether the file
is plain JSON *before* reading anything out of it, and if it is not, it prints
the block for you to paste and changes nothing.

---

## Deliberately not included

- **A git dirty flag.** It needs subprocess spawning, a cache, atomic writes,
  garbage collection, timeouts, and a lock protocol — the largest subsystem in
  the design — to render one asterisk. Revisit if a month of use says the
  asterisk was load-bearing.
- **A format-string DSL.** `{name}` substitution, and nothing else.
- **Transcript parsing or usage-API calls.** Everything shown comes from the
  payload Claude Code already hands us.
- **Windows.** Git discovery, XDG paths, and the install prefix each need an
  answer nobody has written.

---

## Development

```sh
make check      # gofmt, vet, tests
make build      # ./bin/cc-statusline
make install    # into ~/.local/bin
make gate       # the visual gate, docs/M4-visual-gate.md
```

The design document is [`docs/PRD.md`](docs/PRD.md). It is unusually long for a
status line and unusually honest about what has been measured versus assumed —
`§14 Review history` records what each milestone proved wrong.

## License

MIT — see [`LICENSE`](LICENSE).

## Prior art

[`felipeelias/claude-statusline`](https://github.com/felipeelias/claude-statusline)
is the closest thing and is a good, small tool. This one exists because of the
context bar, the width model, and the capability degradation — if you do not want
those, prefer the smaller program.
