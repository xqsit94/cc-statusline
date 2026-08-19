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

func TestWidthRuleShowsItsNumber(t *testing.T) {
	for n := 6; n <= 200; n++ {
		if !strings.Contains(widthRule(n), strconv.Itoa(n)) {
			t.Errorf("widthRule(%d) does not contain %d: %q", n, n, widthRule(n))
		}
	}
}

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
				if pairs < 8 {
					t.Errorf("found %d rules; four states render at least eight lines", pairs)
				}
			})
		}
	}
}

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
		if !strings.Contains(lines[0], strconv.Itoa(cols/10*10)) {
			t.Errorf("COLUMNS=%d: no %d label in %q", cols, cols/10*10, lines[0])
		}
	}
}

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
		"ja_JP.UTF-8",
		"ambiguous=2",
		"truecolor",
		"columns=100",
		"available=",
		fmt.Sprintf("%d", config.Defaults().General.WidthReserve),
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("header does not record %q:\n%s", want, out.String())
		}
	}
}
