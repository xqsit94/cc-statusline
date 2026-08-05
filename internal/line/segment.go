// Package line renders segments and assembles them into lines (PRD §4.3, §5.2).
//
// Three rules govern everything here:
//
//   - Segments never perform I/O. Every input arrives in Context, already
//     resolved. A segment that could read a file could also block the render.
//   - Empty means absent. A zero Rendered omits the segment and its adjacent
//     separator, rather than leaving a gap where data used to be.
//   - Segments know nothing about layout. Width, fitting, and truncation are
//     the joiner's and the fitter's business (fitting lands at M3).
package line

import (
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// Rendered carries the styled bytes and the unstyled text side by side.
//
// Width is measured from Plain, never from Styled. The alternative — stripping
// ANSI at measure time — means every width calculation depends on a regex
// keeping pace with every escape sequence a terminal understands, and it is
// wrong the first time someone emits an OSC 8 hyperlink.
type Rendered struct {
	Styled string
	Plain  string
}

// Empty reports absence. It tests Plain because Styled can hold escape
// sequences with no visible content — a reset sequence is not a segment.
func (r Rendered) Empty() bool { return r.Plain == "" }

// Segment is one unit of the status line.
type Segment interface {
	Name() string
	Render(ctx Context) Rendered
}

// Truncatable is implemented by segments that can shrink in place rather than
// disappearing. The fitter (M3) asks in ascending drop order.
type Truncatable interface {
	Truncate(r Rendered, cells int) Rendered
}

// Context is everything a segment may read.
type Context struct {
	Payload *payload.Payload
	Git     gitinfo.Info
	Config  *config.Config
	Style   *style.Style
}

// part is one substitution in a format string.
//
// A part is either plain text plus a colour key, or already-rendered content
// whose colouring is internal — the bar, whose cells differ from one another.
// The distinction matters for coalescing: adjacent plain parts sharing a colour
// merge into one escape span, opaque ones never merge.
type part struct {
	plain  string
	color  string
	opaque string // pre-rendered styled content; when set, plain is its text
}

// text builds a part from a plain string and a colour key.
func text(st *style.Style, colorKey, s string) part {
	return part{plain: s, color: colorKey}
}

// pre builds a part from already-rendered content.
func pre(r Rendered) part {
	return part{plain: r.Plain, opaque: r.Styled}
}

// expand substitutes {name} placeholders and nothing else.
//
// PRD §5.7 makes this the entire grammar: no padding syntax, no escapes, no
// conditionals. A template language here would be a second, worse config
// format that every segment then has to validate against.
//
// literalColor styles the text between placeholders — the `/` in `+150/-30`,
// the `%` after a percentage. An unknown placeholder is left verbatim so a typo
// is visible on screen rather than silently deleting content; `config` and
// `doctor` reject it properly at M3/M5.
func expand(st *style.Style, format, literalColor string, vars map[string]part) Rendered {
	var parts []part

	emitLiteral := func(s string) {
		if s == "" {
			return
		}
		// Whitespace is never painted. Wrapping a space in escapes emits
		// bytes that cannot be seen, doubles the size of the exact-escape
		// goldens in §9.2, and would paint a visible block the moment a theme
		// sets a background colour.
		if strings.TrimSpace(s) == "" {
			parts = append(parts, part{plain: s})
			return
		}
		parts = append(parts, part{plain: s, color: literalColor})
	}

	rest := format
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			emitLiteral(rest)
			break
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing < 0 {
			emitLiteral(rest)
			break
		}
		closing += open

		emitLiteral(rest[:open])
		if p, ok := vars[rest[open+1:closing]]; ok {
			parts = append(parts, p)
		} else {
			// An unknown placeholder is left verbatim so a typo is visible on
			// screen rather than silently deleting content. `config` and
			// `doctor` reject it properly at M3 and M5.
			emitLiteral(rest[open : closing+1])
		}
		rest = rest[closing+1:]
	}

	return assemble(st, parts)
}

// assemble renders parts, merging adjacent plain parts that share a colour.
//
// Without the merge, `${n}` emits two full escape spans for `$` and `0.85`
// where one would do. That is not just noise: the status line is rendered on
// every assistant turn, and §9.2's exact-escape goldens have to be read by a
// human when they change.
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
		// A part with nothing in it must not break a colour run. `{warn}` is
		// empty outside the danger band and carries the danger colour, so
		// without this `92%` and ` 1M` — same colour, adjacent — would emit two
		// spans with an empty one wedged between them.
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

// trimLeadingSpace removes the space a suppressed leading placeholder leaves
// behind — `{bar} {n}%` with the bar disabled must not render " 42%".
func trimLeadingSpace(r Rendered) Rendered {
	for strings.HasPrefix(r.Plain, " ") {
		r.Plain = r.Plain[1:]
		r.Styled = strings.TrimPrefix(r.Styled, " ")
	}
	return r
}
