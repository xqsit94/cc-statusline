package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Flexible string

func (f *Flexible) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		*f = Flexible(strings.ToLower(strings.TrimSpace(t)))
	case bool:
		*f = Flexible(strconv.FormatBool(t))
	case int64:
		*f = Flexible(strconv.FormatInt(t, 10))
	case float64:
		*f = Flexible(strconv.FormatFloat(t, 'f', -1, 64))
	default:
		return fmt.Errorf("want a string, boolean, or integer; got %T", v)
	}
	return nil
}

func (f Flexible) is(alternatives ...string) bool {
	s := strings.ToLower(strings.TrimSpace(string(f)))
	for _, a := range alternatives {
		if s == a {
			return true
		}
	}
	return false
}

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
