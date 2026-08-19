package config

import (
	"fmt"
	"regexp"
	"strings"
)

type Override struct {
	Table string
	Key   string
	Value string
}

func ApplyOverrides(body string, overrides []Override) (string, []string) {
	lines := strings.Split(body, "\n")
	var applied []string

	for _, o := range overrides {
		pattern := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(o.Key) + `\s*=\s*).*$`)
		table := ""
		found := false
		for i, l := range lines {
			if t, ok := tableHeader(l); ok {
				table = t
				continue
			}
			if table != o.Table {
				continue
			}
			if m := pattern.FindStringSubmatch(l); m != nil {
				if lines[i] != m[1]+o.Value {
					applied = append(applied, o.Key+" = "+o.Value)
				}
				lines[i] = m[1] + o.Value
				found = true
				break
			}
		}
		if found {
			continue
		}
		for i, l := range lines {
			if t, ok := tableHeader(l); ok && t == o.Table {
				lines = append(lines[:i+1], append([]string{o.Key + " = " + o.Value}, lines[i+1:]...)...)
				applied = append(applied, o.Key+" = "+o.Value)
				found = true
				break
			}
		}
		if !found {
			lines = appendTable(lines, o)
			applied = append(applied, o.Key+" = "+o.Value)
		}
	}
	return strings.Join(lines, "\n"), applied
}

func appendTable(lines []string, o Override) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return append(lines, "", "["+o.Table+"]", o.Key+" = "+o.Value, "")
}

func tableHeader(l string) (string, bool) {
	t := strings.TrimSpace(l)
	if len(t) < 3 || t[0] != '[' || t[1] == '[' || t[len(t)-1] != ']' {
		return "", false
	}
	return t[1 : len(t)-1], true
}

func QuoteTOML(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

type ErrUnrecognisedLineRegion struct{ Reason string }

func (e ErrUnrecognisedLineRegion) Error() string {
	return "cannot rewrite the [[line]] blocks: " + e.Reason
}

func ReplaceLines(body string, rows []Line) (string, error) {
	src := strings.Split(body, "\n")
	start, end, err := lineRegion(src)
	if err != nil {
		return "", err
	}
	for _, l := range src[start:end] {
		t := strings.TrimSpace(l)
		switch {
		case t == "", t == "[[line]]", t == "]":
		case strings.HasPrefix(t, "segments"), strings.HasPrefix(t, "{name"):
		default:
			return "", ErrUnrecognisedLineRegion{
				Reason: fmt.Sprintf("it holds a line this cannot regenerate: %q", strings.TrimSpace(l)),
			}
		}
	}

	out := make([]string, 0, len(src)+8)
	out = append(out, src[:start]...)
	out = append(out, formatLines(rows)...)
	out = append(out, src[end:]...)
	return strings.Join(out, "\n"), nil
}

func lineRegion(src []string) (start, end int, err error) {
	start, end = -1, -1
	for i, l := range src {
		t := strings.TrimSpace(l)
		if t == "[[line]]" {
			if start < 0 {
				start = i
			}
			if end >= 0 {
				return 0, 0, ErrUnrecognisedLineRegion{
					Reason: "the [[line]] blocks are not contiguous",
				}
			}
			continue
		}
		if start >= 0 && end < 0 {
			if _, ok := tableHeader(l); ok || strings.HasPrefix(t, "[[") || strings.HasPrefix(t, "#") {
				end = i
				for end > start && strings.TrimSpace(src[end-1]) == "" {
					end--
				}
			}
		}
	}
	if start < 0 {
		return 0, 0, ErrUnrecognisedLineRegion{Reason: "there are no [[line]] blocks in the file"}
	}
	if end < 0 {
		end = len(src)
		for end > start && strings.TrimSpace(src[end-1]) == "" {
			end--
		}
	}
	return start, end, nil
}

func formatLines(rows []Line) []string {
	var out []string
	for i, row := range rows {
		if len(row.Segments) == 0 {
			continue
		}
		if i > 0 && len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, "[[line]]")
		out = append(out, "segments = [")
		cur := " "
		for _, s := range row.Segments {
			item := " " + formatSegmentRef(s) + ","
			if len(cur)+len(item) > 76 {
				out = append(out, cur)
				cur = " "
			}
			cur += item
		}
		if strings.TrimSpace(cur) != "" {
			out = append(out, cur)
		}
		out = append(out, "]")
	}
	return out
}

func formatSegmentRef(s SegmentRef) string {
	if s.IsFlex() {
		return fmt.Sprintf("{name=%q}", s.Name)
	}
	return fmt.Sprintf("{name=%q, drop=%d}", s.Name, s.Drop)
}
