package line

import (
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

type Rendered struct {
	Styled string
	Plain  string
}

func (r Rendered) Empty() bool { return r.Plain == "" }

type Segment interface {
	Name() string
	Render(ctx Context) Rendered
}

type Truncatable interface {
	Truncate(ctx Context, cells int) Rendered
}

type Context struct {
	Payload *payload.Payload
	Git     gitinfo.Info
	Config  *config.Config
	Style   *style.Style
	Zone    *time.Location
}

func (c Context) zone() *time.Location {
	if c.Zone == nil {
		return time.Local
	}
	return c.Zone
}

type part struct {
	plain  string
	color  string
	opaque string
}

func text(colorKey, s string) part {
	return part{plain: s, color: colorKey}
}

func pre(r Rendered) part {
	return part{plain: r.Plain, opaque: r.Styled}
}

func expand(st *style.Style, format, literalColor string, vars map[string]part) Rendered {
	var parts []part

	emitLiteral := func(s string) {
		if s == "" {
			return
		}
		if strings.TrimSpace(s) == "" {
			parts = append(parts, part{plain: s})
			return
		}
		parts = append(parts, part{plain: s, color: literalColor})
	}

	for _, tok := range config.Tokenize(format) {
		switch {
		case !tok.Placeholder:
			emitLiteral(tok.Text)
		default:
			if p, ok := vars[tok.Text]; ok {
				parts = append(parts, p)
			} else {
				emitLiteral("{" + tok.Text + "}")
			}
		}
	}

	return assemble(st, parts)
}

func assemble(st *style.Style, parts []part) Rendered {
	var styled, plain strings.Builder

	flush := func(text, color string) {
		if text == "" {
			return
		}
		if color == "" {
			styled.WriteString(text)
			return
		}
		styled.WriteString(st.Paint(color, text))
	}

	var pending, pendingColor string
	for _, p := range parts {
		if p.plain == "" && p.opaque == "" {
			continue
		}
		plain.WriteString(p.plain)

		if p.opaque != "" {
			flush(pending, pendingColor)
			pending, pendingColor = "", ""
			styled.WriteString(p.opaque)
			continue
		}
		if pending != "" && p.color == pendingColor {
			pending += p.plain
			continue
		}
		flush(pending, pendingColor)
		pending, pendingColor = p.plain, p.color
	}
	flush(pending, pendingColor)

	return Rendered{Styled: styled.String(), Plain: plain.String()}
}

func trimLeadingSpace(r Rendered) Rendered {
	for strings.HasPrefix(r.Plain, " ") {
		r.Plain = r.Plain[1:]
		r.Styled = strings.TrimPrefix(r.Styled, " ")
	}
	return r
}
