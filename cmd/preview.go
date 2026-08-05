package cmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// Preview implements `cc-statusline preview`: the harness for PRD §9.4's manual
// visual gate.
//
// # Why a subcommand exists for something a human does
//
// §9.4 is the one gate no test can stand in for. The goldens measure with the
// same go-runewidth the renderer uses, so they can prove the arithmetic is
// self-consistent and never that a terminal agrees with it; and nothing in Go
// can tell whether a Nerd Font glyph rendered or came out as a replacement box.
// Someone has to look.
//
// What that person needed, before this existed, was to hand-assemble twenty-odd
// invocations of `render` with a fixture on stdin and four environment variables
// in front of it, then judge alignment by eye against nothing. A gate that
// expensive is a gate that gets skipped, and a gate judged against nothing
// records an opinion rather than a measurement.
//
// So this does two things the render path cannot:
//
//	It renders capability sets this terminal does not have. Capabilities are a
//	pure function of an environment map (§6.4), which is exactly what lets a
//	Nerd Font preview run in a terminal with no patched font — the same property
//	M7's wizard is built on (§10.4).
//
//	It draws a width rule under every line: `|--- 62 ---|`, exactly 62 cells of
//	ASCII. ASCII is one cell wide in every terminal there is, so the rule is a
//	ruler this build cannot be wrong about. If a status line ends past its own
//	rule, the terminal and go-runewidth disagree — which is the single bug §9.4
//	exists to catch, now visible at a glance and preserved by a screenshot.
//
// It renders the embedded defaults and never reads ~/.config. §5.1 calls the
// reference states "acceptance criteria for the default preset", and a gate run
// against whatever the developer's config happens to contain is a gate on
// nothing.
func Preview(args []string, env map[string]string, stdin io.Reader, stdout io.Writer) int {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	fs.SetOutput(stdout)
	var (
		state     = fs.String("state", "", "render one fixture by name (default: §5.1's four reference states)")
		width     = fs.Int("width", 0, "render at N columns (default: COLUMNS, else 80)")
		icons     = fs.String("icons", "", "ascii | unicode | nerdfont (default: detected)")
		sep       = fs.String("sep", "", "plain | powerline (default: detected)")
		ambiguous = fs.Int("ambiguous", 0, "1 or 2 cells for East Asian Ambiguous glyphs (default: from LANG)")
		matrix    = fs.Bool("matrix", false, "every capability set §9.4 names, at one width")
		probe     = fs.Bool("probe", false, "print a column ruler instead of a status line (measures C-7)")
		plain     = fs.Bool("plain", false, "no colour, whatever the terminal supports")
		ruler     = fs.Bool("ruler", false, "print a column ruler above each block")
		bare      = fs.Bool("bare", false, "lines only: no header, no rules, no ruler")
	)
	fs.Usage = func() {
		fmt.Fprint(stdout, previewUsage)
		fs.PrintDefaults()
		fmt.Fprintf(stdout, "\nfixtures: %s\n", strings.Join(refstate.Names(), ", "))
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// The probe is a different program that happens to live behind the same
	// name: it prints no status line at all. See probeRuler.
	if *probe {
		return runProbe(env, stdin, stdout, *width)
	}

	if err := checkChoice("--icons", *icons, "ascii", "unicode", "nerdfont"); err != nil {
		fmt.Fprintln(stdout, err)
		return 2
	}
	if err := checkChoice("--sep", *sep, "plain", "powerline"); err != nil {
		fmt.Fprintln(stdout, err)
		return 2
	}
	if *ambiguous != 0 && *ambiguous != 1 && *ambiguous != 2 {
		fmt.Fprintln(stdout, "cc-statusline: --ambiguous takes 1 or 2")
		return 2
	}

	states := refstate.References()
	if *state != "" {
		st, ok := refstate.ByName(*state)
		if !ok {
			fmt.Fprintf(stdout, "cc-statusline: no fixture named %q; have %s\n",
				*state, strings.Join(refstate.Names(), ", "))
			return 2
		}
		states = []refstate.State{st}
	}

	opt := blockOpts{colour: !*plain, rules: !*bare, header: !*bare, ruler: *ruler && !*bare}

	// One capability set, or the four §9.4 names.
	sets := []capSet{{icons: *icons, sep: *sep}}
	if *matrix {
		sets = gateCapSets
	}

	if !*bare {
		writeHeader(stdout, env, sets[0], *ambiguous, *width, *matrix)
	}
	for _, cs := range sets {
		cfg := previewConfig(*ambiguous)
		caps := style.Detect(previewEnv(env, cs.icons, cs.sep, *width), cfg)
		if *matrix && !*bare {
			fmt.Fprintf(stdout, "\n=== %s %s\n", cs.label(), strings.Repeat("=", max(0, 62-len(cs.label()))))
		}
		for _, st := range states {
			writeBlock(stdout, st, cfg, caps, opt)
		}
	}
	return 0
}

const previewUsage = `cc-statusline preview — the harness for PRD §9.4's manual visual gate

Renders the reference states against capability sets this terminal may not have,
with a width rule under each line. Reads the embedded defaults, never ~/.config.

`

// capSet is one column of §6.2's glyph table plus a separator style.
type capSet struct{ icons, sep string }

func (c capSet) label() string {
	i, s := c.icons, c.sep
	if i == "" {
		i = "detected"
	}
	if s == "" {
		s = "detected"
	}
	return i + " · " + s
}

// gateCapSets is the four capability sets §9.4 names. Powerline is not a fifth
// icon column — it is the Nerd Font column with arrow separators, because
// §6.1's resolution makes Powerline follow the icon set and §6.2 has no
// Powerline glyph that a non-patched font can draw.
var gateCapSets = []capSet{
	{icons: "ascii", sep: "plain"},
	{icons: "unicode", sep: "plain"},
	{icons: "nerdfont", sep: "plain"},
	{icons: "nerdfont", sep: "powerline"},
}

type blockOpts struct {
	colour bool
	rules  bool
	header bool
	ruler  bool
}

// writeBlock renders one fixture and, under each line, the rule that makes the
// gate a measurement.
func writeBlock(w io.Writer, st refstate.State, cfg *config.Config, caps style.Capabilities, opt blockOpts) {
	p, _ := payload.Parse(st.Payload)
	ctx := line.Context{
		Payload: p,
		Config:  cfg,
		Git:     st.Git,
		Style:   style.NewStyle(caps, cfg),
	}
	avail := line.Available(ctx)

	if opt.header {
		fmt.Fprintf(w, "\n--- %s · %s · available=%d\n", st.Name, st.Desc, avail)
	}
	if opt.ruler {
		fmt.Fprintln(w, columnRuler(avail))
	}

	// Both are rendered: RenderPlain is what the width is measured from, and
	// measuring the styled string instead would count escape sequences as
	// content. §5.6 prohibits ANSI-stripping at measure time for the same
	// reason — the plain text is kept, not reconstructed.
	plainLines := line.RenderPlain(ctx)
	shown := plainLines
	if opt.colour {
		shown = line.Render(ctx)
	}
	for i, l := range shown {
		fmt.Fprintln(w, l)
		if opt.rules && i < len(plainLines) {
			fmt.Fprintln(w, widthRule(ctx.Style.Width(plainLines[i])))
		}
	}
}

// widthRule draws exactly n cells of ASCII with n written in the middle.
//
// Every character it uses — `|`, `-`, a digit, a space — is East Asian Narrow,
// so the rule occupies n columns in a CJK locale and n columns outside one and
// n columns in a terminal that has its own opinion about ambiguous glyphs. That
// is the entire point: the rule is the one line on screen whose width this
// program cannot be wrong about, so any disagreement it reveals is the status
// line's.
func widthRule(n int) string {
	switch {
	case n <= 0:
		return ""
	case n <= 4:
		return strings.Repeat("-", n)
	}
	label := " " + strconv.Itoa(n) + " "
	inner := n - 2
	if len(label) > inner {
		return "|" + strings.Repeat("-", inner) + "|"
	}
	left := (inner - len(label)) / 2
	right := inner - len(label) - left
	return "|" + strings.Repeat("-", left) + label + strings.Repeat("-", right) + "|"
}

// columnRuler is the familiar `----+----1----+----2`, n cells wide: a `+` every
// five columns, the tens digit every ten. ASCII, for the reason widthRule is.
func columnRuler(n int) string {
	var b strings.Builder
	b.Grow(n)
	for i := 1; i <= n; i++ {
		switch {
		case i%10 == 0:
			b.WriteByte(byte('0' + (i/10)%10))
		case i%5 == 0:
			b.WriteByte('+')
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// tensLabels writes the full column number ending at each multiple of ten, so
// that a ruler wider than a hundred columns can still be read off. `120` sits
// in columns 118-120.
func tensLabels(n int) string {
	buf := []byte(strings.Repeat(" ", n))
	for k := 10; k <= n; k += 10 {
		s := strconv.Itoa(k)
		if k-len(s) < 0 {
			continue
		}
		copy(buf[k-len(s):k], s)
	}
	return string(buf)
}

// runProbe is C-7, turned from an argument into a measurement.
//
// §5.6 reserves twelve columns on the right of the status line "because Claude
// Code renders system notifications there". Nobody measured twelve. It became
// load-bearing at M3 — it is the two cells that stop §5.1's danger state from
// fitting at eighty columns — and it is still a number somebody guessed.
//
// It cannot be measured from inside this process: Claude Code captures stdout,
// so there is no terminal to interrogate and no ioctl that would answer. What
// can be done is to print a ruler exactly COLUMNS wide and let Claude Code draw
// it. Whatever covers the right-hand end of that ruler is the thing the reserve
// exists to avoid, and the column where the ruler stops being readable gives
// the number directly. docs/M4-visual-gate.md has the four commands.
//
// Stdin is drained rather than ignored. Claude Code writes the payload to this
// process; exiting without reading it hands the writer an EPIPE for a status
// line that is otherwise working perfectly.
func runProbe(env map[string]string, stdin io.Reader, stdout io.Writer, width int) int {
	io.Copy(io.Discard, stdin)

	cfg := config.Defaults()
	cols := style.Detect(previewEnv(env, "", "", width), cfg).Columns

	fmt.Fprintln(stdout, tensLabels(cols))
	fmt.Fprintln(stdout, columnRuler(cols))
	return 0
}

// previewConfig is the embedded defaults, with the ambiguous-width override
// applied. §5.1's criteria are for the default preset; reading the developer's
// own config would make the gate a gate on that file instead.
func previewConfig(ambiguous int) *config.Config {
	cfg := config.Defaults()
	if ambiguous == 1 || ambiguous == 2 {
		cfg.General.AmbiguousWidth = config.Flexible(strconv.Itoa(ambiguous))
	}
	return cfg
}

// previewEnv overlays the flags onto a copy of the environment and lets
// style.Detect resolve from there.
//
// Overriding the resolved Capabilities directly would be shorter and would
// quietly fork §6.1's precedence — ASCII beating NERDFONT, Powerline refusing to
// turn on under ASCII — into a second implementation that nothing tests. Going
// back through the environment means the preview resolves capabilities by
// exactly the rules the render path does, which is the property that makes the
// gate transferable to what the user will actually see.
func previewEnv(env map[string]string, icons, sep string, cols int) map[string]string {
	e := make(map[string]string, len(env)+4)
	for k, v := range env {
		e[k] = v
	}
	switch icons {
	case "ascii":
		e["CC_STATUSLINE_ASCII"] = "1"
		delete(e, "CC_STATUSLINE_NERDFONT")
	case "unicode":
		delete(e, "CC_STATUSLINE_ASCII")
		delete(e, "CC_STATUSLINE_NERDFONT")
	case "nerdfont":
		delete(e, "CC_STATUSLINE_ASCII")
		e["CC_STATUSLINE_NERDFONT"] = "1"
	}
	switch sep {
	case "plain":
		e["CC_STATUSLINE_POWERLINE"] = "0"
	case "powerline":
		e["CC_STATUSLINE_POWERLINE"] = "1"
	}
	if cols > 0 {
		e["COLUMNS"] = strconv.Itoa(cols)
	}
	return e
}

// writeHeader records the environment the gate ran in, so a screenshot of the
// output is a screenshot of its own provenance. A screenshot that does not say
// which locale and which colour profile produced it settles nothing later.
func writeHeader(w io.Writer, env map[string]string, cs capSet, ambiguous, width int, matrix bool) {
	cfg := previewConfig(ambiguous)
	caps := style.Detect(previewEnv(env, cs.icons, cs.sep, width), cfg)

	fmt.Fprint(w, previewUsage[:strings.Index(previewUsage, "\n")], "\n\n")
	fmt.Fprintf(w, "  terminal   TERM=%s COLORTERM=%s LANG=%s COLUMNS=%s\n",
		orDash(env["TERM"]), orDash(env["COLORTERM"]), orDash(localeOf(env)), orDash(env["COLUMNS"]))
	if matrix {
		fmt.Fprintf(w, "  resolved   ambiguous=%d columns=%d colour=%s (icons and separator vary below)\n",
			caps.Ambiguous, caps.Columns, profileName(caps.Profile))
	} else {
		fmt.Fprintf(w, "  resolved   icons=%s powerline=%v colour=%s ambiguous=%d columns=%d\n",
			caps.Icons, caps.Powerline, profileName(caps.Profile), caps.Ambiguous, caps.Columns)
	}
	fmt.Fprintf(w, "  config     the embedded defaults; ~/.config is deliberately not read\n")
	fmt.Fprintf(w, "  budget     available = %d - 2×%d padding - %d reserve = %d cells\n",
		caps.Columns, cfg.General.Padding, cfg.General.WidthReserve,
		max(20, caps.Columns-2*cfg.General.Padding-cfg.General.WidthReserve))
	fmt.Fprint(w, `
  The rule under each line is exactly as many cells as this build believes the
  line occupies, drawn in ASCII — one column per character in every terminal.
  A status line that ends past its own rule means this terminal disagrees with
  go-runewidth, which is the one failure the goldens cannot see (§9.4).
`)
}

// localeOf mirrors §6.4's precedence for the display in the header, so that the
// variable printed is the variable that decided the ambiguous width.
func localeOf(env map[string]string) string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := env[k]; v != "" {
			if k == "LANG" {
				return v
			}
			return k + "=" + v
		}
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

// profileName names a termenv.Profile. termenv ships no String method, and the
// constants are an unnamed int sequence in which TrueColor is 0 — so printing
// one directly reads as `colour=0` for the best case and `colour=3` for none.
func profileName(p termenv.Profile) string {
	switch p {
	case termenv.TrueColor:
		return "truecolor"
	case termenv.ANSI256:
		return "256"
	case termenv.ANSI:
		return "16"
	case termenv.Ascii:
		return "none"
	default:
		return "unknown"
	}
}

func checkChoice(flagName, got string, allowed ...string) error {
	if got == "" {
		return nil
	}
	for _, a := range allowed {
		if got == a {
			return nil
		}
	}
	return fmt.Errorf("cc-statusline: %s=%q; want one of %s",
		flagName, got, strings.Join(allowed, ", "))
}
