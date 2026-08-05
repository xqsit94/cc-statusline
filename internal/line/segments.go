package line

import (
	"math"
	"strconv"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/payload"
)

// The eight segments of PRD §5.3, in line order.

// ── model ──────────────────────────────────────────────────────────────────

type modelSegment struct{}

func (modelSegment) Name() string { return "model" }

// Render produces `◆ {name}`. PRD §5.3 lists this as never empty, which holds
// for any real payload; with no display_name at all there is nothing to show,
// and cmd.Render's fallback covers that case rather than a lone marker.
func (modelSegment) Render(ctx Context) Rendered {
	name, ok := ctx.Payload.ModelName()
	if !ok || strings.TrimSpace(name) == "" {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Model.Format, "", map[string]part{
		"marker": text(st, "model_marker", st.Glyphs.ModelMarker),
		"name":   text(st, "model_name", name),
	})
}

// ── context ────────────────────────────────────────────────────────────────

type contextSegment struct{}

func (contextSegment) Name() string { return "context" }

// Render produces `{bar} {n}%{warn}{size}`.
//
// The bar and the percentage are one segment because the reference states join
// them with a single space while every other junction uses ` │ `, and there is
// exactly one global separator (PRD §5.3). They are also one visual unit: `⚠`
// and the size marker straddle what would otherwise be the boundary.
func (contextSegment) Render(ctx Context) Rendered {
	if !ctx.Payload.ContextPresent() {
		return Rendered{}
	}
	st := ctx.Style
	exact, _ := ctx.Payload.PercentExact()
	shown, _ := ctx.Payload.PercentShown()
	band := bandColor(shown, ctx.Config)

	vars := map[string]part{
		"bar":  pre(renderBar(ctx, exact, band)),
		"n":    text(st, band, strconv.Itoa(shown)),
		"warn": text(st, "danger", warnMarker(ctx, shown)),
		"size": text(st, band, sizeMarker(ctx)),
	}

	out := expand(st, ctx.Config.Segments.Context.Format, band, vars)
	if !ctx.Config.Bar.Enabled {
		out = trimLeadingSpace(out)
	}
	return out
}

// renderBar draws the fill. PRD §5.5.
//
// The fill comes from p_exact, never the rounded integer: Claude Code's
// used_percentage is already rounded, and inheriting that would quantise a
// second time. The difference is at most one cell, and only near a boundary.
//
// Colouring is the degraded branch of §5.5 — every filled cell takes the band
// colour. The fill-relative gradient arrives at M3; the shape is already final,
// so only the colour of existing cells changes.
func renderBar(ctx Context, exact float64, band string) Rendered {
	cfg := ctx.Config
	if !cfg.Bar.Enabled || cfg.Bar.Width <= 0 {
		return Rendered{}
	}
	st := ctx.Style
	width := cfg.Bar.Width

	filled := int(math.Round(exact * float64(width) / 100))
	filled = max(0, min(filled, width))

	plain := strings.Repeat(st.Glyphs.BarFilled, filled) +
		strings.Repeat(st.Glyphs.BarEmpty, width-filled)
	styled := st.Paint(band, strings.Repeat(st.Glyphs.BarFilled, filled)) +
		st.Paint("bar_empty", strings.Repeat(st.Glyphs.BarEmpty, width-filled))

	return Rendered{Styled: styled, Plain: plain}
}

// bandColor selects the colour key for a percentage.
//
// PRD §5.4 compares against p_shown — the rounded integer — specifically so the
// displayed number and its colour can never disagree. Comparing against p_exact
// would let 69.6 render as "70%" in the normal colour.
func bandColor(shown int, cfg *config.Config) string {
	switch {
	case shown >= cfg.Thresholds.Danger:
		return "danger"
	case shown >= cfg.Thresholds.Warning:
		return "warning"
	default:
		return "normal"
	}
}

// warnMarker renders " ⚠" in the danger band and "" otherwise. The leading
// space belongs to the marker, which is what lets one format string produce
// both `42%` and `92% ⚠ 1M` (PRD §5.3).
func warnMarker(ctx Context, shown int) string {
	if shown < ctx.Config.Thresholds.Danger {
		return ""
	}
	return " " + ctx.Style.Glyphs.Danger
}

// sizeMarker renders " 1M" when the context size should be shown.
//
// Under the default "non_default" it appears only for a window that is not
// 200000, and then in every state. Presence therefore carries information,
// instead of a marker that flickers on at some threshold.
func sizeMarker(ctx Context) string {
	size, ok := ctx.Payload.WindowSize()
	if !ok || size <= 0 {
		return ""
	}
	switch ctx.Config.Context.ShowSize {
	case "never":
		return ""
	case "always":
	default: // "non_default"
		if size == 200000 {
			return ""
		}
	}
	return " " + sizeLabel(size)
}

// sizeLabel formats a window size. PRD §5.7.
func sizeLabel(size float64) string {
	switch {
	case size == 1000000:
		return "1M"
	case size >= 1000:
		return strconv.Itoa(int(math.Round(size/1000))) + "k"
	default:
		return strconv.Itoa(int(size))
	}
}

// ── cost ───────────────────────────────────────────────────────────────────

type costSegment struct{}

func (costSegment) Name() string { return "cost" }

// Render produces `$0.85`.
//
// FormatFloat with 2 decimal places is doing real work: an observed session
// reported 107.43094200000006, and handing that to a default formatter would
// put the float's representation error on screen.
func (costSegment) Render(ctx Context) Rendered {
	v, ok := ctx.Payload.CostUSD()
	if !ok {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Cost.Format, "cost", map[string]part{
		"n": text(st, "cost", strconv.FormatFloat(v, 'f', 2, 64)),
	})
}

// ── duration ───────────────────────────────────────────────────────────────

type durationSegment struct{}

func (durationSegment) Name() string { return "duration" }

// Render produces `3m`, `1h5m`, or `2d3h`.
//
// Minute granularity is deliberate (PRD §5.1 delta b): a seconds digit ticking
// in peripheral vision contradicts §1's whole premise, and it would force a
// ten-second refresh interval to keep the digit honest.
func (durationSegment) Render(ctx Context) Rendered {
	d, ok := ctx.Payload.Duration()
	if !ok {
		return Rendered{}
	}
	total := int(d.Seconds())

	// Below a minute the segment is empty. total_duration_ms is wall-clock
	// since session start and is never truly zero at first render, so a
	// zero-minute reading is noise rather than information.
	if total < 60 {
		return Rendered{}
	}

	cfg := ctx.Config.Segments.Duration
	st := ctx.Style
	days, hours, minutes := total/86400, total%86400/3600, total%3600/60

	// pad zero-fills the *minor* unit only: `1h05m`, not `01h05m`. Which unit
	// is minor depends on the format, so the padding decision is made per
	// branch rather than per placeholder name.
	minor := func(v int) string {
		if cfg.Pad && v < 10 {
			return "0" + strconv.Itoa(v)
		}
		return strconv.Itoa(v)
	}
	plain := func(v int) string { return strconv.Itoa(v) }

	var format string
	vars := map[string]part{}
	switch {
	case total < 3600:
		// One unit, so nothing is minor and nothing is padded.
		format, vars["m"] = cfg.UnderHour, text(st, "duration", plain(minutes))
	case total < 86400:
		format = cfg.OverHour
		vars["h"] = text(st, "duration", plain(hours))
		vars["m"] = text(st, "duration", minor(minutes))
	default:
		format = cfg.OverDay
		vars["d"] = text(st, "duration", plain(days))
		vars["h"] = text(st, "duration", minor(hours))
	}

	return expand(st, format, "duration", vars)
}

// ── ratelimits ─────────────────────────────────────────────────────────────

type rateLimitsSegment struct{}

func (rateLimitsSegment) Name() string { return "ratelimits" }

// Render produces `5h:15% 7d:8%`.
//
// The windows are independent: a non-subscriber has neither, both are absent
// before a session's first response, and one can arrive without the other. An
// absent window contributes nothing *and drops its joiner*, so a lone 5-hour
// window renders `5h:15%` rather than `5h:15% `.
func (rateLimitsSegment) Render(ctx Context) Rendered {
	cfg := ctx.Config.Segments.RateLimits
	st := ctx.Style

	var parts []Rendered
	for _, w := range []struct {
		key    payload.RateLimitKey
		format string
	}{
		{payload.FiveHour, cfg.FiveFormat},
		{payload.SevenDay, cfg.SevenFormat},
	} {
		v, ok := ctx.Payload.RateLimitPercent(w.key)
		if !ok {
			continue
		}
		n := int(math.Round(v))
		color := "ratelimit"
		if n >= ctx.Config.Thresholds.RateLimitWarn {
			color = "warning"
		}
		parts = append(parts, expand(st, w.format, color, map[string]part{
			"n": text(st, color, strconv.Itoa(n)),
		}))
	}

	if len(parts) == 0 {
		return Rendered{}
	}
	return joinRendered(parts, Rendered{Styled: cfg.Join, Plain: cfg.Join})
}

// ── branch ─────────────────────────────────────────────────────────────────

type branchSegment struct{}

func (branchSegment) Name() string { return "branch" }

// Render produces `⎇ main`.
//
// The marker is not in the format string. PRD §7.2 gives [segments.branch] only
// `{name}`, so the glyph comes from the icon set and is prepended here — which
// is what makes CC_STATUSLINE_ASCII=1 turn it into `>` without the user editing
// their config.
func (branchSegment) Render(ctx Context) Rendered {
	if !ctx.Config.Git.Enabled || !ctx.Git.Found || ctx.Git.Branch == "" {
		return Rendered{}
	}
	st := ctx.Style
	name := truncateBranch(ctx.Git.Branch, ctx.Config.Git.BranchMaxLen, st.Glyphs.Ellipsis)

	body := expand(st, ctx.Config.Segments.Branch.Format, "branch", map[string]part{
		"name": text(st, "branch", name),
	})
	if body.Empty() {
		return Rendered{}
	}

	marker := st.Glyphs.Branch
	return Rendered{
		Styled: st.Paint("branch", marker) + " " + body.Styled,
		Plain:  marker + " " + body.Plain,
	}
}

// truncateBranch applies git.branch_max_len with the icon set's ellipsis.
// PRD §5.8: unconditional at render; §5.6's fitter may reduce further, to 8.
func truncateBranch(name string, maxLen int, ellipsis string) string {
	if maxLen <= 0 || name == "" {
		return name
	}
	runes := []rune(name)
	if len(runes) <= maxLen {
		return name
	}
	keep := maxLen - len([]rune(ellipsis))
	if keep < 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:keep]) + ellipsis
}

// ── diffstat ───────────────────────────────────────────────────────────────

type diffstatSegment struct{}

func (diffstatSegment) Name() string { return "diffstat" }

// Render produces `+150/-30`.
//
// The counts come from cost.total_lines_added / total_lines_removed, not from
// git (PRD §3.2). They are session-scoped, which is the more useful quantity
// here — and it is why the render path needs no subprocess at all.
func (diffstatSegment) Render(ctx Context) Rendered {
	added, removed, ok := ctx.Payload.LinesChanged()
	if !ok || (added == 0 && removed == 0) {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Diffstat.Format, "diffstat_delim", map[string]part{
		"added":   text(st, "added", strconv.Itoa(added)),
		"removed": text(st, "removed", strconv.Itoa(removed)),
	})
}

// ── project ────────────────────────────────────────────────────────────────

type projectSegment struct{}

func (projectSegment) Name() string { return "project" }

// Render produces the basename of workspace.project_dir.
func (projectSegment) Render(ctx Context) Rendered {
	name, ok := ctx.Payload.ProjectName()
	if !ok {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Project.Format, "project", map[string]part{
		"name": text(st, "project", name),
	})
}
