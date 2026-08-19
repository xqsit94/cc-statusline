package line

import (
	"strconv"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/payload"
)

type rateWindow struct {
	name string
	key  payload.RateLimitKey
}

func (w rateWindow) Name() string { return w.name }

func (w rateWindow) formats(cfg *config.Config) (format, layout string) {
	s := cfg.Segments.RateLimit7d
	if w.key == payload.FiveHour {
		s = cfg.Segments.RateLimit5h
	}
	return s.Format, s.ResetFormat
}

func (w rateWindow) Render(ctx Context) Rendered {
	return w.render(ctx, true)
}

func (w rateWindow) render(ctx Context, withReset bool) Rendered {
	v, ok := ctx.Payload.RateLimitPercent(w.key)
	if !ok {
		return Rendered{}
	}
	st := ctx.Style
	format, layout := w.formats(ctx.Config)

	n := percent(v)
	color := config.BandColor(config.BandRateLimit, n, ctx.Config)

	icon, reset := "", ""
	if at, ok := ctx.Payload.RateLimitResetsAt(w.key); ok && withReset {
		icon = " " + st.Glyphs.Reset
		reset = " " + at.In(ctx.zone()).Format(layout)
	}

	return trimTrailingSpace(expand(st, format, color, map[string]part{
		"n":     text(color, strconv.Itoa(n)),
		"icon":  text("ratelimit", icon),
		"reset": text("ratelimit", reset),
	}))
}

func (w rateWindow) Truncate(ctx Context, cells int) Rendered {
	full := w.Render(ctx)
	if full.Empty() {
		return full
	}
	if ctx.Style.Width(full.Plain) <= cells {
		return full
	}
	short := w.render(ctx, false)
	if short.Empty() {
		return full
	}
	return short
}

func trimTrailingSpace(r Rendered) Rendered {
	for strings.HasSuffix(r.Plain, " ") {
		r.Plain = r.Plain[:len(r.Plain)-1]
		r.Styled = strings.TrimSuffix(r.Styled, " ")
	}
	return r
}

func rateLimit5h() Segment { return rateWindow{"ratelimit_5h", payload.FiveHour} }
func rateLimit7d() Segment { return rateWindow{"ratelimit_7d", payload.SevenDay} }

func init() {
	register("ratelimit_5h", rateLimit5h)
	register("ratelimit_7d", rateLimit7d)
}
