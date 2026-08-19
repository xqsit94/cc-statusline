# Contributing

`make check` before every commit — `gofmt -l`, `go vet`, `go test ./...`. It is
the whole gate; if it is green the change is mergeable on mechanics.

```sh
make check                                  # the gate
make build                                  # -> bin/cc-statusline
go test ./internal/line/ -run TestX -count=1 -v   # one test
```

`-count=1` matters. Without it Go serves cached results, and a "passing" run may
have executed nothing.

`README.md` explains what the thing does; `docs/PRD.md` is the design reference
and `CLAUDE.md` is the architecture tour with the invariants that are easy to
break. This file is how to add the two things people most often want to add.

## Adding a segment

Three files are required. A fourth, fifth and sixth apply only if the segment
needs something new.

### 1. Declare it — `internal/config/schema.go`

Add a `SegmentDef` to `SegmentDefs`, in the position you want it to occupy in
the wizard's sidebar. This is the whole interface: the TOML keys, the
placeholders each accepts, the type of value behind each, the colour that paints
it, and the ring `tab` cycles.

```go
{
    Name: "effort",
    Doc:  "the reasoning effort this session is set to, from the payload's " +
        "effort.level. Absent on a model that has no effort setting.",
    Keys: []Key{{
        Name:    "format",
        Syntax:  SyntaxPlaceholders,
        Fields:  []Field{{Name: "level", Kind: KindText, Color: "effort"}},
        Default: "Effort: {level}",
        Get:     func(c *Config) string { return c.Segments.Effort.Format },
        Set:     func(c *Config, v string) { c.Segments.Effort.Format = v },
    }},
    Presentations: []Presentation{
        {"Effort: {level}"},
        {"{level}"},
    },
},
```

`SegmentNames`, `FormatKeys`, `TimeKeys`, `ColorKeys`, `Variants`, the defaults
and the wizard's hint are all read back out of this. There is no second place to
register a name.

Things the declaration decides:

- **`Syntax`** picks the validator. `SyntaxPlaceholders` is `{name}` grammar;
  `SyntaxTimeLayout` is a Go reference-time layout, checked by whether it
  substitutes anything at all; `SyntaxOpaque` is a key with no grammar — a
  separator, a bool — which is declared so the wizard's hint can name it, and
  which carries no accessor if it is not a string.
- **`Kind`** is the type of the value, and for three of them it is a claim
  about what reaches the screen: `KindMoney` is two decimal places,
  `KindPercent` and `KindCount` are bare integers. Nothing dispatches on it —
  your `Render` calls `money` or `percent` itself — but
  `TestEveryFieldRendersItsDeclaredKind` renders each field alone and checks the
  output against the declaration, so a `Kind` you do not honour fails. The rest
  (`KindText`, `KindGlyph`, `KindGauge`, `KindClock`) claim no shape on purpose:
  text is printed exactly as it arrived, including values added to the payload
  schema after your build shipped.
- **`Color` / `Band`** — a flat `[colors]` key, or a threshold rule that picks
  one. Every field needs one or the other.
- **`Presentations`** is positional over the segment's non-opaque keys, in
  declaration order, and every entry must assign all of them. The first must be
  what your `Default`s say, or a fresh config sits in the middle of the ring.

### 2. Give it a config key — `internal/config/config.go`

One field on `Segments`. `FormatSeg` if it has a single `format`; a new struct
if it has more.

```go
Effort FormatSeg `toml:"effort"`
```

This stays hand-written on purpose. A `map[string]string` would remove the line
and turn `Segments.Duration.OverHour` into a string literal — a typo becoming a
silently empty format instead of a compile error.

### 3. Render it — `internal/line/segments.go`

A type with `Name`, `Render`, and an `init` that registers it. Optionally
`Truncate`, if there is something it can usefully give up under a narrow
terminal.

```go
type effortSegment struct{}

func (effortSegment) Name() string { return "effort" }

func init() { register("effort", func() Segment { return effortSegment{} }) }

func (effortSegment) Render(ctx Context) Rendered {
    level, ok := ctx.Payload.EffortLevel()
    if !ok || strings.TrimSpace(level) == "" {
        return Rendered{}
    }
    st := ctx.Style
    return expand(st, ctx.Config.Segments.Effort.Format, "effort", map[string]part{
        "level": text(st, "effort", level),
    })
}
```

Two rules here are load-bearing, and both still compile when broken:

- **Segments perform no I/O and resolve none of their own inputs.** Everything
  arrives already resolved in `line.Context`, including `Zone`. A segment that
  could read a file could block the render; one that reads `time.Local` for
  itself is an input no test and no preview can vary.
- **Empty means absent.** A zero `Rendered` omits the segment *and* its adjacent
  separator, rather than leaving a gap where data used to be.

### 4. If it needs a new colour — `internal/config/schema.go` and `internal/style/escape_test.go`

A row in `ColorDefs` with its default, a field on the `Colors` struct, and — if
the hex is one the palette does not already use — an entry in `escapeByHex`
plus one in `defaultColors`. Generate the escapes by running the code and
reading them back; do not type them.

Reusing an existing colour needs none of this.

### 5. If it needs data the payload does not expose yet — `internal/payload/`

A field on the struct and an accessor returning `(value, ok)`. Every field in
the payload is optional (§3.1) and absent is normal, not an error — the accessor
distinguishes absent from empty so the segment can decide which one means
"render nothing".

### 6. Document it — `config/default.toml`

A commented `[segments.<name>]` block. `default.toml` is documentation that
happens to be executable, and `TestDefaultPresetMatchesDefaults` asserts it
decodes to exactly the embedded defaults — so every value in it must be the
default.

### What you get for free

Validation of the format against its placeholders, the `tab` ring, the wizard
sidebar row and hint block, `doctor` coverage, the disabled pool, and a set of
contract tests that will fail if any two halves disagree.

### Should it be on a shipped preset?

Probably not, at first. A segment nobody has to accept is cheaper to be wrong
about than a row of the default, and adding one to a preset moves goldens.

## Adding a preset

Drop a `.toml` into `config/`. That is the whole procedure — it appears in
`Names()`, in `init --preset`'s help, in the wizard's picker, and in
`TestEveryPresetLoadsCleanly` as its own subtest, with no Go touched.

The first comment line becomes the picker's description, less the
`# cc-statusline — ` it opens with.

## House rules

**Verify a new test fails against the unfixed code before accepting it.** Revert
the fix, run the test, confirm it reports the symptom you set out to catch, then
restore. A test written after the fix and never seen red is a test of nothing.
Say so in the commit.

**PRD section numbers are load-bearing.** Roughly 480 `§` citations across the
Go files point into `docs/PRD.md`. Never renumber. When you change behaviour the
PRD specifies, update the PRD; when the implementation corrected the PRD, add a
`> **Corrected at M7 — …**` blockquote under the section saying what it did not
say and why, rather than silently rewriting it.

**Comments say why, not what.** The density is deliberate — a comment naming the
failure mode a line prevents is the point; one restating the line below it is
noise.

**Goldens are read, not regenerated.** `make golden` rewrites §9.2 tiers 1 and
3. Read the diff before committing it: a golden regenerated to make a red test
green is a bug written down and agreed to.
