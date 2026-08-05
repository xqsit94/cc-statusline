package style

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// Width measures text in terminal cells (PRD §5.6).
//
// # Why a Condition and not the package-level functions
//
// go-runewidth exposes runewidth.StringWidth, which reads a package-global
// EastAsianWidth flag initialised from the process environment. Using it would
// break §6.4 twice over: capability resolution would stop being a pure function
// of the environment map, and M7's wizard could only preview a CJK locale by
// mutating its own process environment — a global change, in a program that
// also renders the non-CJK case in the same frame.
//
// A Condition is per-Style, so two Styles with different Ambiguous settings can
// measure the same string differently in the same process, which is exactly
// what the wizard needs.
//
// The caller must pass Rendered.Plain. Measuring Styled would count escape
// sequences as text; stripping them first is prohibited by §5.6, because it
// makes every width calculation depend on a regex keeping pace with every
// escape a terminal understands, and it is wrong the first time something emits
// an OSC 8 hyperlink.
func (s *Style) Width(text string) int {
	return s.width.StringWidth(text)
}

func newWidthCondition(ambiguous int) *runewidth.Condition {
	c := runewidth.NewCondition()
	// ▓ ░ ◆ ⚠ are East Asian Ambiguous. Under a CJK locale a terminal draws
	// them two cells wide, so a ten-cell bar occupies twenty and every fitting
	// decision downstream is made against a width that is half the truth.
	c.EastAsianWidth = ambiguous == 2
	return c
}

// TruncateCells shortens text to at most cells columns, at a rune boundary.
//
// It returns the text unchanged when it already fits, so a caller can use the
// result without comparing lengths. Nothing is appended: the ellipsis is a
// glyph and belongs to the segment that owns the icon set, not to a width
// helper.
func (s *Style) TruncateCells(text string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if s.Width(text) <= cells {
		return text
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		w := s.width.RuneWidth(r)
		if used+w > cells {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

// ClipStyled hard-clips styled text to at most cells columns and appends a
// reset. This is stage 3 of §5.6's fitter — what makes never-wrap a guarantee
// rather than an aspiration.
//
// It steps over ANSI escape sequences rather than counting them, which is a
// different operation from measuring: §5.6 prohibits stripping escapes to
// *measure* width, because the measurement would then depend on recognising
// every escape a terminal understands. Here recognition failure is bounded —
// an unrecognised sequence costs the cells it occupies in the output, not a
// wrong answer everywhere downstream — and there is no alternative, since
// something has to decide where in the byte stream the cut falls.
//
// The trailing reset is unconditional when anything was cut. A clip can land
// between a colour and its reset, or mid-Powerline-arrow with a background
// still active, and a terminal left in that state paints the rest of the row.
func (s *Style) ClipStyled(styled string, cells int) string {
	if cells < 0 {
		cells = 0
	}
	var b strings.Builder
	used, clipped := 0, false

	for i := 0; i < len(styled); {
		if styled[i] == 0x1b {
			n := escapeLen(styled[i:])
			b.WriteString(styled[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(styled[i:])
		w := s.width.RuneWidth(r)
		if used+w > cells {
			clipped = true
			break
		}
		b.WriteString(styled[i : i+size])
		used += w
		i += size
	}

	if clipped {
		b.WriteString(reset)
	}
	return b.String()
}

// reset is SGR 0. Written as a constant so the one place that emits an escape
// sequence by hand is greppable.
const reset = "\x1b[0m"

// escapeLen returns the byte length of the escape sequence starting at s[0],
// which the caller has already established is ESC.
//
// It recognises CSI and OSC, which covers everything this program emits and
// everything a colour library emits. An unrecognised introducer consumes two
// bytes — the ESC and one more — which is the shortest possible escape and
// therefore the reading least likely to swallow visible text.
func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[': // CSI: parameters, then a final byte in @–~
		for i := 2; i < len(s); i++ {
			if s[i] >= '@' && s[i] <= '~' {
				return i + 1
			}
		}
		return len(s)
	case ']': // OSC: terminated by BEL or ST (ESC \)
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	default:
		return 2
	}
}
