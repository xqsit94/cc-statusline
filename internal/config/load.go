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

func Load(env map[string]string) (*Config, []Defaulted) {
	cfg := Defaults()
	notes := loadFile(cfg, Path(env))
	notes = append(notes, applyEnv(cfg, env)...)
	return cfg, append(notes, Validate(cfg)...)
}

func loadFile(cfg *Config, path string) []Defaulted {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Defaulted{{Key: "config file", Got: path, Reason: err.Error()}}
	}
	decoded, notes := Decode(string(b), path)
	*cfg = *decoded
	return notes
}

func Decode(body, source string) (*Config, []Defaulted) {
	scratch := Defaults()
	scratch.Lines = nil
	scratch.Colors.GradientStops = nil

	md, err := toml.Decode(body, scratch)
	if err != nil {
		return Defaults(), []Defaulted{{Key: "config file", Got: source,
			Reason: "not valid TOML: " + err.Error()}}
	}

	var notes []Defaulted
	for _, key := range md.Undecoded() {
		notes = append(notes, Defaulted{Key: key.String(), Got: "",
			Reason: "unknown key in " + source + "; ignored"})
	}

	def := Defaults()
	if scratch.Lines == nil {
		scratch.Lines = def.Lines
	}
	if scratch.Colors.GradientStops == nil {
		scratch.Colors.GradientStops = def.Colors.GradientStops
	}
	return scratch, notes
}

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
