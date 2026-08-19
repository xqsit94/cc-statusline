package style

import (
	"strconv"
	"strings"
)

func Rule(n int) string {
	switch {
	case n <= 0:
		return ""
	case n <= 4:
		return strings.Repeat("-", n)
	}
	label := " " + strconv.Itoa(n) + " "
	inner := n - 2
	if len(label) > inner {
		return "|" + strings.Repeat("-", inner) + "|"
	}
	left := (inner - len(label)) / 2
	right := inner - len(label) - left
	return "|" + strings.Repeat("-", left) + label + strings.Repeat("-", right) + "|"
}
