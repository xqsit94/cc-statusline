package style

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

func TestRampInterpolatesLinearlyInSRGB(t *testing.T) {
	r := NewRamp([]string{"#000000", "#ffffff"})
	cases := []struct {
		t    float64
		want string
	}{
		{0, "#000000"},
		{0.5, "#808080"},
		{1, "#ffffff"},
		{-5, "#000000"},
		{99, "#ffffff"},
	}
	for _, tc := range cases {
		if got := r.At(tc.t); got != tc.want {
			t.Errorf("At(%v) = %s, want %s", tc.t, got, tc.want)
		}
	}
}

func TestRampStopsAreEvenlySpaced(t *testing.T) {
	stops := config.Defaults().Colors.GradientStops
	r := NewRamp(stops)
	for i, want := range stops {
		at := float64(i) / float64(len(stops)-1)
		if got := r.At(at); got != want {
			t.Errorf("At(%v) = %s, want stop %d = %s", at, got, i, want)
		}
	}
	if mid := r.At(1.0 / 6); mid == stops[0] || mid == stops[1] {
		t.Errorf("At(1/6) = %s; nothing was interpolated", mid)
	}
}

func TestRampDegenerateCases(t *testing.T) {
	if got := NewRamp(nil); got.Valid() {
		t.Error("an empty ramp reports Valid")
	}
	if got := NewRamp(nil).At(0.5); got != "" {
		t.Errorf("At on an empty ramp = %q, want empty", got)
	}
	one := NewRamp([]string{"#4ade80"})
	if !one.Valid() {
		t.Error("a single-stop ramp is not Valid")
	}
	for _, at := range []float64{0, 0.5, 1} {
		if got := one.At(at); got != "#4ade80" {
			t.Errorf("At(%v) = %s on a single-stop ramp", at, got)
		}
	}
	mixed := NewRamp([]string{"nope", "#ffffff", "", "#000000"})
	if got := mixed.At(0); got != "#ffffff" {
		t.Errorf("At(0) = %s, want the first parseable stop", got)
	}
}

func TestGradientRequiresTruecolor(t *testing.T) {
	cfg := config.Defaults()
	for _, tc := range []struct {
		profile termenv.Profile
		want    bool
	}{
		{termenv.TrueColor, true},
		{termenv.ANSI256, false},
		{termenv.ANSI, false},
		{termenv.Ascii, false},
	} {
		st := NewStyle(Capabilities{Profile: tc.profile, Ambiguous: 1}, cfg)
		if got := st.Gradient(); got != tc.want {
			t.Errorf("Gradient at %v = %v, want %v", tc.profile, got, tc.want)
		}
	}

	off := config.Defaults()
	off.Bar.Gradient = false
	st := NewStyle(Capabilities{Profile: termenv.TrueColor, Ambiguous: 1}, off)
	if st.Gradient() {
		t.Error("gradient = false in the config did not disable it")
	}
}

func TestWidthUsesTheCondition(t *testing.T) {
	cfg := config.Defaults()
	narrow := NewStyle(Capabilities{Ambiguous: 1}, cfg)
	wide := NewStyle(Capabilities{Ambiguous: 2}, cfg)

	for _, s := range []string{"▓", "◆", "│", "…"} {
		if got := narrow.Width(s); got != 1 {
			t.Errorf("Width(%q) = %d at Ambiguous=1, want 1", s, got)
		}
		if got := wide.Width(s); got != 2 {
			t.Errorf("Width(%q) = %d at Ambiguous=2, want 2", s, got)
		}
	}
	for _, s := range []string{"░", "⚠", "⎇"} {
		if got := wide.Width(s); got != 1 {
			t.Errorf("Width(%q) = %d at Ambiguous=2, want 1 — §5.6's list has changed", s, got)
		}
	}
	for _, st := range []*Style{narrow, wide} {
		if got := st.Width("main"); got != 4 {
			t.Errorf("Width(\"main\") = %d, want 4", got)
		}
	}
}

func TestBarCellsShareAWidth(t *testing.T) {
	for _, icons := range []IconSet{IconsASCII, IconsUnicode, IconsNerdFont} {
		for _, amb := range []int{1, 2} {
			st := NewStyle(Capabilities{Icons: icons, Ambiguous: amb}, config.Defaults())
			f, e := st.Width(st.Glyphs.BarFilled), st.Width(st.Glyphs.BarEmpty)
			if f != e {
				t.Errorf("%s at Ambiguous=%d: filled %q is %d cells, empty %q is %d",
					icons, amb, st.Glyphs.BarFilled, f, st.Glyphs.BarEmpty, e)
			}
		}
	}
}

func TestExplicitCellsAreNotSubstituted(t *testing.T) {
	cfg := config.Defaults()
	cfg.Bar.Empty = "░"
	st := NewStyle(Capabilities{Icons: IconsUnicode, Ambiguous: 2}, cfg)
	if st.Glyphs.BarEmpty != "░" {
		t.Errorf("BarEmpty = %q, want the configured ░", st.Glyphs.BarEmpty)
	}
}

func TestConditionIsNotGlobal(t *testing.T) {
	cfg := config.Defaults()
	a := NewStyle(Capabilities{Ambiguous: 2}, cfg)
	b := NewStyle(Capabilities{Ambiguous: 1}, cfg)
	if a.Width("▓") == b.Width("▓") {
		t.Error("two Styles measured identically; the condition is shared")
	}
}

func TestTruncateCells(t *testing.T) {
	st := NewStyle(Capabilities{Ambiguous: 1}, config.Defaults())
	cases := []struct {
		in    string
		cells int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"▓▓▓▓", 2, "▓▓"},
		{"日本語", 2, "日"},
	}
	for _, tc := range cases {
		if got := st.TruncateCells(tc.in, tc.cells); got != tc.want {
			t.Errorf("TruncateCells(%q, %d) = %q, want %q", tc.in, tc.cells, got, tc.want)
		}
	}
}

func TestClipStyledCountsCellsNotBytes(t *testing.T) {
	st := NewStyle(Capabilities{Profile: termenv.TrueColor, Ambiguous: 1}, config.Defaults())
	styled := st.Paint("normal", "abcdefghij")

	if len(styled) <= 10 {
		t.Fatal("nothing was painted; the rest of this test proves nothing")
	}
	got := st.ClipStyled(styled, 4)
	if visible := stripEscapes(got); visible != "abcd" {
		t.Errorf("visible text = %q, want %q", visible, "abcd")
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("clipped output does not end in a reset: %q", got)
	}
}

func TestClipStyledLeavesShortInputAlone(t *testing.T) {
	st := NewStyle(Capabilities{Profile: termenv.TrueColor, Ambiguous: 1}, config.Defaults())
	styled := st.Paint("normal", "abc")
	if got := st.ClipStyled(styled, 80); got != styled {
		t.Errorf("ClipStyled shortened a string that already fitted:\n got %q\nwant %q", got, styled)
	}
}

func TestClipStyledSkipsWholeSequences(t *testing.T) {
	st := NewStyle(Capabilities{Profile: termenv.TrueColor, Ambiguous: 1}, config.Defaults())
	cases := []string{
		"\x1b[38;2;74;222;128mabcdef\x1b[0m",
		"\x1b]8;;https://example.com\x07link\x1b]8;;\x07",
		"\x1b[1m\x1b[4m\x1b[38;5;9mabcdef\x1b[0m",
	}
	for _, in := range cases {
		got := st.ClipStyled(in, 3)
		if strings.Count(got, "\x1b") > strings.Count(in, "\x1b")+1 {
			t.Errorf("ClipStyled invented escapes: %q", got)
		}
		for i := 0; i < len(got); i++ {
			if got[i] != 0x1b {
				continue
			}
			if escapeLen(got[i:]) > len(got)-i {
				t.Errorf("truncated escape at %d in %q", i, got)
			}
		}
	}
}

func TestClipStyledAtZero(t *testing.T) {
	st := NewStyle(Capabilities{Profile: termenv.TrueColor, Ambiguous: 1}, config.Defaults())
	got := st.ClipStyled(st.Paint("normal", "abc"), 0)
	if stripEscapes(got) != "" {
		t.Errorf("clipping to zero left visible text: %q", got)
	}
}

func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += escapeLen(s[i:])
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestEscapeLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"\x1b[0m", 4},
		{"\x1b[38;2;74;222;128m", 18},
		{"\x1b]8;;url\x07", 9},
		{"\x1b]8;;url\x1b\\", 10},
		{"\x1bM", 2},
		{"\x1b", 1},
		{"\x1b[", 2},
		{"\x1b[38;", 5},
	}
	for _, tc := range cases {
		if got := escapeLen(tc.in); got != tc.want {
			t.Errorf("escapeLen(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
