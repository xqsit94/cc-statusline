package style

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
)

type escapes struct{ truecolor, ansi256, ansi string }

var escapeByHex = map[string]escapes{
	"#cba6f7": {"\x1b[38;2;203;166;247m", "\x1b[38;5;183m", "\x1b[95m"},
	"#89dceb": {"\x1b[38;2;137;220;235m", "\x1b[38;5;117m", "\x1b[96m"},
	"#4ade80": {"\x1b[38;2;73;222;128m", "\x1b[38;5;78m", "\x1b[92m"},
	"#facc15": {"\x1b[38;2;250;204;21m", "\x1b[38;5;220m", "\x1b[93m"},
	"#ef4444": {"\x1b[38;2;239;68;68m", "\x1b[38;5;203m", "\x1b[91m"},
	"#6c7086": {"\x1b[38;2;108;112;134m", "\x1b[38;5;60m", "\x1b[94m"},
	"#94e2d5": {"\x1b[38;2;147;226;213m", "\x1b[38;5;116m", "\x1b[96m"},
	"#89b4fa": {"\x1b[38;2;137;179;250m", "\x1b[38;5;111m", "\x1b[94m"},
	"#45475a": {"\x1b[38;2;69;71;89m", "\x1b[38;5;59m", "\x1b[90m"},
	"#fb923c": {"\x1b[38;2;251;146;60m", "\x1b[38;5;209m", "\x1b[91m"},
}

var defaultColors = map[string]string{
	"model_marker":   "#cba6f7",
	"model_name":     "#89dceb",
	"normal":         "#4ade80",
	"warning":        "#facc15",
	"danger":         "#ef4444",
	"cost":           "#4ade80",
	"duration":       "#6c7086",
	"ratelimit":      "#6c7086",
	"effort":         "#94e2d5",
	"branch":         "#cba6f7",
	"added":          "#4ade80",
	"removed":        "#ef4444",
	"project":        "#89b4fa",
	"separator":      "#45475a",
	"diffstat_delim": "#45475a",
	"bar_empty":      "#45475a",
}

var profileEnv = map[string]map[string]string{
	"truecolor": {"TERM": "xterm", "COLORTERM": "truecolor"},
	"256":       {"TERM": "xterm-256color"},
	"16":        {"TERM": "xterm"},
	"none":      {"TERM": "xterm", "COLORTERM": "truecolor", "NO_COLOR": "1"},
}

var allProfiles = []string{"truecolor", "256", "16", "none"}

func TestExactEscapePerProfile(t *testing.T) {
	cfg := config.Defaults()

	for _, k := range config.ColorKeys {
		hex, ok := defaultColors[k.Name]
		if !ok {
			continue
		}
		if got, _ := cfg.Color(k.Name); got != hex {
			t.Errorf("[colors] %s is now %s, not %s.\n"+
				"Update defaultColors, and add the new value to escapeByHex "+
				"after reading what it emits.", k.Name, got, hex)
			continue
		}
		esc, ok := escapeByHex[hex]
		if !ok {
			continue
		}
		for _, p := range allProfiles {
			t.Run(k.Name+"/"+p, func(t *testing.T) {
				st := NewStyle(Detect(profileEnv[p], cfg), cfg)
				assertEscape(t, st.Paint(k.Name, "X"), esc, p, k.Name)
			})
		}
	}

	for i, hex := range cfg.Colors.GradientStops {
		esc, ok := escapeByHex[hex]
		if !ok {
			continue
		}
		for _, p := range allProfiles {
			name := fmt.Sprintf("gradient_stops[%d]", i)
			t.Run(name+"/"+p, func(t *testing.T) {
				st := NewStyle(Detect(profileEnv[p], cfg), cfg)
				assertEscape(t, st.PaintHex(hex, "X"), esc, p, name)
			})
		}
	}
}

func assertEscape(t *testing.T, got string, esc escapes, profile, what string) {
	t.Helper()
	want := "X"
	if s := esc.at(profile); s != "" {
		want = s + "X\x1b[0m"
	}
	if got != want {
		t.Errorf("%s at %s\n got: %q\nwant: %q", what, profile, got, want)
	}
}

func (e escapes) at(profile string) string {
	switch profile {
	case "truecolor":
		return e.truecolor
	case "256":
		return e.ansi256
	case "16":
		return e.ansi
	default:
		return ""
	}
}

func TestEveryColorKeyHasAFrozenEscape(t *testing.T) {
	cfg := config.Defaults()

	for _, k := range config.ColorKeys {
		hex, ok := defaultColors[k.Name]
		if !ok {
			t.Errorf("[colors] %s has no entry in defaultColors", k.Name)
			continue
		}
		if _, ok := escapeByHex[hex]; !ok {
			t.Errorf("[colors] %s is %s, which has no entry in escapeByHex", k.Name, hex)
		}
	}
	for i, hex := range cfg.Colors.GradientStops {
		if _, ok := escapeByHex[hex]; !ok {
			t.Errorf("gradient_stops[%d] is %s, which has no entry in escapeByHex", i, hex)
		}
	}
}

func TestNoFrozenEscapeIsUnused(t *testing.T) {
	used := map[string]bool{}
	for _, hex := range defaultColors {
		used[hex] = true
	}
	for _, hex := range config.Defaults().Colors.GradientStops {
		used[hex] = true
	}
	for hex := range escapeByHex {
		if !used[hex] {
			t.Errorf("escapeByHex has %s, which no [colors] key or gradient stop uses", hex)
		}
	}
}

func TestNoProfileEmitsAnotherProfilesEscape(t *testing.T) {
	cfg := config.Defaults()
	forbidden := map[string][]string{
		"256":  {"\x1b[38;2;"},
		"16":   {"\x1b[38;2;", "\x1b[38;5;"},
		"none": {"\x1b[38;2;", "\x1b[38;5;", "\x1b["},
	}
	for profile, bad := range forbidden {
		st := NewStyle(Detect(profileEnv[profile], cfg), cfg)
		for _, k := range config.ColorKeys {
			got := st.Paint(k.Name, "X")
			for _, b := range bad {
				if strings.Contains(got, b) {
					t.Errorf("%s at profile %s emitted %q: %q", k.Name, profile, b, got)
				}
			}
		}
	}
}
