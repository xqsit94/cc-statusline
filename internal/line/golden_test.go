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

var update = flag.Bool("update", false, "rewrite the golden files")

var (
	goldenIcons  = []string{"ascii", "unicode", "nerdfont"}
	goldenSeps   = []string{"plain", "powerline"}
	goldenWidths = []int{40, 80, 120, 200}
)

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
	fmt.Fprintf(&b, "--- unicode plain 80 LANG=ja_JP.UTF-8\n")
	for _, l := range RenderPlain(goldenContext(t, st, "unicode", "plain", 80, "2")) {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

func goldenContext(t testing.TB, st refstate.State, icons, sep string, cols int, ambiguous string) Context {
	t.Helper()

	p, _ := payload.Parse(st.Payload)

	env := map[string]string{
		"COLUMNS": fmt.Sprint(cols),
		"TERM":    "xterm-256color",
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

	cfg := config.Defaults()
	cfg.General.AmbiguousWidth = config.Flexible(ambiguous)

	return Context{
		Payload: p,
		Config:  cfg,
		Git:     st.Git,
		Style:   style.NewStyle(style.Detect(env, cfg), cfg),
	}
}

func TestStyledGoldens(t *testing.T) {
	for _, st := range refstate.References() {
		t.Run(st.Name, func(t *testing.T) {
			var b strings.Builder
			for _, icons := range goldenIcons {
				fmt.Fprintf(&b, "--- %s truecolor 120\n", icons)
				for _, l := range Render(styledContext(t, st, icons, "plain", 120)) {
					fmt.Fprintf(&b, "%q\n", l)
				}
			}
			fmt.Fprintf(&b, "--- nerdfont powerline truecolor 40\n")
			for _, l := range Render(styledContext(t, st, "nerdfont", "powerline", 40)) {
				fmt.Fprintf(&b, "%q\n", l)
			}
			got := b.String()

			path := filepath.Join("testdata", "styled", st.Name+".txt")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v\n\nrun `make golden` to create it", err)
			}
			if got != string(want) {
				t.Errorf("styled golden mismatch for %s\n\n%s",
					st.Name, firstDifference(string(want), got))
			}
		})
	}
}

func TestEveryStyledLineIsBalanced(t *testing.T) {
	for _, st := range refstate.All() {
		for _, cols := range []int{20, 40, 60, 80, 120, 200} {
			for _, sep := range goldenSeps {
				t.Run(fmt.Sprintf("%s/%s/%d", st.Name, sep, cols), func(t *testing.T) {
					for i, l := range Render(styledContext(t, st, "nerdfont", sep, cols)) {
						opens := strings.Count(l, "\x1b[38;2;")
						resets := strings.Count(l, "\x1b[0m")
						if opens != resets {
							t.Errorf("line %d opens %d colours and closes %d:\n%q",
								i+1, opens, resets, l)
						}
						if opens > 0 && !strings.HasSuffix(l, "\x1b[0m") {
							t.Errorf("line %d does not end reset; the colour bleeds into the next prompt:\n%q",
								i+1, l)
						}
					}
				})
			}
		}
	}
}

func styledContext(t testing.TB, st refstate.State, icons, sep string, cols int) Context {
	t.Helper()
	ctx := goldenContext(t, st, icons, sep, cols, "1")

	env := map[string]string{
		"COLUMNS":   fmt.Sprint(cols),
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
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
	ctx.Style = style.NewStyle(style.Detect(env, ctx.Config), ctx.Config)
	return ctx
}

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
