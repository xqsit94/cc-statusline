package config

import "strings"

// Token is one piece of a format string.
//
// PRD §5.7 makes the grammar exactly `{name}` substitution: no padding syntax,
// no escapes, no conditionals. A template language here would be a second,
// worse config format that every segment then has to validate against.
type Token struct {
	Text        string // the literal text, or the placeholder name without braces
	Placeholder bool
}

// Tokenize splits a format string into literals and placeholders.
//
// It is the single implementation of §5.7's grammar, used by the validator and
// by internal/line's expander. Two implementations would be two grammars the
// moment one of them handled an unterminated `{` differently, and the symptom
// would be a format that validates cleanly and renders as literal braces.
//
// An unterminated `{` is literal text. That is deliberate: a user writing a
// shell snippet or a JSON fragment into a separator should see it rendered, not
// see the rest of their format string swallowed.
func Tokenize(format string) []Token {
	var out []Token
	rest := format

	literal := func(s string) {
		if s != "" {
			out = append(out, Token{Text: s})
		}
	}

	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			literal(rest)
			return out
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing < 0 {
			literal(rest)
			return out
		}
		closing += open

		literal(rest[:open])
		out = append(out, Token{Text: rest[open+1 : closing], Placeholder: true})
		rest = rest[closing+1:]
	}
}

// unknownPlaceholders returns the placeholders in format that are not in
// allowed, preserving the order they appear.
func unknownPlaceholders(format string, allowed []string) []string {
	var bad []string
	seen := map[string]bool{}
	for _, tok := range Tokenize(format) {
		if !tok.Placeholder || seen[tok.Text] {
			continue
		}
		seen[tok.Text] = true
		if !contains(allowed, tok.Text) {
			bad = append(bad, tok.Text)
		}
	}
	return bad
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
