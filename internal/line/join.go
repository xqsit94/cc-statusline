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

// Names lists every registered segment, for `doctor` and the M7 wizard.
//
// The list itself lives in config, because the validator has to reject an
// unknown segment name and this package already imports that one. New is the
// registry; config.SegmentNames is the vocabulary. TestRegistryMatchesConfig
// asserts they agree in both directions — a name in one and not the other is
// either a segment nobody can configure or a config key that renders nothing.
func Names() []string { return config.SegmentNames }

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
	var items []fitted
	for _, ref := range l.Segments {
		seg, ok := New(ref.Name)
		if !ok {
			continue
		}
		if r := seg.Render(ctx); !r.Empty() {
			items = append(items, fitted{ref: ref, seg: seg, r: r})
		}
	}
	if len(items) == 0 {
		return Rendered{}
	}
	return fit(ctx, items, available(ctx))
}

// separator is the styled ` │ ` between segments, or ` ` under Powerline.
//
// # What Powerline means here, and what it does not
//
// A full Powerline prompt fills each segment with a background colour and draws
// the arrow as the previous background against the next one, so the arrow reads
// as a seam between two solid blocks. That needs a background colour per
// segment and a contrasting foreground for the text on top — and PRD §7.2's
// [colors] table has neither. Inventing a palette here would mean choosing
// fifteen background colours and one text colour against a terminal background
// nobody has looked at yet, which is precisely the class of decision §9.4's
// visual gate exists to make.
//
// So this is the arrow as a separator glyph, in the separator colour: the shape
// of Powerline without the fills. It is a real style that real prompts use, and
// it is what CC_STATUSLINE_POWERLINE=1 delivers today. The filled variant is
// C-6 in §14.1, to be settled at M4 when someone is looking at a terminal.
//
// Style.ClipStyled already appends a reset unconditionally, so the filled
// variant would not need stage 3 revisited when it arrives.
func separator(ctx Context) Rendered {
	g := ctx.Style.Glyphs
	glyph := g.Separator
	if ctx.Style.Caps.Powerline && g.PowerlineSep != "" {
		glyph = g.PowerlineSep
	}
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
