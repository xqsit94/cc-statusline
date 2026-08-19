package style

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func (s *Style) Width(text string) int {
	return s.width.StringWidth(text)
}

func newWidthCondition(ambiguous int) *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = ambiguous == 2
	return c
}

func (s *Style) TruncateCells(text string, cells int) string {
	if cells <= 0 {
		return ""
	}
	if s.Width(text) <= cells {
		return text
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		w := s.width.RuneWidth(r)
		if used+w > cells {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

func (s *Style) ClipStyled(styled string, cells int) string {
	if cells < 0 {
		cells = 0
	}
	var b strings.Builder
	used, clipped := 0, false

	for i := 0; i < len(styled); {
		if styled[i] == 0x1b {
			n := escapeLen(styled[i:])
			b.WriteString(styled[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(styled[i:])
		w := s.width.RuneWidth(r)
		if used+w > cells {
			clipped = true
			break
		}
		b.WriteString(styled[i : i+size])
		used += w
		i += size
	}

	if clipped {
		b.WriteString(reset)
	}
	return b.String()
}

const reset = "\x1b[0m"

func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= '@' && s[i] <= '~' {
				return i + 1
			}
		}
		return len(s)
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
		}
		return len(s)
	default:
		return 2
	}
}
