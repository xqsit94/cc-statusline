package config

import "strings"

type Token struct {
	Text        string
	Placeholder bool
}

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
