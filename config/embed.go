// Package presets ships the configuration files `cc-statusline init` installs.
//
// The .toml files beside this one are the artefact; this file exists only to
// make them reachable from a compiled binary. PRD §7.1 says the config folder
// "is not read at runtime", and that stays true — nothing here opens a file.
// But `go install github.com/xqsit94/cc-statusline@latest` leaves no repository
// on disk, so `init` has nowhere to copy a preset *from* unless the bytes are
// in the binary. Embedding is what makes both statements hold at once.
//
// //go:embed cannot reach across a parent directory, which is why this Go file
// lives in config/ rather than the presets living under internal/.
package presets

import (
	_ "embed"
)

// Default is the commented reference configuration. Every value in it is the
// embedded default, so installing it verbatim changes nothing — its purpose is
// to document what is configurable at the point where someone goes looking.
//
// TestDefaultPresetMatchesDefaults asserts it decodes to exactly
// config.Defaults(). Without that, the file users read and the behaviour they
// get would drift apart the first time a default changed.
//
//go:embed default.toml
var Default string

// Minimal is PRD §7.2's single-line preset.
//
//go:embed minimal.toml
var Minimal string

// ByName resolves a preset for `init --preset NAME`.
func ByName(name string) (string, bool) {
	switch name {
	case "", "default":
		return Default, true
	case "minimal":
		return Minimal, true
	default:
		return "", false
	}
}

// Names lists the shipped presets, for `init`'s usage text and the M7 wizard.
func Names() []string { return []string{"default", "minimal"} }
