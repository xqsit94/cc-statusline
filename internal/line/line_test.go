package line

import (
	"strconv"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

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
		state string
		want  []string
	}{
		{
			state: "normal-42",
			want: []string{
				"◆ Claude Opus 4.6 │ ▓▓▓▓░░░░░░ 42% │ $0.85 │ 3m │ 5h:15% 7d:8%",
				"⎇ main │ +150/-30 │ my-project",
			},
		},
		{
			state: "warning-75",
			want: []string{
				"◆ Claude Sonnet 4.6 │ ▓▓▓▓▓▓▓▓░░ 75% │ $3.20 │ 12m │ 5h:48%",
				"⎇ feat/auth │ +280/-45 │ my-project",
			},
		},
		{
			state: "danger-92",
			want: []string{
				"◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 45m │ 5h:85% 7d:62%",
				"⎇ main │ +500/-120 │ api-server",
			},
		},
		{
			state: "startup",
			want: []string{
				"◆ Claude Opus 4.6 │ ░░░░░░░░░░ 0% 1M │ $0.00",
				"claude-temp",
			},
		},
	}

	if len(cases) != len(refstate.References()) {
		t.Fatalf("%d cases against %d reference states", len(cases), len(refstate.References()))
	}

	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			env := map[string]string{"COLUMNS": "120"}
			assertLines(t, RenderPlain(ctxForState(t, tc.state, env)), tc.want)
		})
	}
}

func ctxForState(t *testing.T, name string, env map[string]string) Context {
	t.Helper()
	st, ok := refstate.ByName(name)
	if !ok {
		t.Fatalf("no reference state named %q; have %v", name, refstate.Names())
	}
	p, _ := payload.Parse(st.Payload)
	cfg := config.Defaults()
	return Context{
		Payload: p,
		Config:  cfg,
		Git:     st.Git,
		Style:   style.NewStyle(style.Detect(env, cfg), cfg),
	}
}

func TestReferenceStatesAtEighty(t *testing.T) {
	ctx := ctxForState(t, "danger-92", map[string]string{"COLUMNS": "80"})

	assertLines(t, RenderPlain(ctx), []string{
		"◆ Claude Opus 4.6 │ ▓▓▓▓▓▓▓▓▓░ 92% ⚠ 1M │ $15.30 │ 5h:85% 7d:62%",
		"⎇ main │ +500/-120 │ api-server",
	})
}

func TestASCIIReferenceState(t *testing.T) {
	ctx := ctxForState(t, "normal-42", map[string]string{
		"CC_STATUSLINE_ASCII": "1",
		"COLUMNS":             "120",
	})

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
	cases := []struct {
		tokens, size float64
		want         string
	}{
		{0, 200000, "░░░░░░░░░░"},
		{84000, 200000, "▓▓▓▓░░░░░░"},
		{150000, 200000, "▓▓▓▓▓▓▓▓░░"},
		{184000, 200000, "▓▓▓▓▓▓▓▓▓░"},
		{200000, 200000, "▓▓▓▓▓▓▓▓▓▓"},
		{250000, 200000, "▓▓▓▓▓▓▓▓▓▓"},
		{139200, 200000, "▓▓▓▓▓▓▓░░░"},
		{9000, 200000, "░░░░░░░░░░"},
		{11000, 200000, "▓░░░░░░░░░"},
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
	cfg := config.Defaults()
	cases := []struct {
		shown int
		want  string
	}{
		{0, "normal"}, {69, "normal"}, {70, "warning"},
		{84, "warning"}, {85, "danger"}, {100, "danger"},
	}
	for _, tc := range cases {
		got := config.BandColor(config.BandContext, tc.shown, cfg)
		if got != tc.want {
			t.Errorf("BandColor(context, %d) = %q, want %q", tc.shown, got, tc.want)
		}
	}
}

func TestSegmentAbsence(t *testing.T) {
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
		{"200000", "░░░░░░░░░░ 0%"},
		{"1000000", "░░░░░░░░░░ 0% 1M"},
		{"500000", "░░░░░░░░░░ 0% 500k"},
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

func TestEffortRendersTheLevelVerbatim(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		format  string
		want    string
	}{
		{"the default spells the label out", `{"effort":{"level":"high"}}`, "", "Effort: high"},
		{"the bare variant is just the word", `{"effort":{"level":"high"}}`, "{level}", "high"},
		{"the widest shipped level", `{"effort":{"level":"xhigh"}}`, "", "Effort: xhigh"},
		{"a level this build has never heard of", `{"effort":{"level":"glacial"}}`, "", "Effort: glacial"},

		{"no effort object at all", `{}`, "", ""},
		{"an effort object with no level", `{"effort":{}}`, "", ""},
		{"a level of nothing but spaces", `{"effort":{"level":"  "}}`, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := ctxFor(t, tc.payload, nil, "")
			if tc.format != "" {
				ctx.Config.Segments.Effort.Format = tc.format
			}
			got := effortSegment{}.Render(ctx)
			if got.Plain != tc.want {
				t.Errorf("got %q, want %q", got.Plain, tc.want)
			}
			if got.Empty() != (tc.want == "") {
				t.Errorf("Empty() = %v for %q", got.Empty(), got.Plain)
			}
		})
	}
}

func TestEffortTakesItsSeparatorWithIt(t *testing.T) {
	line := []config.SegmentRef{
		{Name: "model", Drop: config.NeverDrop},
		{Name: "effort", Drop: config.NeverDrop},
		{Name: "cost", Drop: config.NeverDrop},
	}

	with := ctxFor(t, `{"model":{"display_name":"M"},"effort":{"level":"max"},
		"cost":{"total_cost_usd":1}}`, nil, "")
	with.Config.Lines = []config.Line{{Segments: line}}
	assertLines(t, RenderPlain(with), []string{"◆ M │ Effort: max │ $1.00"})

	without := ctxFor(t, `{"model":{"display_name":"M"},"cost":{"total_cost_usd":1}}`, nil, "")
	without.Config.Lines = []config.Line{{Segments: line}}
	assertLines(t, RenderPlain(without), []string{"◆ M │ $1.00"})
}

func TestBranchTruncation(t *testing.T) {
	long := "feature/a-very-long-branch-name-indeed"
	ctx := ctxFor(t, `{"model":{"display_name":"M"}}`, nil, long)

	got := RenderPlain(ctx)
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
	ctx := ctxFor(t, `{"model":{"display_name":"M"},"cost":{"total_cost_usd":1}}`, nil, "")
	ctx.Config.Lines = []config.Line{{Segments: []config.SegmentRef{
		{Name: "model"}, {Name: "no-such-segment"}, {Name: "cost"},
	}}}
	assertLines(t, RenderPlain(ctx), []string{"◆ M │ $1.00"})
}

func TestUnknownPlaceholderIsVisible(t *testing.T) {
	ctx := ctxFor(t, `{"workspace":{"project_dir":"/p/proj"}}`, nil, "")
	ctx.Config.Segments.Project.Format = "{name}@{nonexistent}"
	if got := RenderPlain(ctx); got[0] != "proj@{nonexistent}" {
		t.Errorf("got %q", got[0])
	}
}

func TestEscapesAreCoalesced(t *testing.T) {
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
