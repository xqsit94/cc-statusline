// Package style resolves what the terminal can display and turns colour keys
// into escape sequences.
//
// Everything here is a pure function of an environment map plus a *config.Config
// (PRD §6.4). Nothing calls os.Getenv. That is not stylistic: M7's wizard must
// flip a live preview across three icon sets, two separator styles, four colour
// profiles, and arbitrary widths, and if resolution read the process
// environment the wizard could only do that by mutating its own environment.
package style

import (
	"strconv"
	"strings"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

// IconSet selects a column of PRD §6.2's glyph table.
type IconSet int

const (
	IconsASCII IconSet = iota
	IconsUnicode
	IconsNerdFont
)

func (s IconSet) String() string {
	switch s {
	case IconsASCII:
		return "ascii"
	case IconsNerdFont:
		return "nerdfont"
	default:
		return "unicode"
	}
}

// Capabilities is everything about the terminal that rendering depends on.
type Capabilities struct {
	Icons     IconSet
	Powerline bool
	Profile   termenv.Profile
	Ambiguous int // 1 or 2 cells for East Asian Ambiguous glyphs
	Columns   int
}

// defaultColumns applies when COLUMNS is unset and no max_width is configured.
// Claude Code exports COLUMNS from 2.1.153; below that this is the whole story.
const defaultColumns = 80

// Detect resolves capabilities from the environment and configuration.
func Detect(env map[string]string, cfg *config.Config) Capabilities {
	if cfg == nil {
		cfg = config.Defaults()
	}
	return Capabilities{
		Icons:     resolveIcons(env, cfg),
		Powerline: resolvePowerline(env, cfg),
		Profile:   resolveProfile(env, cfg),
		Ambiguous: resolveAmbiguous(env, cfg),
		Columns:   resolveColumns(env, cfg),
	}
}

// resolveIcons applies PRD §6.1: ASCII beats NERDFONT when both are set,
// because ASCII is the compatibility floor and a user who asked for it has a
// terminal that cannot show the alternative.
func resolveIcons(env map[string]string, cfg *config.Config) IconSet {
	if truthy(env["CC_STATUSLINE_ASCII"]) {
		return IconsASCII
	}
	if truthy(env["CC_STATUSLINE_NERDFONT"]) {
		return IconsNerdFont
	}
	switch strings.ToLower(cfg.General.Icons) {
	case "ascii":
		return IconsASCII
	case "nerdfont":
		return IconsNerdFont
	default:
		return IconsUnicode
	}
}

// resolvePowerline follows the icon set when unset, because Powerline
// separators need a patched font — the same font that supplies the Nerd Font
// glyphs. Turning them on without one renders replacement boxes.
func resolvePowerline(env map[string]string, cfg *config.Config) bool {
	icons := resolveIcons(env, cfg)

	// ASCII has no Powerline separator and no way to draw one. Honouring an
	// explicit POWERLINE=1 here would emit U+E0B0 into a terminal the user has
	// just told us cannot render Unicode at all.
	if icons == IconsASCII {
		return false
	}

	if v, ok := env["CC_STATUSLINE_POWERLINE"]; ok && v != "" {
		return truthy(v)
	}
	if on, known := cfg.General.Powerline.Bool(); known {
		return on
	}
	// "auto" — follows the icon set.
	return icons == IconsNerdFont
}

// resolveAmbiguous decides how wide `▓` `░` `◆` `⚠` are. They are East Asian
// Ambiguous, so a CJK locale renders them double-width and every width
// calculation downstream doubles with them.
func resolveAmbiguous(env map[string]string, cfg *config.Config) int {
	switch cfg.General.AmbiguousWidth.String() {
	case "1":
		return 1
	case "2":
		return 2
	}
	locale := env["LC_ALL"]
	if locale == "" {
		locale = env["LC_CTYPE"]
	}
	if locale == "" {
		locale = env["LANG"]
	}
	locale = strings.ToLower(locale)
	for _, cjk := range []string{"zh", "ja", "ko"} {
		if strings.HasPrefix(locale, cjk) {
			return 2
		}
	}
	return 1
}

// resolveColumns applies PRD §5.6. There is deliberately no ioctl: Claude Code
// captures stdout, so TIOCGWINSZ on fds 0/1/2 fails and `tput cols` cannot
// work. COLUMNS from the environment is the only source that exists.
func resolveColumns(env map[string]string, cfg *config.Config) int {
	if v := env["COLUMNS"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	if cfg.General.MaxWidth > 0 {
		return cfg.General.MaxWidth
	}
	return defaultColumns
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Environ converts a process environment into the map Detect consumes. PRD §6.4
// requires the environment be read exactly once, at the top of main.
func Environ(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}
