package style

import (
	"strconv"
)

type Overrides struct {
	Icons     string
	Separator string
	Colour    string
	Columns   int
}

func Overlay(env map[string]string, o Overrides) map[string]string {
	e := make(map[string]string, len(env)+4)
	for k, v := range env {
		e[k] = v
	}
	switch o.Icons {
	case "ascii":
		e["CC_STATUSLINE_ASCII"] = "1"
		delete(e, "CC_STATUSLINE_NERDFONT")
	case "unicode":
		delete(e, "CC_STATUSLINE_ASCII")
		delete(e, "CC_STATUSLINE_NERDFONT")
	case "nerdfont":
		delete(e, "CC_STATUSLINE_ASCII")
		e["CC_STATUSLINE_NERDFONT"] = "1"
	}
	switch o.Separator {
	case "plain":
		e["CC_STATUSLINE_POWERLINE"] = "0"
	case "powerline":
		e["CC_STATUSLINE_POWERLINE"] = "1"
	}
	if o.Colour != "" {
		e["CC_STATUSLINE_COLOR"] = o.Colour
	}
	if o.Columns > 0 {
		e["COLUMNS"] = strconv.Itoa(o.Columns)
	}
	return e
}
