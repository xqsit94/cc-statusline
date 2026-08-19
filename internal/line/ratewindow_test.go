package line

import (
	"strings"
	"testing"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
)

const (
	fiveEpoch = 1785951600

	bothWindows = `{"model":{"display_name":"Claude Opus 4.6"},
		"rate_limits":{"five_hour":{"used_percentage":15,"resets_at":1785951600},
		               "seven_day":{"used_percentage":8,"resets_at":1786383600}}}`
)

const (
	compact = iota
	withClock
	labelled
	labelledWithClock
)

var utc = time.UTC

func segment(t *testing.T, name string, v int, js string, zone *time.Location, env map[string]string) Rendered {
	t.Helper()
	ctx := ctxFor(t, js, env, "")
	ctx.Zone = zone
	useVariant(t, ctx, name, v)
	seg, ok := New(name)
	if !ok {
		t.Fatalf("no segment named %q", name)
	}
	return seg.Render(ctx)
}

func useVariant(t *testing.T, ctx Context, name string, v int) {
	t.Helper()
	vs := config.Variants[name]
	if v >= len(vs) {
		t.Fatalf("%s ships %d formats; there is no %d", name, len(vs), v)
	}
	config.ApplyVariant(vs[v], &ctx.Config.Segments)
}

func TestAWindowRendersWithoutItsPartner(t *testing.T) {
	five := segment(t, "ratelimit_5h", compact, bothWindows, utc, nil).Plain
	seven := segment(t, "ratelimit_7d", compact, bothWindows, utc, nil).Plain

	if five != "5h:15%" {
		t.Errorf("ratelimit_5h = %q, want %q", five, "5h:15%")
	}
	if seven != "7d:8%" {
		t.Errorf("ratelimit_7d = %q, want %q", seven, "7d:8%")
	}
}

func TestEveryShippedFormatRendersWhatItPromises(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    int
		want string
	}{
		{"ratelimit_5h", compact, "5h:15%"},
		{"ratelimit_5h", withClock, "5h:15% ↻ 17:40"},
		{"ratelimit_5h", labelled, "Session: 15%"},
		{"ratelimit_5h", labelledWithClock, "Session: 15% resets 17:40"},

		{"ratelimit_7d", compact, "7d:8%"},
		{"ratelimit_7d", withClock, "7d:8% ↻ 10 Aug 17:40"},
		{"ratelimit_7d", labelled, "Weekly: 8%"},
		{"ratelimit_7d", labelledWithClock, "Weekly: 8% resets 10 Aug 17:40"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := segment(t, tc.name, tc.v, bothWindows, utc, nil).Plain; got != tc.want {
				t.Errorf("%s format %d = %q, want %q", tc.name, tc.v+1, got, tc.want)
			}
		})
	}
}

func TestAWindowIsAbsentWhenItsPercentageIs(t *testing.T) {
	const sevenOnly = `{"rate_limits":{"seven_day":{"used_percentage":77},
		"five_hour":{"resets_at":1785951600}}}`

	for _, tc := range []struct {
		name  string
		empty bool
	}{
		{"ratelimit_5h", true},
		{"ratelimit_7d", false},
	} {
		for v := range config.Variants[tc.name] {
			got := segment(t, tc.name, v, sevenOnly, utc, nil)
			if got.Empty() != tc.empty {
				t.Errorf("%s format %d rendered %q; empty = %v, want %v",
					tc.name, v+1, got.Plain, got.Empty(), tc.empty)
			}
		}
	}
}

func TestTheResetTimeIsRenderedInTheContextsZone(t *testing.T) {
	at := time.Unix(fiveEpoch, 0)

	for _, tc := range []struct {
		zone *time.Location
		want string
	}{
		{time.UTC, at.In(time.UTC).Format("15:04")},
		{time.FixedZone("plus-nine", 9*3600), at.In(time.FixedZone("plus-nine", 9*3600)).Format("15:04")},
		{time.FixedZone("minus-five", -5*3600), at.In(time.FixedZone("minus-five", -5*3600)).Format("15:04")},
	} {
		got := segment(t, "ratelimit_5h", withClock, bothWindows, tc.zone, nil).Plain
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("in %s the segment read %q, want it to end in %q",
				tc.zone, got, tc.want)
		}
	}

	a := segment(t, "ratelimit_5h", withClock, bothWindows, time.UTC, nil).Plain
	b := segment(t, "ratelimit_5h", withClock, bothWindows, time.FixedZone("plus-nine", 9*3600), nil).Plain
	if a == b {
		t.Errorf("two zones nine hours apart rendered the same time: %q", a)
	}
}

func TestAMissingResetTakesItsMarkerWithIt(t *testing.T) {
	const noReset = `{"rate_limits":{"five_hour":{"used_percentage":15}}}`

	for _, tc := range []struct {
		v    int
		want string
	}{
		{withClock, "5h:15%"},
		{labelledWithClock, "Session: 15%"},
	} {
		got := segment(t, "ratelimit_5h", tc.v, noReset, utc, nil).Plain
		if got != tc.want {
			t.Errorf("format %d with no resets_at rendered %q, want %q", tc.v+1, got, tc.want)
		}
	}
}

func TestTheMarkerFollowsTheIconSet(t *testing.T) {
	got := segment(t, "ratelimit_5h", withClock, bothWindows, utc,
		map[string]string{"CC_STATUSLINE_ASCII": "1"}).Plain

	if strings.ContainsFunc(got, func(r rune) bool { return r > 127 }) {
		t.Errorf("under ASCII the segment rendered %q, which is not ASCII", got)
	}
	if !strings.Contains(got, "@") {
		t.Errorf("under ASCII the segment rendered %q, with no reset marker in it", got)
	}
}

func TestTheClockDoesNotTurnYellow(t *testing.T) {
	ctx := ctxFor(t, `{"rate_limits":{"five_hour":{"used_percentage":95,"resets_at":1785951600}}}`,
		map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, "")
	ctx.Zone = utc
	useVariant(t, ctx, "ratelimit_5h", withClock)

	seg, _ := New("ratelimit_5h")
	styled := seg.Render(ctx).Styled

	if want := ctx.Style.Paint("warning", "5h:95%"); !strings.Contains(styled, want) {
		t.Errorf("95%% is not in the warning colour:\n got: %q\nwant it to contain: %q",
			styled, want)
	}
	if want := ctx.Style.Paint("ratelimit", " ↻ 17:40"); !strings.Contains(styled, want) {
		t.Errorf("the clock does not stand apart from the band:\n got: %q\nwant it to contain: %q",
			styled, want)
	}
}

func TestTheResetTimeGoesBeforeTheSegmentDoes(t *testing.T) {
	ctx := ctxFor(t, bothWindows, map[string]string{"COLUMNS": "44"}, "")
	ctx.Zone = utc
	useVariant(t, ctx, "ratelimit_5h", withClock)
	ctx.Config.Lines = []config.Line{{Segments: []config.SegmentRef{
		{Name: "model", Drop: config.NeverDrop},
		{Name: "ratelimit_5h", Drop: config.NeverDrop},
	}}}

	got := RenderPlain(ctx)
	if len(got) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(got), got)
	}
	if !strings.HasSuffix(got[0], "5h:15%") {
		t.Errorf("the line did not shed the reset time: %q", got[0])
	}
	if strings.Contains(got[0], "↻") {
		t.Errorf("the marker survived without its time: %q", got[0])
	}
	if !strings.Contains(got[0], "Claude") {
		t.Errorf("the model was truncated before the clock was: %q", got[0])
	}
}

func TestTheResetTimeIsNeverCutInHalf(t *testing.T) {
	for _, name := range []string{"ratelimit_5h", "ratelimit_7d"} {
		for _, v := range []int{withClock, labelledWithClock} {
			ctx := ctxFor(t, bothWindows, nil, "")
			ctx.Zone = utc
			useVariant(t, ctx, name, v)

			seg, _ := New(name)
			tr, ok := seg.(Truncatable)
			if !ok {
				t.Fatalf("%s is not Truncatable, so stage 2 cannot reach it", name)
			}
			full := seg.Render(ctx).Plain
			bare := full
			for _, lead := range []string{" ↻", " resets"} {
				if before, _, found := strings.Cut(full, lead); found {
					bare = before
				}
			}

			for cells := 0; cells <= ctx.Style.Width(full); cells++ {
				got := tr.Truncate(ctx, cells).Plain
				if got != full && got != bare {
					t.Errorf("%s format %d at %d cells = %q, want %q or %q",
						name, v+1, cells, got, full, bare)
				}
			}
		}
	}
}

func TestAFormatWithNoClockRefusesRatherThanVanishing(t *testing.T) {
	for _, name := range []string{"ratelimit_5h", "ratelimit_7d"} {
		for _, v := range []int{compact, labelled} {
			ctx := ctxFor(t, bothWindows, nil, "")
			ctx.Zone = utc
			useVariant(t, ctx, name, v)

			seg, _ := New(name)
			tr, ok := seg.(Truncatable)
			if !ok {
				t.Fatalf("%s is not Truncatable; stage 2 would skip it", name)
			}
			full := seg.Render(ctx).Plain
			if got := tr.Truncate(ctx, 1).Plain; got != full {
				t.Errorf("%s format %d asked for 1 cell returned %q, want the unchanged %q",
					name, v+1, got, full)
			}
		}
	}
}

func TestASuppressedPlaceholderLeavesNoGap(t *testing.T) {
	const spacedFormat = "5h:{n}% {icon} {reset}"

	ctx := ctxFor(t, `{"rate_limits":{"five_hour":{"used_percentage":15}}}`, nil, "")
	ctx.Zone = utc
	ctx.Config.Segments.RateLimit5h.Format = spacedFormat

	seg, _ := New("ratelimit_5h")
	if got := seg.Render(ctx).Plain; got != "5h:15%" {
		t.Errorf("with no resets_at: %q, want %q", got, "5h:15%")
	}

	ctx = ctxFor(t, bothWindows, nil, "")
	ctx.Zone = utc
	ctx.Config.Segments.RateLimit5h.Format = spacedFormat

	seg, _ = New("ratelimit_5h")
	if got := seg.(Truncatable).Truncate(ctx, 6).Plain; got != "5h:15%" {
		t.Errorf("truncated: %q, want %q", got, "5h:15%")
	}
}
