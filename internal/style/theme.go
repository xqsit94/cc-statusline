package style

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

type Style struct {
	Caps   Capabilities
	Glyphs Glyphs

	cfg      *config.Config
	renderer *lipgloss.Renderer
	cache    map[string]lipgloss.Style
	width    *runewidth.Condition
	ramp     Ramp
}

func NewStyle(c Capabilities, cfg *config.Config) *Style {
	if cfg == nil {
		cfg = config.Defaults()
	}
	cond := newWidthCondition(c.Ambiguous)
	glyphs := GlyphsFor(c.Icons, cfg)
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

func (s *Style) Gradient() bool {
	return s.cfg.Bar.Gradient && s.Caps.Profile == termenv.TrueColor && s.ramp.Valid()
}

func (s *Style) Ramp() Ramp { return s.ramp }

func newRenderer(w io.Writer, p termenv.Profile) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w, termenv.WithTTY(true))
	r.SetColorProfile(p)
	return r
}

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

func (s *Style) Hex(key string) string {
	hex, _ := s.cfg.Color(key)
	return hex
}

func (s *Style) Colored() bool { return s.Caps.Profile != termenv.Ascii }
