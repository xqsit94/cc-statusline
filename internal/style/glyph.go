package style

import "github.com/xqsit94/cc-statusline/internal/config"

type Glyphs struct {
	ModelMarker  string
	BarFilled    string
	BarEmpty     string
	Separator    string
	PowerlineSep string
	Branch       string
	Danger       string
	Ellipsis     string
	Reset        string
}

var glyphTable = map[IconSet]Glyphs{
	IconsASCII: {
		ModelMarker:  "*",
		BarFilled:    "#",
		BarEmpty:     "-",
		Separator:    "|",
		PowerlineSep: "",
		Branch:       ">",
		Danger:       "!",
		Ellipsis:     ".",
		Reset:        "@",
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
		Reset:        "↻",
	},
	IconsNerdFont: {
		ModelMarker:  "\U000F06A9",
		BarFilled:    "▓",
		BarEmpty:     "░",
		Separator:    "│",
		PowerlineSep: "\uE0B0",
		Branch:       "\uE725",
		Danger:       "\uF071",
		Ellipsis:     "…",
		Reset:        "\uF021",
	},
}

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

var emptyCellCandidates = []string{"▒", " "}

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
}

func explicitGlyph(v string) bool { return v != "" && v != "auto" }
