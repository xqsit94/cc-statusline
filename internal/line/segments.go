package line

import (
	"math"
	"strconv"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

type modelSegment struct{}

func (modelSegment) Name() string { return "model" }

func init() { register("model", func() Segment { return modelSegment{} }) }

func (m modelSegment) Render(ctx Context) Rendered {
	name, ok := ctx.Payload.ModelName()
	if !ok || strings.TrimSpace(name) == "" {
		return Rendered{}
	}
	return m.render(ctx, name)
}

func (modelSegment) render(ctx Context, name string) Rendered {
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Model.Format, "", map[string]part{
		"marker": text("model_marker", st.Glyphs.ModelMarker),
		"name":   text("model_name", name),
	})
}

const modelFloor = 10

func (m modelSegment) Truncate(ctx Context, cells int) Rendered {
	name, ok := ctx.Payload.ModelName()
	if !ok || strings.TrimSpace(name) == "" {
		return Rendered{}
	}
	full := m.render(ctx, name)
	st := ctx.Style

	overhead := st.Width(full.Plain) - st.Width(name)
	budget := max(cells, modelFloor) - overhead
	if budget <= 0 {
		return full
	}
	short := shorten(st, name, budget)
	if short == name {
		return full
	}
	return m.render(ctx, short)
}

func shorten(st *style.Style, text string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if st.Width(text) <= cells {
		return text
	}
	ell := st.Glyphs.Ellipsis
	if w := st.Width(ell); cells > w {
		return strings.TrimRight(st.TruncateCells(text, cells-w), " \t") + ell
	}
	return st.TruncateCells(text, cells)
}

type effortSegment struct{}

func (effortSegment) Name() string { return "effort" }

func init() { register("effort", func() Segment { return effortSegment{} }) }

func (effortSegment) Render(ctx Context) Rendered {
	level, ok := ctx.Payload.EffortLevel()
	if !ok || strings.TrimSpace(level) == "" {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Effort.Format, "effort", map[string]part{
		"level": text("effort", level),
	})
}

type contextSegment struct{}

func (contextSegment) Name() string { return "context" }

func init() { register("context", func() Segment { return contextSegment{} }) }

func (contextSegment) Render(ctx Context) Rendered {
	if !ctx.Payload.ContextPresent() {
		return Rendered{}
	}
	st := ctx.Style
	exact, _ := ctx.Payload.PercentExact()
	shown, _ := ctx.Payload.PercentShown()
	band := config.BandColor(config.BandContext, shown, ctx.Config)

	vars := map[string]part{
		"bar":  pre(renderBar(ctx, exact, band)),
		"n":    text(band, strconv.Itoa(shown)),
		"warn": text("danger", warnMarker(ctx, shown)),
		"size": text(band, sizeMarker(ctx)),
	}

	out := expand(st, ctx.Config.Segments.Context.Format, band, vars)
	if !ctx.Config.Bar.Enabled {
		out = trimLeadingSpace(out)
	}
	return out
}

func (c contextSegment) Truncate(ctx Context, cells int) Rendered {
	full := c.Render(ctx)
	if full.Empty() || !ctx.Config.Bar.Enabled || ctx.Config.Bar.Width <= 0 {
		return full
	}
	st := ctx.Style
	over := st.Width(full.Plain) - cells
	if over <= 0 {
		return full
	}

	cellWidth := max(1, st.Width(st.Glyphs.BarFilled))
	shrinkBy := (over + cellWidth - 1) / cellWidth

	if narrower := ctx.Config.Bar.Width - shrinkBy; narrower >= barFloor {
		return c.Render(withBar(ctx, true, narrower))
	}
	return c.Render(withBar(ctx, false, 0))
}

const barFloor = 3

func withBar(ctx Context, enabled bool, width int) Context {
	cfg := *ctx.Config
	cfg.Bar.Enabled = enabled
	cfg.Bar.Width = width
	ctx.Config = &cfg
	return ctx
}

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

	var styled strings.Builder
	switch {
	case filled == 0:
	case st.Gradient():
		ramp := st.Ramp()
		for i := range filled {
			t := (exact / 100) * float64(i+1) / float64(filled)
			styled.WriteString(st.PaintHex(ramp.At(t), st.Glyphs.BarFilled))
		}
	default:
		styled.WriteString(st.Paint(band, strings.Repeat(st.Glyphs.BarFilled, filled)))
	}
	styled.WriteString(st.Paint("bar_empty", strings.Repeat(st.Glyphs.BarEmpty, width-filled)))

	return Rendered{Styled: styled.String(), Plain: plain}
}

func warnMarker(ctx Context, shown int) string {
	if shown < ctx.Config.Thresholds.Danger {
		return ""
	}
	return " " + ctx.Style.Glyphs.Danger
}

func sizeMarker(ctx Context) string {
	size, ok := ctx.Payload.WindowSize()
	if !ok || size <= 0 {
		return ""
	}
	switch ctx.Config.Context.ShowSize {
	case "never":
		return ""
	case "always":
	default:
		if size == 200000 {
			return ""
		}
	}
	return " " + sizeLabel(size)
}

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

type costSegment struct{}

func (costSegment) Name() string { return "cost" }

func init() { register("cost", func() Segment { return costSegment{} }) }

func (costSegment) Render(ctx Context) Rendered {
	v, ok := ctx.Payload.CostUSD()
	if !ok {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Cost.Format, "cost", map[string]part{
		"n": text("cost", money(v)),
	})
}

type durationSegment struct{}

func (durationSegment) Name() string { return "duration" }

func init() { register("duration", func() Segment { return durationSegment{} }) }

func (durationSegment) Render(ctx Context) Rendered {
	d, ok := ctx.Payload.Duration()
	if !ok {
		return Rendered{}
	}
	total := int(d.Seconds())

	if total < 60 {
		return Rendered{}
	}

	cfg := ctx.Config.Segments.Duration
	st := ctx.Style
	days, hours, minutes := total/86400, total%86400/3600, total%3600/60

	minor := func(v int) string {
		if cfg.Pad && v < 10 {
			return "0" + strconv.Itoa(v)
		}
		return strconv.Itoa(v)
	}
	plain := func(v int) string { return strconv.Itoa(v) }

	var format string
	vars := map[string]part{
		"d": text("duration", plain(days)),
		"h": text("duration", plain(hours)),
		"m": text("duration", plain(minutes)),
	}
	switch {
	case total < 3600:
		format = cfg.UnderHour
	case total < 86400:
		format = cfg.OverHour
		vars["m"] = text("duration", minor(minutes))
	default:
		format = cfg.OverDay
		vars["h"] = text("duration", minor(hours))
	}

	return expand(st, format, "duration", vars)
}

type rateLimitsSegment struct{}

func (rateLimitsSegment) Name() string { return "ratelimits" }

func init() { register("ratelimits", func() Segment { return rateLimitsSegment{} }) }

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
		n := percent(v)
		color := config.BandColor(config.BandRateLimit, n, ctx.Config)
		parts = append(parts, expand(st, w.format, color, map[string]part{
			"n": text(color, strconv.Itoa(n)),
		}))
	}

	if len(parts) == 0 {
		return Rendered{}
	}
	return joinRendered(parts, Rendered{Styled: cfg.Join, Plain: cfg.Join})
}

type branchSegment struct{}

func (branchSegment) Name() string { return "branch" }

func init() { register("branch", func() Segment { return branchSegment{} }) }

func (b branchSegment) Render(ctx Context) Rendered {
	if !ctx.Config.Git.Enabled || !ctx.Git.Found || ctx.Git.Branch == "" {
		return Rendered{}
	}
	return b.render(ctx, truncateBranch(ctx.Git.Branch,
		ctx.Config.Git.BranchMaxLen, ctx.Style.Glyphs.Ellipsis))
}

func (branchSegment) render(ctx Context, name string) Rendered {
	st := ctx.Style
	body := expand(st, ctx.Config.Segments.Branch.Format, "branch", map[string]part{
		"name": text("branch", name),
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

const branchFloor = 8

func (b branchSegment) Truncate(ctx Context, cells int) Rendered {
	if !ctx.Config.Git.Enabled || !ctx.Git.Found || ctx.Git.Branch == "" {
		return Rendered{}
	}
	st := ctx.Style
	name := truncateBranch(ctx.Git.Branch, ctx.Config.Git.BranchMaxLen, st.Glyphs.Ellipsis)
	full := b.render(ctx, name)

	overhead := st.Width(full.Plain) - st.Width(name)
	budget := max(cells, branchFloor) - overhead
	if budget <= 0 {
		return full
	}
	short := shorten(st, name, budget)
	if short == name {
		return full
	}
	return b.render(ctx, short)
}

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

type diffstatSegment struct{}

func (diffstatSegment) Name() string { return "diffstat" }

func init() { register("diffstat", func() Segment { return diffstatSegment{} }) }

func (diffstatSegment) Render(ctx Context) Rendered {
	added, removed, ok := ctx.Payload.LinesChanged()
	if !ok || (added == 0 && removed == 0) {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Diffstat.Format, "diffstat_delim", map[string]part{
		"added":   text("added", strconv.Itoa(added)),
		"removed": text("removed", strconv.Itoa(removed)),
	})
}

type projectSegment struct{}

func (projectSegment) Name() string { return "project" }

func init() { register("project", func() Segment { return projectSegment{} }) }

func (projectSegment) Render(ctx Context) Rendered {
	name, ok := ctx.Payload.ProjectName()
	if !ok {
		return Rendered{}
	}
	st := ctx.Style
	return expand(st, ctx.Config.Segments.Project.Format, "project", map[string]part{
		"name": text("project", name),
	})
}
