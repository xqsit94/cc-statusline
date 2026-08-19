package style

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

func TestColorSurvivesAPipe(t *testing.T) {
	if os.Getenv("CC_STATUSLINE_PIPE_CHILD") == "1" {
		st := NewStyle(
			Detect(map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, nil),
			nil,
		)
		os.Stdout.WriteString(st.Paint("model_marker", "X"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestColorSurvivesAPipe")
	cmd.Env = append(os.Environ(), "CC_STATUSLINE_PIPE_CHILD=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	if !strings.Contains(string(out), "\x1b[38;2;") {
		t.Errorf("output through a pipe = %q, want 24-bit escapes.\n"+
			"Colour is being stripped because stdout is not a terminal — PRD §6.5.", out)
	}
}

func TestProfileIsHonoured(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"truecolor", map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, "\x1b[38;2;203;166;247m"},
		{"256", map[string]string{"TERM": "xterm-256color"}, "\x1b[38;5;183m"},
		{"16", map[string]string{"TERM": "xterm"}, "\x1b[95m"},
		{"NO_COLOR wins over COLORTERM", map[string]string{"TERM": "xterm", "COLORTERM": "truecolor", "NO_COLOR": "1"}, ""},
		{"NO_COLOR empty still counts", map[string]string{"TERM": "xterm", "NO_COLOR": ""}, ""},
		{"TERM=dumb beats a globally exported COLORTERM", map[string]string{"TERM": "dumb", "COLORTERM": "truecolor"}, ""},
		{"TERM unset", map[string]string{"COLORTERM": "truecolor"}, ""},
		{"explicit override beats inference", map[string]string{"TERM": "xterm", "COLORTERM": "truecolor", "CC_STATUSLINE_COLOR": "16"}, "\x1b[95m"},
		{"explicit none", map[string]string{"TERM": "xterm-256color", "CC_STATUSLINE_COLOR": "none"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := NewStyle(Detect(tc.env, nil), nil)
			got := st.Paint("model_marker", "X")

			if tc.want == "" {
				if got != "X" {
					t.Errorf("Paint = %q, want %q (no escapes)", got, "X")
				}
				if st.Colored() {
					t.Error("Colored() = true for a no-colour profile")
				}
				return
			}
			if want := tc.want + "X\x1b[0m"; got != want {
				t.Errorf("Paint = %q, want %q", got, want)
			}
		})
	}
}

func TestDetectIcons(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want IconSet
	}{
		{"default", nil, IconsUnicode},
		{"ascii", map[string]string{"CC_STATUSLINE_ASCII": "1"}, IconsASCII},
		{"nerdfont", map[string]string{"CC_STATUSLINE_NERDFONT": "1"}, IconsNerdFont},
		{"ascii beats nerdfont", map[string]string{"CC_STATUSLINE_ASCII": "1", "CC_STATUSLINE_NERDFONT": "1"}, IconsASCII},
		{"0 is not truthy", map[string]string{"CC_STATUSLINE_ASCII": "0"}, IconsUnicode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.env, nil).Icons; got != tc.want {
				t.Errorf("Icons = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPowerlineFollowsTheFont(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"off under unicode", nil, false},
		{"on under nerdfont", map[string]string{"CC_STATUSLINE_NERDFONT": "1"}, true},
		{"explicit off under nerdfont", map[string]string{"CC_STATUSLINE_NERDFONT": "1", "CC_STATUSLINE_POWERLINE": "0"}, false},
		{"explicit on under unicode", map[string]string{"CC_STATUSLINE_POWERLINE": "1"}, true},
		{"never under ascii, even when asked", map[string]string{"CC_STATUSLINE_ASCII": "1", "CC_STATUSLINE_POWERLINE": "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.env, nil).Powerline; got != tc.want {
				t.Errorf("Powerline = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectColumns(t *testing.T) {
	cfg := config.Defaults()
	cases := []struct {
		name string
		env  map[string]string
		cfg  *config.Config
		want int
	}{
		{"unset falls back to 80", nil, cfg, 80},
		{"COLUMNS wins", map[string]string{"COLUMNS": "120"}, cfg, 120},
		{"whitespace tolerated", map[string]string{"COLUMNS": " 100 "}, cfg, 100},
		{"garbage ignored", map[string]string{"COLUMNS": "wide"}, cfg, 80},
		{"zero ignored", map[string]string{"COLUMNS": "0"}, cfg, 80},
		{"negative ignored", map[string]string{"COLUMNS": "-5"}, cfg, 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.env, tc.cfg).Columns; got != tc.want {
				t.Errorf("Columns = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("max_width applies when COLUMNS is unset", func(t *testing.T) {
		c := config.Defaults()
		c.General.MaxWidth = 60
		if got := Detect(nil, c).Columns; got != 60 {
			t.Errorf("Columns = %d, want 60", got)
		}
		if got := Detect(map[string]string{"COLUMNS": "120"}, c).Columns; got != 120 {
			t.Errorf("Columns = %d, want 120 (COLUMNS outranks max_width)", got)
		}
	})
}

func TestAmbiguousWidth(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want int
	}{
		{"default", nil, 1},
		{"ja_JP", map[string]string{"LANG": "ja_JP.UTF-8"}, 2},
		{"zh_CN via LC_CTYPE", map[string]string{"LC_CTYPE": "zh_CN.UTF-8"}, 2},
		{"ko_KR", map[string]string{"LANG": "ko_KR.UTF-8"}, 2},
		{"LC_ALL outranks LANG", map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "ja_JP.UTF-8"}, 1},
		{"en_US", map[string]string{"LANG": "en_US.UTF-8"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.env, nil).Ambiguous; got != tc.want {
				t.Errorf("Ambiguous = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("explicit config overrides the locale", func(t *testing.T) {
		c := config.Defaults()
		c.General.AmbiguousWidth = "1"
		if got := Detect(map[string]string{"LANG": "ja_JP.UTF-8"}, c).Ambiguous; got != 1 {
			t.Errorf("Ambiguous = %d, want 1", got)
		}
	})
}

func TestGlyphOverrides(t *testing.T) {
	c := config.Defaults()
	if g := GlyphsFor(IconsASCII, c); g.BarFilled != "#" {
		t.Errorf(`"auto" under ASCII gave %q, want "#"`, g.BarFilled)
	}

	c.Bar.Filled = "="
	if g := GlyphsFor(IconsASCII, c); g.BarFilled != "=" {
		t.Errorf("explicit override gave %q, want %q", g.BarFilled, "=")
	}
	if g := GlyphsFor(IconsNerdFont, c); g.BarFilled != "=" {
		t.Error("an explicit override must apply to every icon set")
	}
}

func TestProfileName(t *testing.T) {
	for p, want := range map[termenv.Profile]string{
		termenv.TrueColor: "truecolor", termenv.ANSI256: "256",
		termenv.ANSI: "16", termenv.Ascii: "none",
	} {
		if got := ProfileName(p); got != want {
			t.Errorf("ProfileName(%v) = %q, want %q", p, got, want)
		}
	}
}

func TestEnviron(t *testing.T) {
	got := Environ([]string{"A=1", "B=x=y", "MALFORMED", "C="})
	want := map[string]string{"A": "1", "B": "x=y", "C": ""}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Environ()[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["MALFORMED"]; ok {
		t.Error("an entry with no '=' must be skipped, not stored")
	}
}
