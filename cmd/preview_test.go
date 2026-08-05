package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// PRD §9.4's gate rests on the width rule being trustworthy. These are the
// properties that make it so — and the one property that makes the gate mean
// anything at all, which is that what the human looks at is what the goldens
// assert.

// TestWidthRuleIsExactlyNCells is the rule's entire justification. It is
// offered to the reader as "exactly n cells", and the claim survives only while
// every byte in it is ASCII: one byte, one rune, one column, in every locale
// and every terminal. A single box-drawing character slipped in for looks would
// make the ruler East Asian Ambiguous — the very class of glyph it exists to
// measure — and it would then be wrong in exactly the case the gate is run for.
func TestWidthRuleIsExactlyNCells(t *testing.T) {
	for n := 0; n <= 200; n++ {
		r := widthRule(n)
		if len(r) != n {
			t.Errorf("widthRule(%d) is %d bytes: %q", n, len(r), r)
		}
		for i := 0; i < len(r); i++ {
			if r[i] >= 0x80 {
				t.Fatalf("widthRule(%d) has a non-ASCII byte at %d: %q", n, i, r)
			}
		}
	}
}

func TestColumnRulerIsExactlyNCells(t *testing.T) {
	for n := 0; n <= 200; n++ {
		for name, got := range map[string]string{
			"columnRuler": columnRuler(n),
			"tensLabels":  tensLabels(n),
		} {
			if len(got) != n {
				t.Errorf("%s(%d) is %d bytes: %q", name, n, len(got), got)
			}
			for i := 0; i < len(got); i++ {
				if got[i] >= 0x80 {
					t.Fatalf("%s(%d) has a non-ASCII byte at %d", name, n, i)
				}
			}
		}
	}
}

// TestWidthRuleShowsItsNumber: the rule is read off a screenshot, so the number
// has to be in it. Below six cells there is nowhere to put one, which is why
// widthRule degrades to bare dashes rather than truncating a digit.
func TestWidthRuleShowsItsNumber(t *testing.T) {
	for n := 6; n <= 200; n++ {
		if !strings.Contains(widthRule(n), strconv.Itoa(n)) {
			t.Errorf("widthRule(%d) does not contain %d: %q", n, n, widthRule(n))
		}
	}
}

// TestPreviewShowsWhatTheGoldensAssert is the test that makes the gate mean
// something.
//
// A human looks at `preview` and signs off that the glyphs render, the columns
// line up, and the gradient is legible. That signature transfers to the shipped
// binary only while `preview` and `render` produce the same bytes from the same
// fixture. Nothing else in the suite compares them, and the failure mode is
// silent in the worst possible way: a gate that passes on output nobody ships.
func TestPreviewShowsWhatTheGoldensAssert(t *testing.T) {
	for _, icons := range []string{"ascii", "unicode", "nerdfont"} {
		for _, st := range refstate.All() {
			t.Run(icons+"/"+st.Name, func(t *testing.T) {
				env := map[string]string{"TERM": "xterm-256color"}

				var out bytes.Buffer
				code := Preview([]string{
					"--bare", "--plain", "--width", "120",
					"--icons", icons, "--state", st.Name,
				}, env, strings.NewReader(""), &out)
				if code != 0 {
					t.Fatalf("exit %d\n%s", code, out.String())
				}

				cfg := config.Defaults()
				caps := style.Detect(previewEnv(env, icons, "", 120), cfg)
				p, _ := payload.Parse(st.Payload)
				want := line.RenderPlain(line.Context{
					Payload: p, Config: cfg, Git: st.Git,
					Style: style.NewStyle(caps, cfg),
				})

				got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
				if len(got) != len(want) {
					t.Fatalf("preview printed %d lines, render produced %d\n got: %q\nwant: %q",
						len(got), len(want), got, want)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Errorf("line %d:\n got: %q\nwant: %q", i+1, got[i], want[i])
					}
				}
			})
		}
	}
}

// TestPreviewRuleMatchesItsLine checks that each rule describes the line it
// sits under.
//
// It cannot check the thing the gate is for. Both sides of the comparison
// measure with the same go-runewidth, so a terminal that disagrees with the
// library is invisible here by construction — that is §9.4's whole premise and
// the reason a human is involved at all. What this catches is the plumbing
// failure underneath: a rule drawn from the styled string instead of the plain
// one, or offset by a line, would leave the instrument pointing at the wrong
// thing while still looking exactly like an instrument.
func TestPreviewRuleMatchesItsLine(t *testing.T) {
	for _, ambiguous := range []string{"1", "2"} {
		for _, cs := range gateCapSets {
			t.Run(cs.icons+"/"+cs.sep+"/amb"+ambiguous, func(t *testing.T) {
				env := map[string]string{"TERM": "xterm-256color"}
				var out bytes.Buffer
				code := Preview([]string{
					"--plain", "--width", "200", "--ambiguous", ambiguous,
					"--icons", cs.icons, "--sep", cs.sep,
				}, env, strings.NewReader(""), &out)
				if code != 0 {
					t.Fatalf("exit %d", code)
				}

				cfg := previewConfig(atoiOrZero(ambiguous))
				st := style.NewStyle(style.Detect(previewEnv(env, cs.icons, cs.sep, 200), cfg), cfg)

				lines := strings.Split(out.String(), "\n")
				pairs := 0
				for i := 0; i+1 < len(lines); i++ {
					rule := lines[i+1]
					if !isRule(rule) {
						continue
					}
					pairs++
					if got, want := st.Width(lines[i]), len(rule); got != want {
						t.Errorf("rule says %d cells, Style.Width says %d:\n%s\n%s",
							want, got, lines[i], rule)
					}
				}
				// Without this the loop above passes trivially if the format
				// ever changes and no rule is recognised.
				if pairs < 8 {
					t.Errorf("found %d rules; four states render at least eight lines", pairs)
				}
			})
		}
	}
}

// isRule matches the `|---- 62 ----|` shape without matching a status line that
// happens to contain a pipe.
func isRule(s string) bool {
	if len(s) < 6 || s[0] != '|' || s[len(s)-1] != '|' {
		return false
	}
	for i := 1; i < len(s)-1; i++ {
		switch {
		case s[i] == '-' || s[i] == ' ':
		case s[i] >= '0' && s[i] <= '9':
		default:
			return false
		}
	}
	return true
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// TestPreviewIgnoresTheUserConfig. §5.1's reference states are criteria for the
// default preset. A gate that rendered the developer's own config would be a
// gate on that file — and would pass or fail for reasons the shipped binary
// never reproduces on anyone else's machine.
func TestPreviewIgnoresTheUserConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[general]
icons = "ascii"
separator = " @ "

[[line]]
segments = [{name = "cost", drop = 99}]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The same file, proven to bite when it is read.
	cfg, _ := config.Load(map[string]string{"CC_STATUSLINE_CONFIG": path})
	if len(cfg.Lines) != 1 {
		t.Fatalf("the fixture config does not actually override anything (%d lines)", len(cfg.Lines))
	}

	var out bytes.Buffer
	code := Preview([]string{"--bare", "--plain", "--width", "120", "--state", "normal-42"},
		map[string]string{"CC_STATUSLINE_CONFIG": path, "TERM": "xterm-256color"},
		strings.NewReader(""), &out)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out.String(), "◆ Claude Opus 4.6 │") {
		t.Errorf("preview read the config file:\n%s", out.String())
	}
}

// TestProbeIsExactlyColumnsWide. The probe is a measuring stick that Claude
// Code holds up against its own chrome (C-7). A stick of the wrong length
// yields a wrong number and nobody can tell, because the only way to check it
// is with the thing being measured.
func TestProbeIsExactlyColumnsWide(t *testing.T) {
	for _, cols := range []int{40, 80, 100, 173, 200} {
		var out bytes.Buffer
		code := Preview([]string{"--probe"},
			map[string]string{"COLUMNS": strconv.Itoa(cols)},
			strings.NewReader(""), &out)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("probe printed %d lines, want 2", len(lines))
		}
		for i, l := range lines {
			if len(l) != cols {
				t.Errorf("COLUMNS=%d: probe line %d is %d cells", cols, i+1, len(l))
			}
		}
		// The last label has to be readable, or the ruler cannot be read off.
		if !strings.Contains(lines[0], strconv.Itoa(cols/10*10)) {
			t.Errorf("COLUMNS=%d: no %d label in %q", cols, cols/10*10, lines[0])
		}
	}
}

// TestProbeDrainsStdin. Claude Code writes the payload to this process. Exiting
// without reading it hands the writer an EPIPE for a status line that is
// otherwise working perfectly, and the error surfaces nowhere near the cause.
func TestProbeDrainsStdin(t *testing.T) {
	in := strings.NewReader(strings.Repeat(`{"model":{"display_name":"x"}}`, 500))
	var out bytes.Buffer
	if code := Preview([]string{"--probe"}, map[string]string{"COLUMNS": "80"}, in, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if in.Len() != 0 {
		t.Errorf("%d bytes left unread on stdin", in.Len())
	}
}

func TestPreviewRejectsBadFlags(t *testing.T) {
	cases := [][]string{
		{"--icons", "emoji"},
		{"--sep", "curly"},
		{"--ambiguous", "3"},
		{"--state", "no-such-fixture"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			if code := Preview(args, nil, strings.NewReader(""), &out); code != 2 {
				t.Errorf("exit %d, want 2\n%s", code, out.String())
			}
			if out.Len() == 0 {
				t.Error("rejected without saying why")
			}
		})
	}
}

// TestPreviewMatrixCoversEveryCapabilitySet checks the four sets §9.4 names are
// all present and all distinct. Two sets that render identically would mean the
// human signed off on three configurations while believing they saw four.
func TestPreviewMatrixCoversEveryCapabilitySet(t *testing.T) {
	seen := map[string]string{}
	for _, cs := range gateCapSets {
		var out bytes.Buffer
		code := Preview([]string{"--bare", "--plain", "--width", "120",
			"--state", "danger-92", "--icons", cs.icons, "--sep", cs.sep},
			map[string]string{"TERM": "xterm-256color"}, strings.NewReader(""), &out)
		if code != 0 {
			t.Fatalf("%s: exit %d", cs.label(), code)
		}
		if prev, dup := seen[out.String()]; dup {
			t.Errorf("%s renders identically to %s", cs.label(), prev)
		}
		seen[out.String()] = cs.label()
	}
	if len(gateCapSets) != 4 {
		t.Errorf("§9.4 names four capability sets; gateCapSets has %d", len(gateCapSets))
	}
}

// TestPreviewHeaderRecordsItsProvenance. The output is screenshotted and read
// months later. A screenshot that does not say which locale, which colour
// profile, and which width produced it settles no question it was taken to
// settle.
func TestPreviewHeaderRecordsItsProvenance(t *testing.T) {
	var out bytes.Buffer
	env := map[string]string{
		"TERM": "xterm-256color", "COLORTERM": "truecolor",
		"LANG": "ja_JP.UTF-8", "COLUMNS": "100",
	}
	if code := Preview([]string{"--state", "startup"}, env, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{
		"ja_JP.UTF-8", // the locale that decided the ambiguous width
		"ambiguous=2", // and what it decided
		"truecolor",   // the profile the gradient depends on
		"columns=100", // the width everything was fitted against
		"available=",  // the budget it was fitted against
		fmt.Sprintf("%d", config.Defaults().General.WidthReserve), // C-7's number
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("header does not record %q:\n%s", want, out.String())
		}
	}
}
