package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	presets "github.com/xqsit94/cc-statusline/config"
	"github.com/xqsit94/cc-statusline/internal/config"
)

func loadBody(t *testing.T, body string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, notes := config.Load(map[string]string{"CC_STATUSLINE_CONFIG": path})
	if len(notes) != 0 {
		t.Fatalf("the body did not load cleanly: %v\n%s", notes, body)
	}
	return cfg
}

func TestReplaceLinesRoundTripsEveryPreset(t *testing.T) {
	for _, name := range presets.Names() {
		t.Run(name, func(t *testing.T) {
			body, _ := presets.ByName(name)
			before := loadBody(t, body)

			out, err := config.ReplaceLines(body, before.Lines)
			if err != nil {
				t.Fatalf("ReplaceLines: %v", err)
			}
			after := loadBody(t, out)

			if !reflect.DeepEqual(before, after) {
				t.Errorf("a round-trip changed the configuration\nbefore: %+v\nafter:  %+v",
					before.Lines, after.Lines)
			}
			for _, l := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(l)
				if trimmed == "" || trimmed == "[[line]]" || trimmed == "]" ||
					strings.HasPrefix(trimmed, "segments") || strings.HasPrefix(trimmed, "{name") {
					continue
				}
				if !strings.Contains(out, l) {
					t.Errorf("a line outside the [[line]] region was lost: %q", l)
				}
			}
		})
	}
}

func TestReplaceLinesActuallyReorders(t *testing.T) {
	body, _ := presets.ByName("default")
	cfg := loadBody(t, body)

	row := cfg.Lines[0].Segments
	reordered := append([]config.SegmentRef{row[len(row)-1]}, row[:len(row)-1]...)
	want := []config.Line{{Segments: reordered}, {Segments: nil}}

	out, err := config.ReplaceLines(body, want)
	if err != nil {
		t.Fatalf("ReplaceLines: %v", err)
	}
	got := loadBody(t, out).Lines

	if got[0].Segments[0].Name != row[len(row)-1].Name {
		t.Errorf("row 1 starts with %q, want %q", got[0].Segments[0].Name, row[len(row)-1].Name)
	}
	if len(got) != 1 {
		t.Errorf("got %d rows, want 1: the emptied row was written out\n%s", len(got), out)
	}
	if strings.Contains(out, "segments = []") {
		t.Error("wrote an empty segments array; the loader drops that row silently")
	}
}

func TestReplaceLinesRefusesRatherThanDeletes(t *testing.T) {
	rows := []config.Line{{Segments: []config.SegmentRef{{Name: "model", Drop: 99}}}}

	cases := map[string]string{
		"a comment between the rows": `[general]
icons = "unicode"

[[line]]
segments = [{name="model", drop=99}]

# I moved cost down here because I never look at it
[[line]]
segments = [{name="cost", drop=4}]
`,
		"non-contiguous rows": `[[line]]
segments = [{name="model", drop=99}]

[bar]
width = 8

[[line]]
segments = [{name="cost", drop=4}]
`,
		"no rows at all": `[general]
icons = "unicode"
`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := config.ReplaceLines(body, rows)
			if err == nil {
				t.Fatalf("rewrote it instead of refusing:\n%s", out)
			}
			if out != "" {
				t.Errorf("returned a body alongside the error; a caller could write it")
			}
			var target config.ErrUnrecognisedLineRegion
			if !errors.As(err, &target) {
				t.Errorf("error is %T, not the typed refusal a caller can branch on", err)
			}
			if target.Reason == "" {
				t.Error("refused without saying why")
			}
		})
	}
}

func TestReplaceLinesKeepsTheTrailingBlankLine(t *testing.T) {
	body, _ := presets.ByName("default")
	rows := loadBody(t, body).Lines

	out := body
	for i := 0; i < 3; i++ {
		var err error
		if out, err = config.ReplaceLines(out, rows); err != nil {
			t.Fatalf("save %d: %v", i+1, err)
		}
	}
	if once, err := config.ReplaceLines(body, rows); err != nil {
		t.Fatal(err)
	} else if out != once {
		t.Errorf("three saves differ from one; the file is drifting:\n%s", diffFirstLine(once, out))
	}
}

func TestApplyOverridesLeavesTheRestAlone(t *testing.T) {
	body, _ := presets.ByName("default")
	out, applied := config.ApplyOverrides(body, []config.Override{
		{Table: "general", Key: "icons", Value: config.QuoteTOML("nerdfont")},
		{Table: "general", Key: "powerline", Value: "true"},
	})
	if len(applied) != 2 {
		t.Errorf("reported %v, want both", applied)
	}
	cfg := loadBody(t, out)
	if cfg.General.Icons != "nerdfont" {
		t.Errorf("icons = %q", cfg.General.Icons)
	}
	if got := cfg.General.Powerline.String(); got != "true" {
		t.Errorf("powerline = %q", got)
	}
	if !strings.Contains(out, `icons            = "nerdfont"`) {
		t.Errorf("the preset's alignment was not preserved:\n%s", firstMatch(out, "icons"))
	}
	if _, again := config.ApplyOverrides(out, []config.Override{
		{Table: "general", Key: "icons", Value: config.QuoteTOML("nerdfont")},
	}); len(again) != 0 {
		t.Errorf("re-applying an identical value reported %v", again)
	}
}

func firstMatch(body, needle string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

func diffFirstLine(a, b string) string {
	as, bs := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range as {
		if i >= len(bs) {
			return "b ends early at line " + as[i]
		}
		if as[i] != bs[i] {
			return "line " + string(rune('0'+i%10)) + ":\n once: " + as[i] + "\nthrice: " + bs[i]
		}
	}
	return ""
}

func TestASavedMarkerRoundTripsWithoutAPriority(t *testing.T) {
	body := `# a comment that must survive
[[line]]
segments = [
  {name="model", drop=99}, {name="flex"}, {name="cost", drop=4},
]
`
	before := loadBody(t, body)

	out, err := config.ReplaceLines(body, before.Lines)
	if err != nil {
		t.Fatalf("ReplaceLines: %v", err)
	}
	if !strings.Contains(out, `{name="flex"}`) {
		t.Errorf("the marker was not written back as a bare name:\n%s", out)
	}
	if strings.Contains(out, `{name="flex", drop=`) {
		t.Errorf("the marker was written with a priority it does not have:\n%s", out)
	}
	if !reflect.DeepEqual(before.Lines, loadBody(t, out).Lines) {
		t.Errorf("a round-trip changed the layout\nbefore: %+v\nafter:  %+v",
			before.Lines, loadBody(t, out).Lines)
	}
}

func TestATableTheFileLacksIsAppendedRatherThanDropped(t *testing.T) {
	body, ok := presets.ByName("minimal")
	if !ok {
		t.Fatal("no minimal preset")
	}
	if strings.Contains(body, "[segments.") {
		t.Fatal("minimal.toml now names a [segments.*] table; this test needs one that does not")
	}

	out, applied := config.ApplyOverrides(body, []config.Override{
		{Table: "segments.cost", Key: "format", Value: config.QuoteTOML("Cost: ${n}")},
		{Table: "segments.model", Key: "format", Value: config.QuoteTOML("Model: {name}")},
	})
	if len(applied) != 2 {
		t.Errorf("reported %v as written, want both", applied)
	}

	cfg := loadBody(t, out)
	if got := cfg.Segments.Cost.Format; got != "Cost: ${n}" {
		t.Errorf("cost format = %q, want the appended one", got)
	}
	if got := cfg.Segments.Model.Format; got != "Model: {name}" {
		t.Errorf("model format = %q, want the appended one", got)
	}

	if n := len(cfg.Lines[0].Segments); n != 4 {
		t.Errorf("line 1 has %d segments after the append, want 4", n)
	}

	again, applied := config.ApplyOverrides(out, []config.Override{
		{Table: "segments.cost", Key: "format", Value: config.QuoteTOML("Cost: ${n}")},
	})
	if len(applied) != 0 {
		t.Errorf("rewriting the same value reported %v as a change", applied)
	}
	if again != out {
		t.Errorf("a second identical override changed the file:\n%s", diffFirstLine(out, again))
	}
}

func TestQuoteTOMLSurvivesAQuote(t *testing.T) {
	for _, format := range []string{`say "{n}"`, `back\slash`, `plain {n}`} {
		body, _ := config.ApplyOverrides(
			"[segments.cost]\nformat = \"${n}\"\n\n[[line]]\nsegments = [{name=\"cost\", drop=4}]\n",
			[]config.Override{{Table: "segments.cost", Key: "format", Value: config.QuoteTOML(format)}})

		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, notes := config.Load(map[string]string{"CC_STATUSLINE_CONFIG": path})
		if len(notes) != 0 {
			t.Errorf("%q did not survive the round trip: %v", format, notes)
			continue
		}
		if got := cfg.Segments.Cost.Format; got != format {
			t.Errorf("wrote %q, read back %q", format, got)
		}
	}
}
