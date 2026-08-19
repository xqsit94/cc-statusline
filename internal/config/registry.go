package config

import "strings"

type Kind uint8

const (
	KindText Kind = iota

	KindPercent

	KindMoney

	KindCount

	KindClock

	KindGauge

	KindGlyph
)

type Syntax uint8

const (
	SyntaxPlaceholders Syntax = iota

	SyntaxTimeLayout

	SyntaxOpaque
)

type Field struct {
	Name string
	Kind Kind

	Color string

	Band string
}

type Key struct {
	Name   string
	Syntax Syntax

	Fields []Field

	Default string

	Get func(*Config) string
	Set func(*Config, string)
}

type Presentation []string

type SegmentDef struct {
	Name string

	Doc string

	Tunes []string

	Keys []Key

	Presentations []Presentation
}

func (d SegmentDef) FormatKeyDefs() []Key { return d.keysWith(SyntaxPlaceholders) }

func (d SegmentDef) TimeKeyDefs() []Key { return d.keysWith(SyntaxTimeLayout) }

func (d SegmentDef) keysWith(s Syntax) []Key {
	var out []Key
	for _, k := range d.Keys {
		if k.Syntax == s {
			out = append(out, k)
		}
	}
	return out
}

type ColorDef struct {
	Name    string
	Default string
	Get     func(*Config) string
	Set     func(*Config, string)
}

func segmentKeyPath(segment, key string) string {
	return "segments." + segment + "." + key
}

func SplitKey(dotted string) (table, key string, ok bool) {
	i := strings.LastIndexByte(dotted, '.')
	if i <= 0 || i == len(dotted)-1 {
		return "", "", false
	}
	return dotted[:i], dotted[i+1:], true
}
