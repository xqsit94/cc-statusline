package cmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

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

func writeBlock(w io.Writer, st refstate.State, cfg *config.Config, caps style.Capabilities, opt blockOpts) {
	p, _ := payload.Parse(st.Payload)
	ctx := line.Context{
		Payload: p,
		Config:  cfg,
		Git:     st.Git,
		Style:   style.NewStyle(caps, cfg),
		Zone:    time.Local,
	}
	avail := line.Available(ctx)

	if opt.header {
		fmt.Fprintf(w, "\n--- %s · %s · available=%d\n", st.Name, st.Desc, avail)
	}
	if opt.ruler {
		fmt.Fprintln(w, columnRuler(avail))
	}

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

func widthRule(n int) string { return style.Rule(n) }

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

func runProbe(env map[string]string, stdin io.Reader, stdout io.Writer, width int) int {
	io.Copy(io.Discard, stdin)

	cfg := config.Defaults()
	cols := style.Detect(previewEnv(env, "", "", width), cfg).Columns

	fmt.Fprintln(stdout, tensLabels(cols))
	fmt.Fprintln(stdout, columnRuler(cols))
	return 0
}

func previewConfig(ambiguous int) *config.Config {
	cfg := config.Defaults()
	if ambiguous == 1 || ambiguous == 2 {
		cfg.General.AmbiguousWidth = config.Flexible(strconv.Itoa(ambiguous))
	}
	return cfg
}

func previewEnv(env map[string]string, icons, sep string, cols int) map[string]string {
	return style.Overlay(env, style.Overrides{Icons: icons, Separator: sep, Columns: cols})
}

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
