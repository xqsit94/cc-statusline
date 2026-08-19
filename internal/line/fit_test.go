package line

import (
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
)

const fullPayload = `{"model":{"display_name":"Claude Opus 4.6"},
	"workspace":{"project_dir":"/home/u/api-server"},
	"cost":{"total_cost_usd":15.30,"total_duration_ms":2700000,
	        "total_lines_added":500,"total_lines_removed":120},
	"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
	"rate_limits":{"five_hour":{"used_percentage":85},
	               "seven_day":{"used_percentage":62}}}`

func atWidth(t *testing.T, cols int) Context {
	t.Helper()
	return ctxFor(t, fullPayload, map[string]string{"COLUMNS": itoa(cols)}, "main")
}

func TestDropOrder(t *testing.T) {
	cases := []struct {
		cols   int
		absent []string
		want   []string
	}{
		{200, nil, []string{"45m", "$15.30", "5h:85%", "7d:62%", "▓"}},
		{82, nil, []string{"45m", "$15.30", "5h:85%"}},
		{80, []string{"45m"}, []string{"$15.30", "5h:85%"}},
		{70, []string{"45m", "$15.30"}, []string{"5h:85%", "92%"}},
		{58, []string{"45m", "$15.30", "5h:85%"}, []string{"92%", "▓"}},
	}

	for _, tc := range cases {
		t.Run(itoa(tc.cols), func(t *testing.T) {
			got := RenderPlain(atWidth(t, tc.cols))[0]
			for _, s := range tc.absent {
				if strings.Contains(got, s) {
					t.Errorf("at %d columns %q should have dropped: %q", tc.cols, s, got)
				}
			}
			for _, s := range tc.want {
				if !strings.Contains(got, s) {
					t.Errorf("at %d columns %q should have survived: %q", tc.cols, s, got)
				}
			}
		})
	}
}

func TestNeverDropSurvives(t *testing.T) {
	for _, cols := range []int{10, 15, 20, 25, 30} {
		got := RenderPlain(atWidth(t, cols))
		if len(got) == 0 {
			t.Fatalf("at %d columns nothing rendered at all", cols)
		}
		if !strings.Contains(got[0], "Claude") {
			t.Errorf("at %d columns the model vanished: %q", cols, got[0])
		}
	}
}

func TestBarShrinksBeforeTheModel(t *testing.T) {
	got := RenderPlain(atWidth(t, 50))[0]
	if !strings.Contains(got, "Claude Opus 4.6") {
		t.Errorf("the model name was truncated before the bar shrank: %q", got)
	}
	if strings.Count(got, "▓")+strings.Count(got, "░") == 10 {
		t.Errorf("the bar did not shrink at all: %q", got)
	}
}

func TestBarDisappearsBelowItsFloor(t *testing.T) {
	for _, cols := range []int{20, 30, 40, 50, 60, 70, 80, 90, 120, 200} {
		got := RenderPlain(atWidth(t, cols))[0]
		cells := strings.Count(got, "▓") + strings.Count(got, "░")
		if cells != 0 && cells < barFloor {
			t.Errorf("at %d columns the bar is %d cells, want 0 or ≥%d: %q",
				cols, cells, barFloor, got)
		}
	}
}

func TestClipIsTheLastResort(t *testing.T) {
	ctx := ctxFor(t, `{"model":{"display_name":"`+strings.Repeat("wide ", 40)+`"}}`,
		map[string]string{"COLUMNS": "60"}, "")
	ctx.Config.Lines = []config.Line{{Segments: []config.SegmentRef{
		{Name: "model", Drop: config.NeverDrop},
	}}}

	got := RenderPlain(ctx)
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1", len(got))
	}
	if w := ctx.Style.Width(got[0]); w > available(ctx) {
		t.Errorf("clipped line is %d cells, available is %d", w, available(ctx))
	}
}

func TestClippedStyledEndsWithAReset(t *testing.T) {
	ctx := ctxFor(t, fullPayload, map[string]string{
		"COLUMNS":   "34",
		"COLORTERM": "truecolor",
		"TERM":      "xterm-256color",
	}, "main")

	for _, l := range Render(ctx) {
		if !strings.Contains(l, "\x1b[") {
			continue
		}
		if !strings.HasSuffix(l, "\x1b[0m") {
			t.Errorf("styled line does not end in a reset: %q", l)
		}
	}
}

func TestAmbiguousWidthChangesTheFit(t *testing.T) {
	narrow := ctxFor(t, fullPayload, map[string]string{"COLUMNS": "90"}, "main")
	wide := ctxFor(t, fullPayload, map[string]string{
		"COLUMNS": "90", "LANG": "ja_JP.UTF-8",
	}, "main")

	if wide.Style.Caps.Ambiguous != 2 {
		t.Fatalf("Ambiguous = %d under ja_JP.UTF-8, want 2", wide.Style.Caps.Ambiguous)
	}

	n, w := RenderPlain(narrow)[0], RenderPlain(wide)[0]
	if n == w {
		t.Errorf("the CJK locale fitted identically; the width is not being measured:\n%q", n)
	}
	if wide.Style.Width(w) > available(wide) {
		t.Errorf("CJK line is %d cells, available is %d: %q",
			wide.Style.Width(w), available(wide), w)
	}
}

func TestAvailableHasAFloor(t *testing.T) {
	ctx := atWidth(t, 10)
	if got := available(ctx); got != 20 {
		t.Errorf("available at 10 columns = %d, want the floor of 20", got)
	}
}

func TestPaddingNarrowsTheBudget(t *testing.T) {
	ctx := atWidth(t, 100)
	ctx.Config.General.Padding = 5
	if got, want := available(ctx), 100-10-12; got != want {
		t.Errorf("available = %d, want %d", got, want)
	}
}
