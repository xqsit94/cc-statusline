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
// disappearing. Stage 2 of the fitter asks in ascending drop order (PRD §5.6).
//
// It takes the Context and re-renders, rather than taking the already-rendered
// output and cutting it down. Cutting would mean truncating a string that holds
// escape sequences, and the result would be either a lost colour or a lost
// reset — and there would be nowhere to put the ellipsis in the right colour. A
// segment shortening its own input is the only version that stays correct in
// both halves of Rendered.
//
// cells is the target width, not the amount to remove. A segment returns the
// narrowest rendering it is willing to produce, which may be wider than asked:
// §5.6 floors the branch at 8 cells and the model at 10, and stage 3 exists
// precisely because stage 2 is allowed to refuse.
type Truncatable interface {
	Truncate(ctx Context, cells int) Rendered
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
// The grammar is config.Tokenize's, not a second copy of it. PRD §5.7 requires
// the placeholder table be defined once and consumed by both the validator and
// every segment's renderer; two scanners would become two grammars the moment
// one of them handled an unterminated `{` differently, and the symptom would be
// a format that validates cleanly and renders as literal braces.
//
// literalColor styles the text between placeholders — the `/` in `+150/-30`,
// the `%` after a percentage.
//
// An unknown placeholder is left verbatim. It should be unreachable, because
// config.Validate has already replaced any format naming one; this is the
// defence for a Config built in code rather than loaded, where showing `{foo}`
// on screen beats silently deleting content.
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
