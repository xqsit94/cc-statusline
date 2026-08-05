package style

import "github.com/xqsit94/cc-statusline/internal/config"

// Glyphs is one column of PRD §6.2's table.
type Glyphs struct {
	ModelMarker  string
	BarFilled    string
	BarEmpty     string
	Separator    string
	PowerlineSep string
	Branch       string
	Danger       string
	Ellipsis     string
}

// glyphTable is PRD §6.2, transcribed. The Nerd Font column borrows `▓` and `░`
// from Unicode because the Nerd Font sets have no better bar cell — a patched
// font is a superset, not a replacement.
var glyphTable = map[IconSet]Glyphs{
	IconsASCII: {
		ModelMarker:  "*",
		BarFilled:    "#",
		BarEmpty:     "-",
		Separator:    "|",
		PowerlineSep: "", // no ASCII equivalent; Powerline is suppressed below
		Branch:       ">",
		Danger:       "!",
		Ellipsis:     ".",
	},
	IconsUnicode: {
		ModelMarker:  "◆",
		BarFilled:    "▓",
		BarEmpty:     "░",
		Separator:    "│",
		PowerlineSep: "",
		Branch:       "⎇",
		Danger:       "⚠",
		Ellipsis:     "…",
	},
	// Nerd Font codepoints are written as escapes rather than literals: they
	// live in the Private Use Area, so a literal renders as a replacement box
	// in any editor without a patched font and survives copy-paste badly.
	// PRD §12 Q1 settles the final selection at the M4 visual gate.
	IconsNerdFont: {
		ModelMarker:  "\U000F06A9", // nf-md-robot
		BarFilled:    "▓",
		BarEmpty:     "░",
		Separator:    "│",
		PowerlineSep: "\uE0B0", // nf-pl-left_hard_divider
		Branch:       "\uE725", // nf-dev-git_branch
		Danger:       "\uF071", // nf-fa-warning
		Ellipsis:     "…",
	},
}

// GlyphsFor returns the glyph column for an icon set, with the configured
// overrides applied.
//
// PRD §6.2: an explicitly-set bar.filled, bar.empty, or general.separator
// overrides the table for every icon set, but selecting ASCII does not rewrite
// an explicit value. The shipped presets use the sentinel "auto" precisely so
// that the icon set stays effective — a preset that hardcoded "▓" would silently
// defeat CC_STATUSLINE_ASCII=1.
func GlyphsFor(set IconSet, cfg *config.Config) Glyphs {
	g, ok := glyphTable[set]
	if !ok {
		g = glyphTable[IconsUnicode]
	}
	if cfg == nil {
		return g
	}

	if v := cfg.Bar.Filled; v != "" && v != "auto" {
		g.BarFilled = v
	}
	if v := cfg.Bar.Empty; v != "" && v != "auto" {
		g.BarEmpty = v
	}
	if v := cfg.General.Separator; v != "" && v != "auto" {
		g.Separator = v
	}
	return g
}

// emptyCellCandidates are the substitutes balanceBarCells may use, in order of
// preference. U+2592 MEDIUM SHADE is the important one: it is the only close
// visual relative of `░` that shares `▓`'s width class.
var emptyCellCandidates = []string{"▒", " "}

// balanceBarCells keeps the bar's filled and empty cells the same display
// width, substituting the empty cell when they disagree.
//
// # The bug this exists to prevent
//
// PRD §5.6 asserts that `▓ ░ ◆ ⚠` are East Asian Ambiguous. Measured against
// Unicode's EastAsianWidth.txt, two of the four are not, and two glyphs the
// list omits are:
//
//	▓ U+2593 DARK SHADE        A     ⚠ U+26A0 WARNING SIGN   N  ← claimed A
//	░ U+2591 LIGHT SHADE       N  ←  │ U+2502 BOX DRAWINGS   A  ← omitted
//	◆ U+25C6 BLACK DIAMOND     A     … U+2026 ELLIPSIS       A  ← omitted
//
// U+2591 is the odd one out among the shade blocks — U+2592 and U+2593 are both
// Ambiguous — and that single discrepancy is load-bearing. Under a CJK locale
// the filled cell occupies two columns and the empty cell one, so a ten-cell
// bar is ten columns at 0% and twenty at 100%. It does not merely look wrong:
// line 1 grows by up to ten columns over a session, so the fitter drops the
// cost and then the duration as the context fills, and segments disappear for
// a reason that has nothing to do with them.
//
// The whole Nerd Font column is Private Use Area, which is uniformly Ambiguous,
// so that column is already consistent and untouched here.
//
// Explicitly configured cells are left alone. §6.2 makes an explicit
// [bar].filled or [bar].empty override the table for every icon set, and
// silently substituting a glyph a user named would be a worse surprise than the
// wobble.
func balanceBarCells(g *Glyphs, cfg *config.Config, width func(string) int) {
	if cfg != nil && (explicitGlyph(cfg.Bar.Filled) || explicitGlyph(cfg.Bar.Empty)) {
		return
	}
	target := width(g.BarFilled)
	if width(g.BarEmpty) == target {
		return
	}
	for _, c := range emptyCellCandidates {
		if width(c) == target {
			g.BarEmpty = c
			return
		}
	}
	// Nothing matched. The wobble stands, which is still better than a cell
	// glyph nobody chose.
}

func explicitGlyph(v string) bool { return v != "" && v != "auto" }
