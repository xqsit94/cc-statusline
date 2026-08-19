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

One static binary. No Node, no Python, no subprocesses, no network — in the
render path. It reads a JSON payload on stdin and prints one or two lines.
`update` is the one command that reaches the network, and only when you run it.

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
(override with `PREFIX`).

`init` writes `~/.config/cc-statusline/config.toml` from a commented default,
points Claude Code's `statusLine` at this binary by absolute path, and shows
you a preview before it changes anything.

```sh
cc-statusline init --dry-run        # show what would change, touch nothing
cc-statusline init --preset minimal # one line instead of two
cc-statusline uninstall             # remove the statusLine key, nothing else
```

It is idempotent — running it twice makes no second backup and changes no
bytes — and it backs up `settings.json` before touching it. If that file has
comments or a trailing comma in it, `init` **declines to edit it** and prints
the block for you to paste in yourself; see
[Why it refuses a settings.json with comments](#why-init-refuses-a-settingsjson-with-comments).

### Updating

```sh
cc-statusline update           # check GitHub for a newer release
cc-statusline update --force   # download it, verify its checksum, install it
```

Same checksum verification as `install.sh`: it refuses to install a release
whose `checksums.txt` doesn't account for the archive it downloaded.

---

## When something looks wrong

```sh
cc-statusline doctor
```

Reports what this build actually saw: whether the status line is wired up and
to which binary, which config file was read and what it could not use, the
resolved capabilities (colour, icons, Powerline) and why each was chosen, the
last payload Claude Code sent, and anything the last render had to repair.
`--json` for scripting.

---

## Configuration

Everything lives in `~/.config/cc-statusline/config.toml`. The installed file
is fully commented; the two shipped presets are
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

Invalid values never break the status line — each is repaired to its default,
the render proceeds, and `doctor` tells you what was repaired.

### Edit it while watching the result

```sh
cc-statusline config
```

A two-pane editor: the segment list on the left, your status line on the
right, re-rendered on every keystroke.

| Key | Does |
|---|---|
| `space` | enable / disable the highlighted segment |
| `tab` / `shift+tab` | cycle its format (`$0.85` ↔ `Cost: $0.85`) |
| `+` / `-` | change its drop priority |
| `J` / `K` | move it within its row or to another row |
| `f` | add a flex gap after it, `space` on the gap removes it |
| `<` / `>` | change the previewed width |
| `i` / `p` / `c` | cycle icon set / Powerline / colour profile in the preview |
| `s` | save the config file |
| `ctrl+s` | save **and install** — points Claude Code at this binary too |

The box above the preview explains whatever the cursor is on: what the
segment shows, why it is or isn't on the line right now, and which config keys
control the parts the wizard doesn't expose (colours, formats, thresholds).

At the foot of the sidebar, **`Presets ›`** lets you preview and apply either
shipped preset before committing to it. Nothing is written until you press
`s`.

Saves patch your file in place — every comment you've written survives.

### Rate limits, one window at a time

`ratelimits` shows both windows in one segment (`5h:15% 7d:8%`). Use
`ratelimit_5h` and `ratelimit_7d` instead if you want them on separate rows, at
separate priorities, or pushed to opposite ends of the line:

```toml
[[line]]
segments = [
  {name = "model", drop = 99}, {name = "ratelimit_5h", drop = 99},
  {name = "flex"},
  {name = "ratelimit_7d", drop = 3},
]
```

Each window has four formats (cycle with `tab` in the wizard):

```
5h:15%      5h:15% ↻ 23:10      Session: 15%      Session: 15% resets 23:10
```

The reset clock is your local time and only appears when you put `{reset}` in
the format:

```toml
[segments.ratelimit_5h]
format       = "5h:{n}%{icon}{reset}"
reset_format = "3:04 PM"   # a Go time layout
```

### Compact or spelled out

Most segments ship a second format that says the same thing in words:

| Segment | Compact | Spelled out |
|---|---|---|
| `model` | `◆ Claude Opus 4.6` | `Model: Claude Opus 4.6` |
| `context` | `▓▓▓▓░░░░░░ 42%` | `Ctx: ▓▓▓▓░░░░░░ 42%` |
| `cost` | `$0.85` | `Cost: $0.85` |
| `duration` | `1h5m` | `Time: 1h5m` |
| `diffstat` | `+150/-30` | `Diff: +150/-30` |
| `project` | `api-server` | `Project: api-server` |
| `effort` | `Effort: high` | `high` |

`tab` in `cc-statusline config` walks between them, and whichever one is on
screen when you save is written into your config as a plain string — you can
also just type your own.

`branch` has no second format; `i` (icon set) changes the glyph in front of it
instead.

### Pushing segments to the right

`{name = "flex"}` isn't a segment, it's a gap: it absorbs whatever width the
row didn't use, so everything after it lands on the right edge.

```toml
[[line]]
segments = [
  {name = "model", drop = 99}, {name = "context", drop = 99},
  {name = "flex"},
  {name = "cost", drop = 4}, {name = "duration", drop = 5},
]
```

```
◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42%                        $0.85 │ 3m
```

Two flex gaps split the leftover width evenly; one at the head of the list
right-aligns the whole row.

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

### Not rendering right in your terminal?

```sh
cc-statusline preview --matrix
```

Renders every capability combination (ASCII / Unicode / Nerd Font / Powerline)
with a width rule under each line, so you can see exactly which glyphs your
terminal draws wider or narrower than expected. `[general] ambiguous_width` is
the escape hatch for terminals that render East-Asian-ambiguous characters
differently than assumed.

---

## Why `init` refuses a `settings.json` with comments

Claude Code's `settings.json` is JSON, but people put `//` comments in it
anyway. This matters because a JSON editor that preserves every other byte in
the file cannot tell a comment from data — given

```jsonc
{
  // "statusLine": {"type": "command", "command": "disabled-on-purpose"},
  "theme": "dark"
}
```

an automatic edit would rewrite the value *inside the comment*, leaving no
live `statusLine` key at all while reporting success. So `init` checks whether
the file is plain JSON before reading anything out of it, and if it isn't, it
prints the block for you to paste in yourself and changes nothing.

---

## Deliberately not included

- **A git dirty flag.** It needs subprocess spawning, a cache, and a lock
  protocol to render one asterisk — the largest subsystem for the least value.
- **A format-string DSL.** `{name}` substitution, and nothing else.
- **Transcript parsing or usage-API calls.** Everything shown comes from the
  payload Claude Code already sends.
- **Windows.** Git discovery, XDG paths, and the install prefix each need an
  answer nobody has written yet.

---

## Development

```sh
make check      # gofmt, vet, tests
make build      # ./bin/cc-statusline
make install    # into ~/.local/bin
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for adding a segment or a preset, and
[`docs/PRD.md`](docs/PRD.md) for the full design reference. Open items are
tracked in [`TODOS.md`](TODOS.md); history of what changed and why is in
[`CHANGELOG.md`](CHANGELOG.md).

## License

MIT — see [`LICENSE`](LICENSE).

## Prior art

[`felipeelias/claude-statusline`](https://github.com/felipeelias/claude-statusline)
is the closest thing and is a good, small tool. This one exists because of the
context bar, the width model, and the capability degradation — if you do not
want those, prefer the smaller program.
