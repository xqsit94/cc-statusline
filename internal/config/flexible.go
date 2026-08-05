package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Flexible is a config value the schema deliberately allows to arrive as more
// than one TOML type.
//
// PRD §7.2 writes `powerline = "auto"` and `ambiguous_width = "auto"`, but the
// non-sentinel values are naturally a boolean and an integer. A user who writes
// `powerline = true` has not made a mistake, and neither has one who writes
// `powerline = "true"`. Rejecting either would be pedantry about TOML types in
// a file whose whole purpose is to be hand-edited.
//
// Normalising at decode time rather than at each use is what keeps a single
// representation downstream: every consumer reads a lowercase string, and no
// consumer has to know which TOML types its key happens to accept.
type Flexible string

// UnmarshalTOML implements toml.Unmarshaler.
//
// An unsupported type is an error rather than a silent coercion. Validate turns
// that error into a defaulted value and a recorded reason (PRD §7.1); swallowing
// it here would lose the reason and leave `doctor` with nothing to report.
func (f *Flexible) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		*f = Flexible(strings.ToLower(strings.TrimSpace(t)))
	case bool:
		*f = Flexible(strconv.FormatBool(t))
	case int64:
		*f = Flexible(strconv.FormatInt(t, 10))
	case float64:
		// TOML floats reach a Flexible only through a typo like
		// `ambiguous_width = 2.0`. Formatting with -1 precision keeps `2.0`
		// as "2", so the obvious intent still validates.
		*f = Flexible(strconv.FormatFloat(t, 'f', -1, 64))
	default:
		return fmt.Errorf("want a string, boolean, or integer; got %T", v)
	}
	return nil
}

// is reports whether the value equals any of the given alternatives,
// case-insensitively.
func (f Flexible) is(alternatives ...string) bool {
	s := strings.ToLower(strings.TrimSpace(string(f)))
	for _, a := range alternatives {
		if s == a {
			return true
		}
	}
	return false
}

// Bool interprets the value as a tri-state: the second result is false for
// "auto" and for anything unrecognised, which is what lets a caller fall back
// to its own default rather than guessing.
func (f Flexible) Bool() (value, ok bool) {
	switch {
	case f.is("true", "1", "yes", "on"):
		return true, true
	case f.is("false", "0", "no", "off"):
		return false, true
	default:
		return false, false
	}
}

func (f Flexible) String() string { return string(f) }
