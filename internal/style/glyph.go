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
