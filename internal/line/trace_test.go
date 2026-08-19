package line

import (
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
)

const youngPayload = `{"model":{"display_name":"Claude Opus 4.6"},
	"workspace":{"project_dir":"/home/u/api-server"},
	"context_window":{"context_window_size":200000,"total_input_tokens":20000}}`

func TestTraceTellsADropFromAnEmptySegment(t *testing.T) {
	young := ctxFor(t, youngPayload, map[string]string{"COLUMNS": "200"}, "main")
	if got := Trace(young)[0][3]; got != Absent {
		t.Errorf("duration on a session with no duration: %v, want absent", got)
	}
	if got := Trace(young)[0][0]; got != Shown {
		t.Errorf("model at 200 columns: %v, want shown", got)
	}

	crowded := atWidth(t, 80)
	if got := Trace(crowded)[0][3]; got != Dropped {
		t.Errorf("duration at 80 columns: %v, want dropped", got)
	}

	for _, ctx := range []Context{young, crowded} {
		if got := RenderPlain(ctx)[0]; strings.Contains(got, "45m") || strings.Contains(got, "0m") {
			t.Errorf("a duration is on the line after all: %q", got)
		}
	}
}

func TestTraceReportsTruncationSeparatelyFromSurvival(t *testing.T) {
	ctx := ctxFor(t, fullPayload, map[string]string{"COLUMNS": "40"},
		"some-very-long-feature-branch")
	cfg := *ctx.Config
	cfg.Lines = []config.Line{{Segments: []config.SegmentRef{
		seg("branch", config.NeverDrop), seg("model", config.NeverDrop),
	}}}
	ctx.Config = &cfg

	got := Trace(ctx)[0]
	for i, name := range []string{"branch", "model"} {
		if got[i] != Truncated {
			t.Errorf("%s at 40 columns: %v, want truncated\nline: %q",
				name, got[i], RenderPlain(ctx)[0])
		}
	}

	wide := ctxFor(t, fullPayload, map[string]string{"COLUMNS": "200"},
		"some-very-long-feature-branch")
	wcfg := *wide.Config
	wcfg.Lines = cfg.Lines
	wide.Config = &wcfg
	for i, name := range []string{"branch", "model"} {
		if got := Trace(wide)[0][i]; got != Shown {
			t.Errorf("%s at 200 columns: %v, want shown", name, got)
		}
	}
}

func TestTraceIndexesTheConfigurationAndNotTheScreen(t *testing.T) {
	ctx := ctxFor(t, youngPayload, map[string]string{"COLUMNS": "200"}, "main")
	cfg := *ctx.Config
	cfg.Lines = []config.Line{{Segments: []config.SegmentRef{
		seg("cost", 4), seg("duration", 5), seg("model", config.NeverDrop),
		seg("project", 1),
	}}}
	ctx.Config = &cfg

	want := []Placement{Absent, Absent, Shown, Shown}
	got := Trace(ctx)[0]
	if len(got) != len(want) {
		t.Fatalf("%d entries, want %d — one per configured segment", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("segments[%d] (%s): %v, want %v",
				i, cfg.Lines[0].Segments[i].Name, got[i], want[i])
		}
	}
}

func TestATrimmedMarkerIsNotReportedAsShown(t *testing.T) {
	ctx := ctxFor(t, youngPayload, map[string]string{"COLUMNS": "200"}, "main")
	cfg := *ctx.Config
	cfg.Lines = []config.Line{{Segments: []config.SegmentRef{
		seg("model", config.NeverDrop), flex(), seg("cost", 4),
	}}}
	ctx.Config = &cfg

	if got := Trace(ctx)[0][1]; got != Dropped {
		t.Errorf("a marker with nothing to its right: %v, want dropped", got)
	}
	if got := RenderPlain(ctx)[0]; strings.HasSuffix(got, " ") {
		t.Errorf("the trimmed marker became trailing whitespace: %q", got)
	}

	full := atWidth(t, 200)
	fcfg := *full.Config
	fcfg.Lines = cfg.Lines
	full.Config = &fcfg
	if got := Trace(full)[0][1]; got != Shown {
		t.Errorf("a marker that is spending width: %v, want shown", got)
	}
}

func TestTraceAgreesWithTheLineAtEveryWidth(t *testing.T) {
	probes := map[string]string{
		"model": "Claude", "context": "92%", "cost": "$", "duration": "45m",
		"ratelimits": "5h:", "branch": "main", "diffstat": "+500", "project": "api-server",
	}

	for cols := 56; cols <= 160; cols++ {
		ctx := atWidth(t, cols)
		rendered := RenderPlain(ctx)
		trace := Trace(ctx)

		shown := 0
		for r, row := range trace {
			var onScreen string
			if visible(row) && shown < len(rendered) {
				onScreen = rendered[shown]
				shown++
			}
			for i, p := range row {
				name := ctx.Config.Lines[r].Segments[i].Name
				probe, ok := probes[name]
				if !ok {
					t.Fatalf("no probe for %q — a new segment needs one here", name)
				}
				present := strings.Contains(onScreen, probe)
				if want := p == Shown || p == Truncated; present != want {
					t.Errorf("at %d columns, %s traced %v; on the line: %v\n%q",
						cols, name, p, present, onScreen)
				}
			}
		}
	}
}

func visible(row []Placement) bool {
	for _, p := range row {
		if p == Shown || p == Truncated {
			return true
		}
	}
	return false
}
