package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Path resolves which file to read (PRD §7.1): CC_STATUSLINE_CONFIG if set,
// else $XDG_CONFIG_HOME/cc-statusline/config.toml, defaulting to
// ~/.config/cc-statusline/config.toml.
//
// An empty result means "no config file is locatable" — HOME unset in an
// `env -i` invocation, which §9.3 requires to work. That is not an error; it
// means the embedded defaults are the whole configuration.
func Path(env map[string]string) string {
	if p := strings.TrimSpace(env["CC_STATUSLINE_CONFIG"]); p != "" {
		return p
	}
	dir := strings.TrimSpace(env["XDG_CONFIG_HOME"])
	if dir == "" {
		home := strings.TrimSpace(env["HOME"])
		if home == "" {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "cc-statusline", "config.toml")
}

// Load builds the configuration for one render: PRD §7.1's overlay, embedded
// defaults → selected file → CC_STATUSLINE_* environment, then validated.
//
// It returns no error, by design. Every failure mode — an unreadable file, a
// syntax error, an unknown key, an out-of-range value — becomes a Defaulted
// note against a configuration that is still complete and renderable. The
// status line has nowhere to display an error, and refusing to render would
// present as a blank line the user cannot diagnose.
func Load(env map[string]string) (*Config, []Defaulted) {
	cfg := Defaults()
	notes := loadFile(cfg, Path(env))
	notes = append(notes, applyEnv(cfg, env)...)
	return cfg, append(notes, Validate(cfg)...)
}

// loadFile overlays path onto cfg, which must still hold the embedded defaults.
func loadFile(cfg *Config, path string) []Defaulted {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// The overwhelmingly common case: no config file, all defaults. Not a
		// note — nothing was defaulted that the user asked to be otherwise.
		return nil
	}
	if err != nil {
		return []Defaulted{{Key: "config file", Got: path, Reason: err.Error()}}
	}

	// Decoding happens into a scratch config that is committed only on success,
	// because unification applies keys as it walks: a type error halfway down
	// the file would otherwise leave half the user's config applied and half
	// defaulted, which is a state nobody wrote and nobody can reproduce.
	//
	// The two slice fields start nil. TOML arrays replace rather than merge,
	// but the decoder unifies an array of tables element-by-element against
	// whatever the slice already holds — so a file declaring one [[line]] would
	// have merged it onto the defaults' first line instead of replacing the
	// pair. Starting from nil makes replacement actual rather than incidental.
	scratch := Defaults()
	scratch.Lines = nil
	scratch.Colors.GradientStops = nil

	md, err := toml.Decode(string(b), scratch)
	if err != nil {
		return []Defaulted{{Key: "config file", Got: path,
			Reason: "not valid TOML: " + err.Error()}}
	}

	var notes []Defaulted
	// Undecoded keys are typos — `[genral]`, or `separater = "|"`. They are
	// silently ignored by every TOML decoder, which is exactly why they are
	// worth reporting: the user edited a file and nothing changed.
	for _, key := range md.Undecoded() {
		notes = append(notes, Defaulted{Key: key.String(), Got: "",
			Reason: "unknown key in " + path + "; ignored"})
	}

	def := Defaults()
	if scratch.Lines == nil {
		scratch.Lines = def.Lines
	}
	if scratch.Colors.GradientStops == nil {
		scratch.Colors.GradientStops = def.Colors.GradientStops
	}
	*cfg = *scratch
	return notes
}

// applyEnv is PRD §7.3's overlay.
//
// It handles exactly one variable, and the omission is deliberate. §7.3 also
// maps ASCII, NERDFONT, POWERLINE, COLOR, and NO_COLOR onto config keys, but
// §6.3 defines a resolution order that *interleaves* environment and config:
// NO_COLOR and TERM outrank CC_STATUSLINE_COLOR, which outranks general.color,
// which outranks COLORTERM. Folding the environment into the config first would
// flatten that order — general.color would win over the COLORTERM check that is
// specified to follow it. Those five resolve in internal/style, at the point
// where the whole order is visible in one function.
//
// CONFIG is handled by Path, above. NO_GIT is here because git.enabled has no
// such ordering: there is one config key and one variable, and either turning it
// off is the whole rule.
func applyEnv(cfg *Config, env map[string]string) []Defaulted {
	raw, ok := env["CC_STATUSLINE_NO_GIT"]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	on, known := Flexible(strings.ToLower(strings.TrimSpace(raw))).Bool()
	if !known {
		return []Defaulted{{Key: "CC_STATUSLINE_NO_GIT", Got: raw,
			Reason: fmt.Sprintf("want a boolean; git stays %v", cfg.Git.Enabled)}}
	}
	if on {
		cfg.Git.Enabled = false
	}
	return nil
}
