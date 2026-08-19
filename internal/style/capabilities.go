package style

import (
	"strconv"
	"strings"

	"github.com/muesli/termenv"
	"github.com/xqsit94/cc-statusline/internal/config"
)

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

type Capabilities struct {
	Icons     IconSet
	Powerline bool
	Profile   termenv.Profile
	Ambiguous int
	Columns   int
}

const defaultColumns = 80

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

func resolvePowerline(env map[string]string, cfg *config.Config) bool {
	icons := resolveIcons(env, cfg)

	if icons == IconsASCII {
		return false
	}

	if v, ok := env["CC_STATUSLINE_POWERLINE"]; ok && v != "" {
		return truthy(v)
	}
	if on, known := cfg.General.Powerline.Bool(); known {
		return on
	}
	return icons == IconsNerdFont
}

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

func Environ(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}
