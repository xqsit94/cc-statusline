package line

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// Tier-1 goldens: layout only, no colour (PRD §9.2).
//
// Colour is stored separately because layout does not change when a colour
// does. A one-hex theme edit used to rewrite every golden in a project this
// shape, producing a diff nobody could review — so the axes here are the ones
// that actually vary in unstyled output, and the colour axis is deliberately
// not one of them.
//
// Regenerate with `go test ./internal/line -update`. Read the diff before
// committing it: a golden that changed for a reason you cannot name is a bug
// you have just accepted.

var update = flag.Bool("update", false, "rewrite the golden files")

// The matrix. Widths bracket the interesting cases: 40 forces stage 2 and
// stage 3 on every fixture, 80 is the default when COLUMNS is unset, 120 is a
// normal modern terminal, and 200 is wide enough that nothing fits.
var (
	goldenIcons  = []string{"ascii", "unicode", "nerdfont"}
	goldenSeps   = []string{"plain", "powerline"}
	goldenWidths = []int{40, 80, 120, 200}
)

// The payloads come from internal/refstate, not from a testdata directory
// beside this file. §9.4's visual gate renders the same four states through
// `cc-statusline preview`, and a human signing off on a payload that is not the
// one these goldens assert against would be signing off on nothing.
//
// Git travels with them, injected rather than discovered, so the goldens are
// hermetic: reading the real .git would make every golden depend on which
// branch the developer happens to be standing on.

func TestPlainGoldens(t *testing.T) {
	for _, st := range refstate.All() {
		t.Run(st.Name, func(t *testing.T) {
			got := renderMatrix(t, st)
			path := filepath.Join("testdata", "golden", st.Name+".txt")

			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\n\nrun `go test ./internal/line -update` to create it", err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch for %s; run with -update and read the diff\n\n%s",
					st.Name, firstDifference(string(want), got))
			}
		})
	}
}

// TestNoLineExceedsAvailable is PRD §9.3's width criterion, and it is the
// assertion that actually has teeth.
//
// The goldens above record what the fitter produced; this records what it was
// required to produce. A golden can be regenerated to match a bug — that is
// what `-update` is for — and this test is what stops the regeneration from
// hiding one. It runs at COLUMNS=10 as well, which no golden covers, because
// the floor in `available` is the only thing standing between a 10-column
// terminal and a negative budget.
func TestNoLineExceedsAvailable(t *testing.T) {
	for _, st := range refstate.All() {
		for _, icons := range goldenIcons {
			for _, cols := range []int{10, 20, 40, 60, 80, 120, 200} {
				for _, ambiguous := range []string{"1", "2"} {
					t.Run(fmt.Sprintf("%s/%s/%d/amb%s", st.Name, icons, cols, ambiguous), func(t *testing.T) {
						ctx := goldenContext(t, st, icons, "powerline", cols, ambiguous)
						avail := available(ctx)
						for i, l := range RenderPlain(ctx) {
							if w := ctx.Style.Width(l); w > avail {
								t.Errorf("line %d is %d cells, available is %d:\n%q",
									i+1, w, avail, l)
							}
						}
					})
				}
			}
		}
	}
}

func renderMatrix(t *testing.T, st refstate.State) string {
	t.Helper()
	var b strings.Builder
	for _, icons := range goldenIcons {
		for _, sep := range goldenSeps {
			for _, w := range goldenWidths {
				fmt.Fprintf(&b, "--- %s %s %d\n", icons, sep, w)
				for _, l := range RenderPlain(goldenContext(t, st, icons, sep, w, "1")) {
					b.WriteString(l)
					b.WriteByte('\n')
				}
			}
		}
	}
	// §9.2 requires one CJK row: `▓ ░ ◆ ⚠` are East Asian Ambiguous and occupy
	// two cells under a CJK locale, so this is the only row where the width
	// arithmetic and the glyph count disagree.
	fmt.Fprintf(&b, "--- unicode plain 80 LANG=ja_JP.UTF-8\n")
	for _, l := range RenderPlain(goldenContext(t, st, "unicode", "plain", 80, "2")) {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

func goldenContext(t *testing.T, st refstate.State, icons, sep string, cols int, ambiguous string) Context {
	t.Helper()

	p, _ := payload.Parse(st.Payload)

	env := map[string]string{
		"COLUMNS": fmt.Sprint(cols),
		// TERM and COLORTERM are pinned rather than inherited: a developer
		// running the suite inside a truecolor terminal must get the same
		// bytes as CI running it inside a pipe.
		"TERM": "xterm-256color",
	}
	switch icons {
	case "ascii":
		env["CC_STATUSLINE_ASCII"] = "1"
	case "nerdfont":
		env["CC_STATUSLINE_NERDFONT"] = "1"
	}
	if sep == "powerline" {
		env["CC_STATUSLINE_POWERLINE"] = "1"
	}

	// The embedded defaults, not config.Load: PRD §5.1 calls the reference
	// states "acceptance criteria for the default preset", and reading the
	// developer's own ~/.config would make them criteria for whatever that
	// happens to contain. config.Load is tested where it belongs.
	cfg := config.Defaults()
	cfg.General.AmbiguousWidth = config.Flexible(ambiguous)

	return Context{
		Payload: p,
		Config:  cfg,
		Git:     st.Git,
		Style:   style.NewStyle(style.Detect(env, cfg), cfg),
	}
}

// TestNoOrphanGoldens catches the other direction of the move to refstate: a
// golden file whose fixture no longer exists is never compared against
// anything, so it sits in the repository looking like coverage.
func TestNoOrphanGoldens(t *testing.T) {
	known := map[string]bool{}
	for _, st := range refstate.All() {
		known[st.Name] = true
	}
	entries, err := filepath.Glob(filepath.Join("testdata", "golden", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := strings.TrimSuffix(filepath.Base(e), ".txt")
		if !known[name] {
			t.Errorf("%s has no fixture in internal/refstate; delete it or restore the payload", e)
		}
	}
	if len(entries) != len(known) {
		t.Errorf("%d goldens, %d fixtures", len(entries), len(known))
	}
}

// firstDifference reports the first differing line, so a failure names one line
// rather than printing two hundred.
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		lw, lg := "", ""
		if i < len(w) {
			lw = w[i]
		}
		if i < len(g) {
			lg = g[i]
		}
		if lw != lg {
			return fmt.Sprintf("first difference at line %d:\nwant: %q\n got: %q", i+1, lw, lg)
		}
	}
	return "(files differ but no line does; check the trailing newline)"
}
