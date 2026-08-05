package line

import (
	"strconv"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// PRD §5.1 calls the reference states "byte-identical acceptance criteria for
// the default preset". These tests are that claim, executed. A change to any
// default in §7.2 is a change to the acceptance criteria and must fail here
// rather than pass quietly.

func ctxFor(t *testing.T, js string, env map[string]string, branch string) Context {
	t.Helper()
	p, _ := payload.Parse([]byte(js))
	cfg := config.Defaults()

	git := gitinfo.Info{}
	if branch != "" {
		git = gitinfo.Info{Found: true, Branch: branch, GitDir: "/synthetic/.git"}
	}
	return Context{
		Payload: p,
		Config:  cfg,
		Git:     git,
		Style:   style.NewStyle(style.Detect(env, cfg), cfg),
	}
}

func TestReferenceStates(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		branch  string
		want    []string
	}{
		{
			name: "normal — 42%, everything fine",
			payload: `{"model":{"display_name":"Claude Opus 4.6"},
				"workspace":{"project_dir":"/home/u/my-project"},
				"cost":{"total_cost_usd":0.85,"total_duration_ms":180000,
				        "total_lines_added":150,"total_lines_removed":30},
				"context_window":{"context_window_size":200000,"total_input_tokens":84000},
				"rate_limits":{"five_hour":{"used_percentage":15},
				               "seven_day":{"used_percentage":8}}}`,
			branch: "main",
			want: []string{
				"◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42% │ $0.85 │ 3m │ 5h:15% 7d:8%",
				"⎇ main │ +150/-30 │ my-project",
			},
		},
		{
			name: "warning — 75%, one rate limit window",
			payload: `{"model":{"display_name":"Claude Sonnet 4.6"},
				"workspace":{"project_dir":"/home/u/my-project"},
				"cost":{"total_cost_usd":3.20,"total_duration_ms":720000,
				        "total_lines_added":280,"total_lines_removed":45},
				"context_window":{"context_window_size":200000,"total_input_tokens":150000},
				"rate_limits":{"five_hour":{"used_percentage":48}}}`,
			branch: "feat/auth",
			want: []string{
				"◆ Claude Sonnet 4.6 │ ▓▓▓▓▓▓▓▓░░ 75% │ $3.20 │ 12m │ 5h:48%",
				"⎇ feat/auth │ +280/-45 │ my-project",
			},
		},
		{
			// 92% of 10 cells rounds to 9, not 10. No rounding rule produces
			// the original mockup's (4, 8, 10) triple — PRD §5.1 delta (a).
			name: "danger — 92%, warn marker and a 1M window",
			payload: `{"model":{"display_name":"Claude Opus 4.6"},
				"workspace":{"project_dir":"/home/u/api-server"},
				"cost":{"total_cost_usd":15.30,"total_duration_ms":2700000,
				        "total_lines_added":500,"total_lines_removed":120},
				"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
				"rate_limits":{"five_hour":{"used_percentage":85},
				               "seven_day":{"used_percentage":62}}}`,
			branch: "main",
			want: []string{
				"◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 45m │ 5h:85% 7d:62%",
				"⎇ main │ +500/-120 │ api-server",
			},
		},
		{
			// Every optional segment absent at once: no git, no diff, no rate
			// limits, and a duration below the one-minute floor. Line 2
			// collapses to the bare project name.
			name: "startup — clean, no noise",
			payload: `{"model":{"display_name":"Claude Opus 4.6"},
				"workspace":{"project_dir":"/home/u/claude-temp"},
				"cost":{"total_cost_usd":0,"total_duration_ms":4000,
				        "total_lines_added":0,"total_lines_removed":0},
				"context_window":{"context_window_size":1000000,
				                  "total_input_tokens":0,"used_percentage":null}}`,
			want: []string{
				"◆ Claude Opus 4.6 │ ░░░░░░░░░░ 0% 1M │ $0.00",
				"claude-temp",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// §5.1 states no width, and one of the four needs one.
			//
			// The danger state is 70 cells. `available` is
			// `COLUMNS - 2×padding - width_reserve`, so at the default COLUMNS
			// of 80 it is 68 — two cells short, and the fitter correctly drops
			// `45m`. The reference state as written is what appears from 82
			// columns up; TestReferenceStatesAtEighty records what happens
			// below that. This is a gap in §5.1, not in the fitter.
			env := map[string]string{"COLUMNS": "120"}
			got := RenderPlain(ctxFor(t, tc.payload, env, tc.branch))
			assertLines(t, got, tc.want)
		})
	}
}

// TestReferenceStatesAtEighty pins what the narrowest common terminal shows.
//
// 80 columns is the fallback when COLUMNS is unset, and §5.6's width_reserve of
// 12 leaves 68 cells. Three of the four reference states fit; the danger state
// is 70 and loses its duration. That is the fitter doing its job, and the point
// of writing it down is that the next person to see `45m` missing at 80 columns
// finds this test instead of filing a bug.
func TestReferenceStatesAtEighty(t *testing.T) {
	ctx := ctxFor(t, `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"project_dir":"/home/u/api-server"},
		"cost":{"total_cost_usd":15.30,"total_duration_ms":2700000,
		        "total_lines_added":500,"total_lines_removed":120},
		"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
		"rate_limits":{"five_hour":{"used_percentage":85},
		               "seven_day":{"used_percentage":62}}}`,
		map[string]string{"COLUMNS": "80"}, "main")

	assertLines(t, RenderPlain(ctx), []string{
		"◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 5h:85% 7d:62%",
		"⎇ main │ +500/-120 │ api-server",
	})
}

func TestASCIIReferenceState(t *testing.T) {
	// PRD §6.2's worked example. The point is that one environment variable
	// degrades every glyph — nothing in the shipped config hardcodes `▓`.
	ctx := ctxFor(t, `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"project_dir":"/home/u/my-project"},
		"cost":{"total_cost_usd":0.85,"total_duration_ms":180000,
		        "total_lines_added":150,"total_lines_removed":30},
		"context_window":{"context_window_size":200000,"total_input_tokens":84000},
		"rate_limits":{"five_hour":{"used_percentage":15},
		               "seven_day":{"used_percentage":8}}}`,
		map[string]string{"CC_STATUSLINE_ASCII": "1"}, "main")

	assertLines(t, RenderPlain(ctx), []string{
		"* Claude Opus 4.6 | ####------ 42% | $0.85 | 3m | 5h:15% 7d:8%",
		"> main | +150/-30 | my-project",
	})
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n got: %q\nwant: %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got: %q\nwant: %q", i+1, got[i], want[i])
		}
	}
}

func TestBarFill(t *testing.T) {
	// PRD §5.5. The fill comes from p_exact, never the rounded integer that
	// Claude Code reports — inheriting that would quantise a second time.
	cases := []struct {
		tokens, size float64
		want         string
	}{
		{0, 200000, "░░░░░░░░░░"},
		{84000, 200000, "▓▓▓▓░░░░░░"},  // 42.0 → 4
		{150000, 200000, "▓▓▓▓▓▓▓▓░░"}, // 75.0 → 8
		{184000, 200000, "▓▓▓▓▓▓▓▓▓░"}, // 92.0 → 9
		{200000, 200000, "▓▓▓▓▓▓▓▓▓▓"}, // 100  → 10
		{250000, 200000, "▓▓▓▓▓▓▓▓▓▓"}, // over-full clamps, never overflows
		{139200, 200000, "▓▓▓▓▓▓▓░░░"}, // 69.6 → 7 cells while p_shown is 70
		{9000, 200000, "░░░░░░░░░░"},   // 4.5 → round(0.45) = 0
		{11000, 200000, "▓░░░░░░░░░"},  // 5.5 → round(0.55) = 1
	}
	for _, tc := range cases {
		ctx := ctxFor(t, `{"context_window":{"context_window_size":`+
			ftoa(tc.size)+`,"total_input_tokens":`+ftoa(tc.tokens)+`}}`, nil, "")
		got := RenderPlain(ctx)
		if len(got) == 0 {
			t.Fatalf("%v/%v rendered nothing", tc.tokens, tc.size)
		}
		if bar, _, _ := strings.Cut(got[0], " "); bar != tc.want {
			t.Errorf("%v/%v: bar = %q, want %q", tc.tokens, tc.size, bar, tc.want)
		}
	}
}

func TestBandBoundaries(t *testing.T) {
	// PRD §5.4 compares against p_shown so the number and its colour can never
	// disagree. 69.6 displays as "70" and must therefore be in the warning
	// band, even though p_exact is below the threshold.
	cfg := config.Defaults()
	cases := []struct {
		shown int
		want  string
	}{
		{0, "normal"}, {69, "normal"}, {70, "warning"},
		{84, "warning"}, {85, "danger"}, {100, "danger"},
	}
	for _, tc := range cases {
		if got := bandColor(tc.shown, cfg); got != tc.want {
			t.Errorf("bandColor(%d) = %q, want %q", tc.shown, got, tc.want)
		}
	}
}

func TestSegmentAbsence(t *testing.T) {
	// PRD §4.3: a zero Rendered omits the segment AND its adjacent separator,
	// rather than leaving ` │  │ ` where data used to be.
	cases := []struct {
		name    string
		payload string
		want    []string
	}{
		{
			"no context window at all",
			`{"model":{"display_name":"M"},"cost":{"total_cost_usd":1}}`,
			[]string{"◆ M │ $1.00"},
		},
		{
			"duration below the one-minute floor",
			`{"model":{"display_name":"M"},"cost":{"total_cost_usd":1,"total_duration_ms":59999}}`,
			[]string{"◆ M │ $1.00"},
		},
		{
			"exactly one minute appears",
			`{"model":{"display_name":"M"},"cost":{"total_cost_usd":1,"total_duration_ms":60000}}`,
			[]string{"◆ M │ $1.00 │ 1m"},
		},
		{
			"a zero diffstat is absence, not +0/-0",
			`{"model":{"display_name":"M"},"workspace":{"project_dir":"/p/proj"},
			  "cost":{"total_lines_added":0,"total_lines_removed":0}}`,
			[]string{"◆ M", "proj"},
		},
		{
			"one rate limit window drops the joiner with it",
			`{"model":{"display_name":"M"},"rate_limits":{"seven_day":{"used_percentage":8}}}`,
			[]string{"◆ M │ 7d:8%"},
		},
		{
			"everything absent renders no lines at all",
			`{}`,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLines(t, RenderPlain(ctxFor(t, tc.payload, nil, "")), tc.want)
		})
	}
}

func TestDurationFormats(t *testing.T) {
	cases := []struct {
		ms   int
		want string
	}{
		{60000, "1m"}, {180000, "3m"}, {2700000, "45m"},
		{3600000, "1h0m"}, {3900000, "1h5m"},
		{86400000, "1d0h"}, {183600000, "2d3h"},
	}
	for _, tc := range cases {
		ctx := ctxFor(t, `{"cost":{"total_duration_ms":`+itoa(tc.ms)+`}}`, nil, "")
		got := RenderPlain(ctx)
		if len(got) == 0 || got[0] != tc.want {
			t.Errorf("%dms rendered %q, want %q", tc.ms, got, tc.want)
		}
	}

	t.Run("pad zero-fills only the minor unit", func(t *testing.T) {
		ctx := ctxFor(t, `{"cost":{"total_duration_ms":3900000}}`, nil, "")
		ctx.Config.Segments.Duration.Pad = true
		if got := RenderPlain(ctx); got[0] != "1h05m" {
			t.Errorf("padded = %q, want %q", got[0], "1h05m")
		}
	})
}

func TestSizeMarker(t *testing.T) {
	base := `{"context_window":{"context_window_size":%,"total_input_tokens":0}}`
	cases := []struct {
		size string
		want string
	}{
		{"200000", "░░░░░░░░░░ 0%"},      // the default window carries no marker
		{"1000000", "░░░░░░░░░░ 0% 1M"},  // 1M is spelled out
		{"500000", "░░░░░░░░░░ 0% 500k"}, // anything else is rounded to k
		{"128000", "░░░░░░░░░░ 0% 128k"},
	}
	for _, tc := range cases {
		ctx := ctxFor(t, strings.Replace(base, "%", tc.size, 1), nil, "")
		if got := RenderPlain(ctx); got[0] != tc.want {
			t.Errorf("size %s: %q, want %q", tc.size, got[0], tc.want)
		}
	}

	t.Run("show_size=never suppresses a non-default window", func(t *testing.T) {
		ctx := ctxFor(t, strings.Replace(base, "%", "1000000", 1), nil, "")
		ctx.Config.Context.ShowSize = "never"
		if got := RenderPlain(ctx); got[0] != "░░░░░░░░░░ 0%" {
			t.Errorf("got %q", got[0])
		}
	})

	t.Run("show_size=always marks the default window too", func(t *testing.T) {
		ctx := ctxFor(t, strings.Replace(base, "%", "200000", 1), nil, "")
		ctx.Config.Context.ShowSize = "always"
		if got := RenderPlain(ctx); got[0] != "░░░░░░░░░░ 0% 200k" {
			t.Errorf("got %q", got[0])
		}
	})
}

func TestBranchTruncation(t *testing.T) {
	long := "feature/a-very-long-branch-name-indeed"
	ctx := ctxFor(t, `{"model":{"display_name":"M"}}`, nil, long)

	got := RenderPlain(ctx)
	// branch_max_len is 24, and the ellipsis is one of those cells.
	if want := "⎇ feature/a-very-long-bra…"; got[1] != want {
		t.Errorf("branch = %q, want %q", got[1], want)
	}
	if n := len([]rune(strings.TrimPrefix(got[1], "⎇ "))); n != 24 {
		t.Errorf("truncated to %d runes, want 24", n)
	}

	t.Run("ASCII gets a dot, not an ellipsis", func(t *testing.T) {
		ctx := ctxFor(t, `{"model":{"display_name":"M"}}`,
			map[string]string{"CC_STATUSLINE_ASCII": "1"}, long)
		got := RenderPlain(ctx)[1]
		if want := "> feature/a-very-long-bra."; got != want {
			t.Errorf("branch = %q, want %q", got, want)
		}
	})

	t.Run("a short branch is untouched", func(t *testing.T) {
		ctx := ctxFor(t, `{"model":{"display_name":"M"}}`, nil, "main")
		if want := "⎇ main"; RenderPlain(ctx)[1] != want {
			t.Errorf("branch = %q, want %q", RenderPlain(ctx)[1], want)
		}
	})
}

func TestStyledAndPlainAgree(t *testing.T) {
	// Rendered.Plain drives every width calculation, so it must be exactly the
	// styled form with its escapes removed. If these drift, the fitter at M3
	// measures one string and the terminal displays another.
	ctx := ctxFor(t, `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"project_dir":"/home/u/proj"},
		"cost":{"total_cost_usd":15.30,"total_duration_ms":2700000,
		        "total_lines_added":500,"total_lines_removed":120},
		"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
		"rate_limits":{"five_hour":{"used_percentage":85}}}`,
		map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, "main")

	styled, plain := Render(ctx), RenderPlain(ctx)
	if len(styled) != len(plain) {
		t.Fatalf("styled has %d lines, plain has %d", len(styled), len(plain))
	}
	for i := range styled {
		if stripped := stripANSI(styled[i]); stripped != plain[i] {
			t.Errorf("line %d:\nstyled stripped: %q\n         plain: %q", i+1, stripped, plain[i])
		}
		if styled[i] == plain[i] {
			t.Errorf("line %d carries no escapes at all under truecolor", i+1)
		}
	}
}

// stripANSI removes SGR sequences. It exists only in tests: PRD §5.6 prohibits
// ANSI-stripping at measure time in production precisely because this function
// has to keep pace with every escape a terminal understands, and it is wrong
// the first time something emits an OSC 8 hyperlink.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestUnknownSegmentIsSkipped(t *testing.T) {
	// A typo in the config costs one segment, never the line.
	ctx := ctxFor(t, `{"model":{"display_name":"M"},"cost":{"total_cost_usd":1}}`, nil, "")
	ctx.Config.Lines = []config.Line{{Segments: []config.SegmentRef{
		{Name: "model"}, {Name: "no-such-segment"}, {Name: "cost"},
	}}}
	assertLines(t, RenderPlain(ctx), []string{"◆ M │ $1.00"})
}

func TestUnknownPlaceholderIsVisible(t *testing.T) {
	// Left verbatim rather than deleted: a typo the user can see is a typo the
	// user can fix. M3's validator rejects it before it ever renders.
	ctx := ctxFor(t, `{"workspace":{"project_dir":"/p/proj"}}`, nil, "")
	ctx.Config.Segments.Project.Format = "{name}@{nonexistent}"
	if got := RenderPlain(ctx); got[0] != "proj@{nonexistent}" {
		t.Errorf("got %q", got[0])
	}
}

func TestEscapesAreCoalesced(t *testing.T) {
	// `${n}` must not emit two full escape spans for `$` and the number, and
	// whitespace must never be painted at all — invisible bytes that bloat the
	// exact-escape goldens and would paint a block under a background theme.
	ctx := ctxFor(t, `{"cost":{"total_cost_usd":0.85}}`,
		map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, "")

	got := Render(ctx)[0]
	if want := "\x1b[38;2;73;222;128m$0.85\x1b[0m"; got != want {
		t.Errorf("cost = %q, want %q (one span, not two)", got, want)
	}

	ctx = ctxFor(t, `{"model":{"display_name":"M"},"cost":{"total_cost_usd":1}}`,
		map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, "")
	if line := Render(ctx)[0]; strings.Contains(line, "m \x1b[0m") {
		t.Errorf("a bare space was painted: %q", line)
	}
}

func TestNoGitDisablesTheBranch(t *testing.T) {
	ctx := ctxFor(t, `{"model":{"display_name":"M"},"workspace":{"project_dir":"/p/proj"}}`, nil, "main")
	ctx.Config.Git.Enabled = false
	assertLines(t, RenderPlain(ctx), []string{"◆ M", "proj"})
}

func itoa(v int) string     { return strconv.Itoa(v) }
func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
