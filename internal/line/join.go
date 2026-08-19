package line

import (
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
)

var (
	builders = map[string]func() Segment{}

	duplicates []string
)

func register(name string, build func() Segment) {
	if _, seen := builders[name]; seen {
		duplicates = append(duplicates, name)
		return
	}
	builders[name] = build
}

func New(name string) (Segment, bool) {
	build, ok := builders[name]
	if !ok {
		return nil, false
	}
	return build(), true
}

func Render(ctx Context) []string {
	var out []string
	for _, l := range ctx.Config.Lines {
		if r := renderLine(ctx, l); !r.Empty() {
			out = append(out, r.Styled)
		}
	}
	return out
}

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
	out, _, _ := layout(ctx, l)
	return out
}

func layout(ctx Context, l config.Line) (out Rendered, built, kept []fitted) {
	var items []fitted
	for i, ref := range l.Segments {
		if ref.IsFlex() {
			items = append(items, fitted{idx: i, ref: ref, flex: true})
			continue
		}
		seg, ok := New(ref.Name)
		if !ok {
			continue
		}
		if r := seg.Render(ctx); !r.Empty() {
			items = append(items, fitted{idx: i, ref: ref, seg: seg, r: r})
		}
	}
	if len(items) == 0 {
		return Rendered{}, nil, nil
	}
	out, kept = fit(ctx, items, available(ctx))
	return out, items, kept
}

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
