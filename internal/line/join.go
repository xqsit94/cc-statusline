package line

import (
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
)

// New returns the segment registered under name.
//
// Segments are Go code, not configuration (PRD §1.1): the config selects,
// orders, and styles them but cannot define new ones. An unknown name yields
// ok=false, and Render skips it — a typo costs one segment, never the line.
func New(name string) (Segment, bool) {
	switch name {
	case "model":
		return modelSegment{}, true
	case "context":
		return contextSegment{}, true
	case "cost":
		return costSegment{}, true
	case "duration":
		return durationSegment{}, true
	case "ratelimits":
		return rateLimitsSegment{}, true
	case "branch":
		return branchSegment{}, true
	case "diffstat":
		return diffstatSegment{}, true
	case "project":
		return projectSegment{}, true
	default:
		return nil, false
	}
}

// Names lists every registered segment, for `doctor` and for the M3 validator.
func Names() []string {
	return []string{"model", "context", "cost", "duration", "ratelimits",
		"branch", "diffstat", "project"}
}

// Render assembles every configured line. PRD §5.2.
//
// A line whose segments are all empty is omitted entirely rather than emitted
// blank: the startup state has no branch, no diff, and only a project name, and
// a blank second row would cost a terminal line to display nothing.
func Render(ctx Context) []string {
	var out []string
	for _, l := range ctx.Config.Lines {
		if r := renderLine(ctx, l); !r.Empty() {
			out = append(out, r.Styled)
		}
	}
	return out
}

// RenderPlain assembles the unstyled text of every line, for width measurement
// and for the tier-1 goldens of PRD §9.2 — where colour is invisible and its
// absence keeps a hex-value change from rewriting every golden file.
func RenderPlain(ctx Context) []string {
	var out []string
	for _, l := range ctx.Config.Lines {
		if r := renderLine(ctx, l); !r.Empty() {
			out = append(out, r.Plain)
		}
	}
	return out
}

func renderLine(ctx Context, l config.Line) Rendered {
	var parts []Rendered
	for _, ref := range l.Segments {
		seg, ok := New(ref.Name)
		if !ok {
			continue
		}
		if r := seg.Render(ctx); !r.Empty() {
			parts = append(parts, r)
		}
	}
	if len(parts) == 0 {
		return Rendered{}
	}
	return joinRendered(parts, separator(ctx))
}

// separator is the styled ` │ ` between segments.
//
// Powerline separators are M3: they need per-segment background colours to
// look like anything other than a stray arrow, which is a change to how every
// segment renders rather than to how they are joined.
func separator(ctx Context) Rendered {
	glyph := ctx.Style.Glyphs.Separator
	return Rendered{
		Styled: " " + ctx.Style.Paint("separator", glyph) + " ",
		Plain:  " " + glyph + " ",
	}
}

// joinRendered concatenates parts with sep between them.
//
// Callers pass only non-empty parts, which is what implements PRD §4.3's rule
// that an absent segment takes its adjacent separator with it. Filtering here
// instead would work equally well; doing it at the call site keeps the reason
// visible where the emptiness is decided.
func joinRendered(parts []Rendered, sep Rendered) Rendered {
	var styled, plain strings.Builder
	for i, p := range parts {
		if i > 0 {
			styled.WriteString(sep.Styled)
			plain.WriteString(sep.Plain)
		}
		styled.WriteString(p.Styled)
		plain.WriteString(p.Plain)
	}
	return Rendered{Styled: styled.String(), Plain: plain.String()}
}
