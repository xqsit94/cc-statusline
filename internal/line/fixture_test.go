package line

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/refstate"
)

func TestFixturesStillIsolateWhatTheyWereBuiltFor(t *testing.T) {
	cases := []struct {
		fixture string
		rule    string
		check   func(t *testing.T, l1, l2 string)
	}{
		{
			"null-context", "§5.3 — a null percent renders as 0%, it does not blank the segment",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l1, " 0%")
				mustContain(t, l1, "$2.10")
				mustContain(t, l1, "15m")
			},
		},
		{
			"no-ratelimits", "§5.3 — the segment is empty when both windows are absent",
			func(t *testing.T, l1, l2 string) {
				mustNotContain(t, l1, "5h:")
				mustNotContain(t, l1, "7d:")
			},
		},
		{
			"seven-only", "§5.7 — the two windows are independent; either may be absent",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l1, "7d:77%")
				mustNotContain(t, l1, "5h:")
			},
		},
		{
			"no-git", "§5.3 — branch is empty with no git dir, and takes nothing else with it",
			func(t *testing.T, l1, l2 string) {
				mustNotContain(t, l2, "⎇")
				mustContain(t, l2, "+140/-27")
				mustContain(t, l2, "scratch")
			},
		},
		{
			"detached", "§5.8 — a detached HEAD renders its short SHA as the branch",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l2, "⎇ a1b2c3d")
			},
		},
		{
			"long-branch", "§5.7 — git.branch_max_len truncates with the icon set's ellipsis",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l2, "…")
				mustNotContain(t, l2, "width-model")
			},
		},
		{
			"500k-context", "§5.3 — show_size = non_default renders any size that is not 200k",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l1, "55% 500k")
			},
		},
		{
			"fractional-pct", "§5.3 — p_shown drives the number and the band, p_exact drives the fill",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l1, " 70%")
				if n := strings.Count(l1, "▓"); n != 7 {
					t.Errorf("bar fill = %d cells, want 7 (p_exact 69.6 of 10)", n)
				}
			},
		},
		{
			"wide-cost", "§5.7 — FormatFloat(v,'f',2,64) absorbs the float noise",
			func(t *testing.T, l1, l2 string) {
				mustContain(t, l1, "$107.43")
				mustNotContain(t, l1, "107.430942")
			},
		},
		{
			"sub-minute", "§5.3 — the duration segment is empty below one minute",
			func(t *testing.T, l1, l2 string) {
				mustNotContain(t, l1, "0m")
				mustNotContain(t, l1, "1m")
				mustContain(t, l1, "$0.11")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			st, ok := refstate.ByName(tc.fixture)
			if !ok {
				t.Fatalf("fixture %s is gone.\nIt was the golden for: %s", tc.fixture, tc.rule)
			}
			lines := RenderPlain(goldenContext(t, st, "unicode", "plain", 120, "1"))
			if len(lines) != 2 {
				t.Fatalf("got %d lines, want 2: %q", len(lines), lines)
			}
			t.Logf("rule: %s", tc.rule)
			tc.check(t, lines[0], lines[1])
		})
	}
}

func TestFractionalPercentBandsOnPShown(t *testing.T) {
	st, ok := refstate.ByName("fractional-pct")
	if !ok {
		t.Fatal("fractional-pct is gone; the band-boundary case has no fixture")
	}
	ctx := styledContext(t, st, "unicode", "plain", 120)

	const (
		warning = "\x1b[38;2;250;204;21m"
		normal  = "\x1b[38;2;73;222;128m"
	)
	got := Render(ctx)[0]

	i := strings.Index(got, "70%")
	if i < 0 {
		t.Fatalf("no %q in %q", "70%", got)
	}
	prefix := got[:i]
	last := strings.LastIndex(prefix, "\x1b[38;2;")
	if last < 0 {
		t.Fatalf("the percent is unstyled: %q", got)
	}
	switch {
	case strings.HasPrefix(prefix[last:], warning):
	case strings.HasPrefix(prefix[last:], normal):
		t.Errorf("the percent is in the normal band.\n" +
			"p_exact is 69.6 and p_shown is 70; §5.3 bands on p_shown, so this must be amber.\n" +
			"Something is comparing the band against p_exact.")
	default:
		t.Errorf("the percent is in neither band: %q", prefix[last:last+20])
	}
}

func mustContain(t *testing.T, s, want string) {
	t.Helper()
	if !strings.Contains(s, want) {
		t.Errorf("missing %q in %q", want, s)
	}
}

func mustNotContain(t *testing.T, s, unwanted string) {
	t.Helper()
	if strings.Contains(s, unwanted) {
		t.Errorf("unexpected %q in %q", unwanted, s)
	}
}

func TestSixtyCharacterModelNameUnderAmbiguousTwo(t *testing.T) {
	st, ok := refstate.ByName("normal-42")
	if !ok {
		t.Fatal("normal-42 is gone")
	}
	const sixty = "Claude Opus 4.6 Extended Thinking Preview 1M Ultra Long Name"
	if len([]rune(sixty)) != 60 {
		t.Fatalf("the name is %d characters, not the 60 §9.3 asks for", len([]rune(sixty)))
	}
	st.Payload = []byte(strings.Replace(string(st.Payload),
		`"display_name": "Claude Opus 4.6"`, `"display_name": "`+sixty+`"`, 1))
	if !strings.Contains(string(st.Payload), sixty) {
		t.Fatal("the substitution did not take; the fixture's shape has changed")
	}

	for _, icons := range goldenIcons {
		for _, sep := range goldenSeps {
			for _, cols := range []int{10, 40, 80, 120, 200} {
				t.Run(fmt.Sprintf("%s/%s/%d", icons, sep, cols), func(t *testing.T) {
					ctx := goldenContext(t, st, icons, sep, cols, "2")
					avail := available(ctx)
					for i, l := range RenderPlain(ctx) {
						if w := ctx.Style.Width(l); w > avail {
							t.Errorf("line %d is %d cells, available is %d:\n%q", i+1, w, avail, l)
						}
					}
				})
			}
		}
	}
}
