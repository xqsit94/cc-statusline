package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"CC_STATUSLINE_CONFIG": path}
}

func TestPathResolution(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"explicit wins", map[string]string{
			"CC_STATUSLINE_CONFIG": "/tmp/x.toml",
			"XDG_CONFIG_HOME":      "/xdg",
			"HOME":                 "/home/u",
		}, "/tmp/x.toml"},
		{"xdg beats home", map[string]string{
			"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/u",
		}, "/xdg/cc-statusline/config.toml"},
		{"home is the fallback", map[string]string{
			"HOME": "/home/u",
		}, "/home/u/.config/cc-statusline/config.toml"},
		{"nothing set", map[string]string{}, ""},
		{"nil environment", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Path(tc.env); got != tc.want {
				t.Errorf("Path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadWithNoFileIsTheDefaults(t *testing.T) {
	cfg, notes := Load(map[string]string{"HOME": t.TempDir()})
	if len(notes) != 0 {
		t.Errorf("notes = %v, want none", notes)
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Error("a missing config file did not produce the embedded defaults")
	}
}

func TestLoadOverlaysOnlyWhatTheFileSets(t *testing.T) {
	env := writeConfig(t, `
[general]
icons = "ascii"

[bar]
width = 4
`)
	cfg, notes := Load(env)
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	if cfg.General.Icons != "ascii" || cfg.Bar.Width != 4 {
		t.Errorf("icons=%q width=%d; the file was not applied", cfg.General.Icons, cfg.Bar.Width)
	}
	if cfg.Thresholds.Danger != 85 || cfg.Colors.Normal != "#4ade80" || !cfg.Bar.Enabled {
		t.Error("keys absent from the file did not keep their defaults")
	}
	if len(cfg.Lines) != 2 {
		t.Errorf("got %d lines, want the default 2", len(cfg.Lines))
	}
}

func TestLinesReplaceRatherThanMerge(t *testing.T) {
	cfg, notes := Load(writeConfig(t, `
[[line]]
segments = [{name="branch"}, {name="project", drop=1}]
`))
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	if len(cfg.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(cfg.Lines))
	}
	got := cfg.Lines[0].Segments
	want := []SegmentRef{{Name: "branch", Drop: DefaultDrop}, {Name: "project", Drop: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("segments = %+v, want %+v", got, want)
	}
}

func TestOmittedDropIsFifty(t *testing.T) {
	cfg, _ := Load(writeConfig(t, `
[[line]]
segments = [{name="model"}]
`))
	if got := cfg.Lines[0].Segments[0].Drop; got != DefaultDrop {
		t.Errorf("drop = %d, want %d", got, DefaultDrop)
	}
}

func TestFlexibleAcceptsEveryDocumentedForm(t *testing.T) {
	cases := []struct {
		body           string
		powerline      string
		ambiguousWidth string
	}{
		{"powerline = true\nambiguous_width = 2", "true", "2"},
		{"powerline = false\nambiguous_width = 1", "false", "1"},
		{`powerline = "auto"` + "\n" + `ambiguous_width = "auto"`, "auto", "auto"},
		{`powerline = "TRUE"` + "\n" + `ambiguous_width = "2"`, "true", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.powerline+"/"+tc.ambiguousWidth, func(t *testing.T) {
			cfg, notes := Load(writeConfig(t, "[general]\n"+tc.body))
			if len(notes) != 0 {
				t.Errorf("notes = %v, want none", notes)
			}
			if got := cfg.General.Powerline.String(); got != tc.powerline {
				t.Errorf("powerline = %q, want %q", got, tc.powerline)
			}
			if got := cfg.General.AmbiguousWidth.String(); got != tc.ambiguousWidth {
				t.Errorf("ambiguous_width = %q, want %q", got, tc.ambiguousWidth)
			}
		})
	}
}

func TestHostileConfigsStillRender(t *testing.T) {
	cases := map[string]string{
		"not toml at all":   "}{ this is not toml",
		"empty file":        "",
		"only comments":     "# nothing here\n",
		"wrong types":       "[bar]\nwidth = \"ten\"\nenabled = 7\n",
		"unknown table":     "[nonsense]\nkey = 1\n",
		"unknown key":       "[general]\nseparater = \"|\"\n",
		"negative numbers":  "[bar]\nwidth = -5\n[thresholds]\nwarning = -1\ndanger = 900\n",
		"inverted bands":    "[thresholds]\nwarning = 90\ndanger = 20\n",
		"bad colours":       "[colors]\nnormal = \"green\"\ndanger = \"#abc\"\n",
		"empty stops":       "[colors]\ngradient_stops = []\n",
		"bad stop":          "[colors]\ngradient_stops = [\"#4ade80\", \"nope\"]\n",
		"unknown segment":   "[[line]]\nsegments = [{name=\"weather\"}]\n",
		"all bad segments":  "[[line]]\nsegments = [{name=\"weather\"}, {name=\"stocks\"}]\n",
		"bad placeholder":   "[segments.cost]\nformat = \"${total}\"\n",
		"multi-rune cell":   "[bar]\nfilled = \"ab\"\n",
		"newline separator": "[general]\nseparator = \"a\\nb\"\n",
		"huge width":        "[general]\nmax_width = 99999999\n",
		"absurd enum":       "[general]\nicons = \"emoji\"\ncolor = \"beige\"\n",
		"bad show_size":     "[context]\nshow_size = \"sometimes\"\n",
	}

	def := Defaults()
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, _ := Load(writeConfig(t, body))

			if cfg.Bar.Width < 1 {
				t.Errorf("bar.width = %d", cfg.Bar.Width)
			}
			if len(cfg.Lines) == 0 {
				t.Error("no lines survived")
			}
			for _, l := range cfg.Lines {
				if len(l.Segments) == 0 {
					t.Error("a line has no segments")
				}
				for _, ref := range l.Segments {
					if !contains(SegmentNames, ref.Name) {
						t.Errorf("unknown segment %q survived", ref.Name)
					}
					if ref.Drop < 0 || ref.Drop > NeverDrop {
						t.Errorf("drop %d out of range", ref.Drop)
					}
				}
			}
			if cfg.Thresholds.Warning > cfg.Thresholds.Danger {
				t.Errorf("warning %d > danger %d", cfg.Thresholds.Warning, cfg.Thresholds.Danger)
			}
			for _, k := range ColorKeys {
				if !validHex(k.Get(cfg)) {
					t.Errorf("colors.%s = %q is not a hex colour", k.Name, k.Get(cfg))
				}
			}
			if len(cfg.Colors.GradientStops) == 0 {
				t.Error("gradient_stops is empty")
			}
			for _, s := range cfg.Colors.GradientStops {
				if !validHex(s) {
					t.Errorf("gradient stop %q is not a hex colour", s)
				}
			}
			for _, k := range FormatKeys {
				if bad := unknownPlaceholders(k.Get(cfg), k.Placeholders); len(bad) > 0 {
					t.Errorf("%s = %q still names %v", k.Key, k.Get(cfg), bad)
				}
			}
			for _, k := range TimeKeys {
				if !isTimeLayout(k.Get(cfg)) {
					t.Errorf("%s = %q renders as itself", k.Key, k.Get(cfg))
				}
			}
			if got := cfg.General.Icons; !contains([]string{"ascii", "unicode", "nerdfont"}, got) {
				t.Errorf("icons = %q", got)
			}
			_ = def
		})
	}
}

func TestEveryRepairIsReported(t *testing.T) {
	cases := map[string]string{
		"bar.width":               "[bar]\nwidth = 0\n",
		"colors.normal":           "[colors]\nnormal = \"green\"\n",
		"colors.gradient_stops":   "[colors]\ngradient_stops = [\"nope\"]\n",
		"general.icons":           "[general]\nicons = \"emoji\"\n",
		"context.show_size":       "[context]\nshow_size = \"sometimes\"\n",
		"segments.cost.format":    "[segments.cost]\nformat = \"${total}\"\n",
		"thresholds":              "[thresholds]\nwarning = 90\ndanger = 20\n",
		"line[0].segments":        "[[line]]\nsegments = [{name=\"weather\"}, {name=\"model\"}]\n",
		"general.width_reserve":   "[general]\nwidth_reserve = -3\n",
		"general.ambiguous_width": "[general]\nambiguous_width = 5\n",

		"segments.ratelimit_5h.reset_format": "[segments.ratelimit_5h]\n" +
			"reset_format = \"HH:mm\"\n",
	}
	for wantKey, body := range cases {
		t.Run(wantKey, func(t *testing.T) {
			_, notes := Load(writeConfig(t, body))
			for _, n := range notes {
				if n.Key == wantKey {
					if n.Reason == "" {
						t.Errorf("note for %s has no reason", wantKey)
					}
					return
				}
			}
			t.Errorf("nothing reported for %s; got %v", wantKey, notes)
		})
	}
}

func TestUnknownKeysAreReported(t *testing.T) {
	_, notes := Load(writeConfig(t, "[general]\nseparater = \"|\"\n"))
	found := false
	for _, n := range notes {
		if strings.Contains(n.Key, "separater") {
			found = true
		}
	}
	if !found {
		t.Errorf("the typo was not reported; got %v", notes)
	}
}

func TestUnreadableFileIsReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, notes := Load(map[string]string{"CC_STATUSLINE_CONFIG": path})
	if len(notes) == 0 {
		t.Error("an unreadable config produced no note")
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Error("an unreadable config did not fall back to the defaults")
	}
}

func TestPartialFileIsNotPartiallyApplied(t *testing.T) {
	cfg, notes := Load(writeConfig(t, `
[general]
icons = "ascii"

[bar]
width = "this is not a number"
`))
	if len(notes) == 0 {
		t.Fatal("a type error produced no note")
	}
	if cfg.General.Icons != "unicode" {
		t.Errorf("icons = %q; the file was applied up to the error", cfg.General.Icons)
	}
}

func TestNoGitOverlay(t *testing.T) {
	cases := []struct {
		value   string
		enabled bool
		note    bool
	}{
		{"1", false, false},
		{"true", false, false},
		{"0", true, false},
		{"", true, false},
		{"maybe", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			cfg, notes := Load(map[string]string{"CC_STATUSLINE_NO_GIT": tc.value})
			if cfg.Git.Enabled != tc.enabled {
				t.Errorf("git.enabled = %v, want %v", cfg.Git.Enabled, tc.enabled)
			}
			if got := len(notes) > 0; got != tc.note {
				t.Errorf("notes = %v, want a note: %v", notes, tc.note)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		format string
		want   []Token
	}{
		{"", nil},
		{"plain", []Token{{Text: "plain"}}},
		{"{n}", []Token{{Text: "n", Placeholder: true}}},
		{"${n}", []Token{{Text: "$"}, {Text: "n", Placeholder: true}}},
		{"+{added}/-{removed}", []Token{
			{Text: "+"}, {Text: "added", Placeholder: true},
			{Text: "/-"}, {Text: "removed", Placeholder: true},
		}},
		{"{unclosed", []Token{{Text: "{unclosed"}}},
		{"a}b", []Token{{Text: "a}b"}}},
		{"{}", []Token{{Text: "", Placeholder: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			if got := Tokenize(tc.format); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Tokenize(%q) = %+v, want %+v", tc.format, got, tc.want)
			}
		})
	}
}

func TestValidateIsIdempotent(t *testing.T) {
	cfg, _ := Load(writeConfig(t, `
[bar]
width = 0
[colors]
normal = "green"
[[line]]
segments = [{name="weather"}, {name="model", drop=500}]
`))
	if notes := Validate(cfg); len(notes) != 0 {
		t.Errorf("a second validation found %v", notes)
	}
}

func TestDefaultsAreValid(t *testing.T) {
	cfg := Defaults()
	if notes := Validate(cfg); len(notes) != 0 {
		t.Errorf("the embedded defaults do not validate: %v", notes)
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Error("validating the defaults changed them")
	}
}

func TestDefaultsAreNotShared(t *testing.T) {
	a, b := Defaults(), Defaults()
	a.Lines[0].Segments[0].Name = "mutated"
	a.Colors.GradientStops[0] = "#000000"
	if b.Lines[0].Segments[0].Name == "mutated" {
		t.Error("Lines is shared between calls")
	}
	if b.Colors.GradientStops[0] == "#000000" {
		t.Error("GradientStops is shared between calls")
	}
}

func TestValidHex(t *testing.T) {
	for _, s := range []string{"#4ade80", "#FFFFFF", "#000000"} {
		if !validHex(s) {
			t.Errorf("validHex(%q) = false", s)
		}
	}
	for _, s := range []string{"", "#abc", "4ade80", "#4ade8", "#4ade80f", "#ghijkl", "red"} {
		if validHex(s) {
			t.Errorf("validHex(%q) = true", s)
		}
	}
}

func TestFlexIsAcceptedWithoutBeingASegment(t *testing.T) {
	if contains(SegmentNames, FlexName) {
		t.Fatalf("%q is in SegmentNames; internal/line has no renderer for it", FlexName)
	}

	cfg, notes := Load(writeConfig(t, `
[[line]]
segments = [{name="model", drop=99}, {name="flex"}, {name="cost", drop=4}]
`))

	for _, n := range notes {
		if strings.Contains(n.Got, FlexName) || strings.Contains(n.Key, "line") {
			t.Errorf("the marker was reported as a problem: %+v", n)
		}
	}
	got := cfg.Lines[0].Segments
	if len(got) != 3 || !got[1].IsFlex() {
		t.Fatalf("the marker did not survive validation: %+v", got)
	}
}

func TestFlexHasNoDropPriorityToSet(t *testing.T) {
	cfg, _ := Load(writeConfig(t, `
[[line]]
segments = [{name="model", drop=99}, {name="flex", drop=3}, {name="cost", drop=4}]
`))

	if got := cfg.Lines[0].Segments[1].Drop; got != NeverDrop {
		t.Errorf("the marker kept drop=%d from the file, want %d", got, NeverDrop)
	}
}

func TestALineOfNothingButMarkersIsDropped(t *testing.T) {
	cfg, _ := Load(writeConfig(t, `
[[line]]
segments = [{name="model", drop=99}]

[[line]]
segments = [{name="flex"}, {name="flex"}]
`))

	if len(cfg.Lines) != 1 {
		t.Fatalf("got %d lines, want 1: %+v", len(cfg.Lines), cfg.Lines)
	}
	if cfg.Lines[0].Segments[0].Name != "model" {
		t.Errorf("the wrong line survived: %+v", cfg.Lines[0])
	}
}

func TestUnknownSegmentsStillNameTheMarker(t *testing.T) {
	_, notes := Load(writeConfig(t, `
[[line]]
segments = [{name="model", drop=99}, {name="nonesuch"}]
`))

	for _, n := range notes {
		if n.Got == "nonesuch" {
			if !strings.Contains(n.Reason, FlexName) {
				t.Errorf("the note does not mention %q: %s", FlexName, n.Reason)
			}
			return
		}
	}
	t.Error("the unknown segment produced no note at all")
}

func TestATimeLayoutHasToSubstituteSomething(t *testing.T) {
	for _, tc := range []struct {
		layout string
		keep   bool
		why    string
	}{
		{"15:04", true, "the default's own form"},
		{"3:04 PM", true, "a twelve-hour clock"},
		{"2 Jan 15:04", true, "the seven-day default"},
		{"Mon 15:04", true, "a weekday"},
		{"resets 15:04", true, "literal text around a real component"},
		{"HH:mm", false, "the strftime-shaped guess"},
		{"%H:%M", false, "the other strftime-shaped guess"},
		{"soon", false, "prose"},
		{"", false, "nothing at all"},
	} {
		t.Run(tc.why, func(t *testing.T) {
			cfg, _ := Load(writeConfig(t,
				"[segments.ratelimit_5h]\nreset_format = "+QuoteTOML(tc.layout)+"\n"))
			got := cfg.Segments.RateLimit5h.ResetFormat

			if tc.keep && got != tc.layout {
				t.Errorf("%q was replaced by %q; it substitutes, so it is the user's business",
					tc.layout, got)
			}
			if !tc.keep && got != Defaults().Segments.RateLimit5h.ResetFormat {
				t.Errorf("%q survived validation as %q; it would render as itself",
					tc.layout, got)
			}
		})
	}
}
