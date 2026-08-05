package style

import (
	"strings"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

// resolveProfile implements PRD §6.3's ordered resolution. First match wins.
//
// Note what is absent: any call to termenv.ColorProfile() or any check of
// whether stdout is a terminal. Claude Code always captures stdout, so every
// such check reports "not a terminal" and would resolve to no colour on a
// machine that displays colour perfectly well. See §6.5.
func resolveProfile(env map[string]string, cfg *config.Config) termenv.Profile {
	// 1. NO_COLOR — any value, per the no-color.org convention.
	if _, ok := env["NO_COLOR"]; ok {
		return termenv.Ascii
	}

	// 2. A terminal that cannot render escapes at all.
	//
	// Checked before COLORTERM deliberately: many shell profiles export
	// COLORTERM globally, so a `TERM=dumb` session would otherwise be told it
	// has truecolor by a variable inherited from a login script.
	term := env["TERM"]
	if term == "" || term == "dumb" {
		return termenv.Ascii
	}

	// 3. Explicit environment override.
	if p, ok := parseProfile(env["CC_STATUSLINE_COLOR"]); ok {
		return p
	}

	// 4. Explicit configuration.
	if p, ok := parseProfile(cfg.General.Color); ok {
		return p
	}

	// 5-7. Inference.
	switch strings.ToLower(env["COLORTERM"]) {
	case "truecolor", "24bit":
		return termenv.TrueColor
	}
	if strings.Contains(term, "256color") {
		return termenv.ANSI256
	}
	return termenv.ANSI
}

// parseProfile reads the shared vocabulary of CC_STATUSLINE_COLOR and
// general.color. "auto" is not a value here — it means "keep resolving", so it
// returns ok=false and the caller falls through.
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

// ProfileName renders a profile for `doctor` and for test failure messages.
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
