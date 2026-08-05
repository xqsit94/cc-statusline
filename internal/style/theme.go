package style

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

// Style turns a colour key into escape sequences, using the capabilities
// resolved for this render.
type Style struct {
	Caps   Capabilities
	Glyphs Glyphs

	cfg      *config.Config
	renderer *lipgloss.Renderer
	cache    map[string]lipgloss.Style
	width    *runewidth.Condition
	ramp     Ramp
}

// NewStyle builds the renderer for one render.
//
// # PRD §6.5 — the highest-severity implementation trap in the project
//
// Claude Code captures stdout rather than connecting it to the terminal, so
// stdout is always a pipe. termenv's default ColorProfile() calls isatty(),
// sees the pipe, and returns Ascii. A faithful implementation of §6.3 would
// therefore resolve truecolor, hand hex values to Lipgloss, and emit zero
// escape sequences — presenting as "the colours just don't work", with no
// error anywhere to explain it.
//
// See newRenderer for what the fix actually is, which is not what §6.5
// originally prescribed.
//
// lipgloss.ColorProfile() and the package-level default renderer are prohibited
// codebase-wide. TestColorSurvivesAPipe and TestProfileIsHonoured catch
// regressions in each direction.
func NewStyle(c Capabilities, cfg *config.Config) *Style {
	if cfg == nil {
		cfg = config.Defaults()
	}
	cond := newWidthCondition(c.Ambiguous)
	glyphs := GlyphsFor(c.Icons, cfg)
	// Must run after the icon set is chosen and with this render's width
	// condition, because whether the bar's two cells agree depends on both.
	balanceBarCells(&glyphs, cfg, cond.StringWidth)

	return &Style{
		Caps:     c,
		Glyphs:   glyphs,
		cfg:      cfg,
		renderer: newRenderer(io.Discard, c.Profile),
		cache:    map[string]lipgloss.Style{},
		width:    cond,
		ramp:     NewRamp(cfg.Colors.GradientStops),
	}
}

// Gradient reports whether the bar's filled cells should be painted along the
// ramp rather than in one flat band colour (PRD §5.5).
//
// Truecolor is required, and not as a nicety: the ramp's whole content is the
// intermediate colours between stops, and a 256-colour downsample collapses
// neighbouring cells onto the same palette entry. What survives is a bar with
// three or four visible steps, which reads as a rendering fault rather than as
// a gradient. The band colour is the better degradation.
func (s *Style) Gradient() bool {
	return s.cfg.Bar.Gradient && s.Caps.Profile == termenv.TrueColor && s.ramp.Valid()
}

// Ramp is the configured gradient. Callers should test Gradient first.
func (s *Style) Ramp() Ramp { return s.ramp }

// newRenderer is the single place a lipgloss.Renderer is constructed.
//
// # Why this is not what PRD §6.5 originally prescribed
//
// §6.5 specified `lipgloss.NewRenderer(w, termenv.WithProfile(p),
// termenv.WithTTY(true))`. Measured against lipgloss v1.1.0, that is wrong in a
// way that is worse than the bug it was written to fix:
//
//	profile passed          escapes emitted
//	termenv.Ascii    →      \x1b[38;2;203;166;247m   ← must have been none
//	termenv.ANSI     →      \x1b[38;2;203;166;247m   ← must have been \x1b[95m
//	termenv.ANSI256  →      \x1b[38;2;203;166;247m   ← must have been 38;5;183
//	termenv.TrueColor→      \x1b[38;2;203;166;247m   ← correct by accident
//
// WithProfile configures the termenv Output. Lipgloss keeps its *own* profile
// field, initialised lazily, and never reads the one on the Output it was
// handed. So the profile resolved by §6.3 was silently discarded and every
// terminal got truecolor — meaning NO_COLOR and TERM=dumb would both have
// emitted colour, and a 16-colour terminal would have received sequences it
// cannot render.
//
// SetColorProfile writes lipgloss's own field and is the actual control. It is
// necessary and sufficient; the table above becomes correct in all four rows.
//
// WithTTY(true) is kept even though v1.1.0 no longer needs it once the profile
// is set explicitly. It disables the lazy isatty() detection rather than
// overriding its result, so it is the guard that keeps a future refactor — or a
// lipgloss version that restores lazy initialisation — from silently
// reintroducing the original §6.5 bug on a pipe.
//
// The writer is io.Discard because nothing here writes: Style only produces
// strings, and cmd.Render owns the one write to stdout (PRD §3.3). Passing
// os.Stdout would behave identically and would wrongly imply otherwise.
func newRenderer(w io.Writer, p termenv.Profile) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w, termenv.WithTTY(true))
	r.SetColorProfile(p)
	return r
}

// Paint styles text with a colour key from PRD §7.2's [colors] table.
//
// An unknown key returns the text unstyled rather than panicking or
// substituting a visible error colour: a colour is decoration, and a render
// path that can fail on decoration is a render path that can blank the line.
func (s *Style) Paint(key, text string) string {
	if text == "" {
		return ""
	}
	hex := s.Hex(key)
	if hex == "" {
		return text
	}
	return s.style(hex).Render(text)
}

// PaintHex styles text with a literal hex colour, for values computed at render
// time rather than named in the config — the gradient ramp at M3.
func (s *Style) PaintHex(hex, text string) string {
	if text == "" || hex == "" {
		return text
	}
	return s.style(hex).Render(text)
}

func (s *Style) style(hex string) lipgloss.Style {
	if st, ok := s.cache[hex]; ok {
		return st
	}
	st := s.renderer.NewStyle().Foreground(lipgloss.Color(hex))
	s.cache[hex] = st
	return st
}

// Hex resolves a colour key to its configured hex value.
//
// The key list lives in config.ColorKeys, not in a switch here. A key named in
// [colors] but missing from this function would render unpainted with no error
// anywhere — the same silent-decoration failure Paint is written to tolerate,
// which is precisely why it must not be possible to introduce by accident.
func (s *Style) Hex(key string) string {
	hex, _ := s.cfg.Color(key)
	return hex
}

// Colored reports whether this render will emit escape sequences at all.
// Golden tests for the plain layout assert this is false; the pipe test
// asserts it is true.
func (s *Style) Colored() bool { return s.Caps.Profile != termenv.Ascii }
