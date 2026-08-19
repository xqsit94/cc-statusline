package style

import (
	"strings"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

func resolveProfile(env map[string]string, cfg *config.Config) termenv.Profile {
	if _, ok := env["NO_COLOR"]; ok {
		return termenv.Ascii
	}

	term := env["TERM"]
	if term == "" || term == "dumb" {
		return termenv.Ascii
	}

	if p, ok := parseProfile(env["CC_STATUSLINE_COLOR"]); ok {
		return p
	}

	if p, ok := parseProfile(cfg.General.Color); ok {
		return p
	}

	switch strings.ToLower(env["COLORTERM"]) {
	case "truecolor", "24bit":
		return termenv.TrueColor
	}
	if strings.Contains(term, "256color") {
		return termenv.ANSI256
	}
	return termenv.ANSI
}

func parseProfile(v string) (termenv.Profile, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none":
		return termenv.Ascii, true
	case "16":
		return termenv.ANSI, true
	case "256":
		return termenv.ANSI256, true
	case "truecolor", "24bit":
		return termenv.TrueColor, true
	default:
		return termenv.Ascii, false
	}
}

func ProfileName(p termenv.Profile) string {
	switch p {
	case termenv.TrueColor:
		return "truecolor"
	case termenv.ANSI256:
		return "256"
	case termenv.ANSI:
		return "16"
	default:
		return "none"
	}
}
