package line

import (
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
)

func flexCtx(t *testing.T, cols int, segments ...config.SegmentRef) Context {
	t.Helper()
	ctx := atWidth(t, cols)
	cfg := *ctx.Config
	cfg.Lines = []config.Line{{Segments: segments}}
	ctx.Config = &cfg
	return ctx
}

func flex() config.SegmentRef {
	return config.SegmentRef{Name: config.FlexName, Drop: config.NeverDrop}
}

func seg(name string, drop int) config.SegmentRef {
	return config.SegmentRef{Name: name, Drop: drop}
}

func TestFlexFillsTheLineToExactlyAvailable(t *testing.T) {
	for _, cols := range []int{200, 120, 96, 84} {
		ctx := flexCtx(t, cols,
			seg("model", config.NeverDrop), flex(), seg("cost", 4))

		got := RenderPlain(ctx)
		if len(got) != 1 {
			t.Fatalf("at %d columns: %d lines, want 1", cols, len(got))
		}
		if w, want := ctx.Style.Width(got[0]), Available(ctx); w != want {
			t.Errorf("at %d columns the line is %d cells, want exactly %d (available)\n%q",
				cols, w, want, got[0])
		}
		if !strings.HasSuffix(got[0], "$15.30") {
			t.Errorf("at %d columns the flex did not push the cost to the end: %q", cols, got[0])
		}
	}
}

func TestFlexReplacesTheSeparatorRatherThanJoiningIt(t *testing.T) {
	ctx := flexCtx(t, 200, seg("model", config.NeverDrop), flex(), seg("cost", 4))

	got := RenderPlain(ctx)[0]
	if strings.Contains(got, "│") {
		t.Errorf("a separator survived beside the marker: %q", got)
	}
	if strings.Contains(Render(ctx)[0], "│") {
		t.Errorf("a separator survived in the styled line: %q", Render(ctx)[0])
	}
}

func TestFlexNeverWidensALineOverBudget(t *testing.T) {
	for cols := 20; cols <= 120; cols++ {
		ctx := flexCtx(t, cols,
			seg("model", config.NeverDrop), flex(), seg("cost", 4),
			flex(), seg("duration", 5), seg("ratelimits", 3))

		for i, l := range RenderPlain(ctx) {
			if w, avail := ctx.Style.Width(l), Available(ctx); w > avail {
				t.Fatalf("at %d columns line %d is %d cells, over the %d available\n%q",
					cols, i+1, w, avail, l)
			}
		}
	}
}

func TestFlexAtItsFloorKeepsSegmentsApart(t *testing.T) {
	cases := []struct {
		cols int
		want string
	}{
		{36, "◆ Claude Opus 4.6 $15.30"},
		{35, "◆ Claude Opus 4… $15.30"},
	}

	for _, tc := range cases {
		ctx := flexCtx(t, tc.cols,
			seg("model", config.NeverDrop), flex(), seg("cost", config.NeverDrop))
		if got := RenderPlain(ctx)[0]; got != tc.want {
			t.Errorf("at %d columns the marker did not pay its floor cell\n got: %q\nwant: %q",
				tc.cols, got, tc.want)
		}
	}
}

func TestALeadingFlexDoesNotIndent(t *testing.T) {
	both := []config.SegmentRef{
		flex(), seg("model", config.NeverDrop), seg("cost", config.NeverDrop),
	}

	wide := flexCtx(t, 200, both...)
	got := RenderPlain(wide)[0]
	if !strings.HasPrefix(got, "  ") {
		t.Errorf("a leading marker did not right-align: %q", got)
	}
	if w, want := wide.Style.Width(got), Available(wide); w != want {
		t.Errorf("the right-aligned line is %d cells, want %d", w, want)
	}

	flush := flexCtx(t, 38, both...)
	if got := RenderPlain(flush)[0]; strings.HasPrefix(got, " ") {
		t.Errorf("a leading marker indented a line it had no room to align: %q", got)
	}
}

func TestATrailingFlexIsNotTrailingWhitespace(t *testing.T) {
	cases := []struct {
		name     string
		cols     int
		segments []config.SegmentRef
	}{
		{"written", 200, []config.SegmentRef{seg("model", config.NeverDrop), flex()}},
		{"emptied", 200, []config.SegmentRef{seg("branch", config.NeverDrop), flex(), seg("nonesuch", 1)}},
		{"dropped", 40, []config.SegmentRef{seg("model", config.NeverDrop), flex(), seg("cost", 1)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderPlain(flexCtx(t, tc.cols, tc.segments...))
			if len(got) == 0 {
				t.Fatal("nothing rendered")
			}
			if strings.HasSuffix(got[0], " ") {
				t.Errorf("the line ends in whitespace: %q", got[0])
			}
		})
	}
}

func TestTwoFlexesSplitTheLeftover(t *testing.T) {
	ctx := flexCtx(t, 200,
		seg("model", config.NeverDrop), flex(), seg("cost", 4), flex(), seg("duration", 5))

	got := RenderPlain(ctx)[0]
	gaps := gapWidths(got)
	if len(gaps) != 2 {
		t.Fatalf("want two gaps, got %d in %q", len(gaps), got)
	}
	if d := gaps[0] - gaps[1]; d < 0 || d > 1 {
		t.Errorf("the leftover split %d/%d, which is not even to within the remainder: %q",
			gaps[0], gaps[1], got)
	}
	if w, want := ctx.Style.Width(got), Available(ctx); w != want {
		t.Errorf("the line is %d cells, want %d", w, want)
	}
}

func gapWidths(s string) []int {
	var out []int
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
			continue
		}
		if n > 1 {
			out = append(out, n)
		}
		n = 0
	}
	if n > 1 {
		out = append(out, n)
	}
	return out
}

func TestALineOfOnlyFlexIsNotALine(t *testing.T) {
	ctx := atWidth(t, 200)
	cfg := *ctx.Config
	cfg.Lines = []config.Line{
		{Segments: []config.SegmentRef{seg("model", config.NeverDrop)}},
		{Segments: []config.SegmentRef{flex(), flex()}},
	}
	ctx.Config = &cfg

	if got := RenderPlain(ctx); len(got) != 1 {
		t.Errorf("got %d lines, want 1 — the marker-only row was rendered: %q", len(got), got)
	}
}

func TestFlexUnderPowerline(t *testing.T) {
	ctx := ctxFor(t, fullPayload, map[string]string{
		"COLUMNS": "200", "CC_STATUSLINE_POWERLINE": "1",
	}, "main")
	cfg := *ctx.Config
	cfg.Lines = []config.Line{{Segments: []config.SegmentRef{
		seg("model", config.NeverDrop), flex(), seg("cost", 4),
	}}}
	ctx.Config = &cfg

	got := RenderPlain(ctx)[0]
	if strings.Count(got, "  ") == 0 {
		t.Errorf("the marker did not expand under Powerline: %q", got)
	}
	if w, want := ctx.Style.Width(got), Available(ctx); w != want {
		t.Errorf("the line is %d cells, want %d: %q", w, want, got)
	}
}
