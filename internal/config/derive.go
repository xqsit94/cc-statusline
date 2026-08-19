package config

func (d SegmentDef) PresentationKeys() []Key {
	var out []Key
	for _, k := range d.Keys {
		if k.Syntax != SyntaxOpaque {
			out = append(out, k)
		}
	}
	return out
}

func (k Key) Path(segment string) string { return segmentKeyPath(segment, k.Name) }

func (d SegmentDef) Colors() []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, k := range d.Keys {
		for _, f := range k.Fields {
			for _, name := range BandColors(f.Band) {
				add(name)
			}
			add(f.Color)
		}
	}
	return out
}

func BandColors(band string) []string {
	switch band {
	case BandContext:
		return []string{"normal", "warning", "danger"}
	case BandRateLimit:
		return []string{"ratelimit", "warning"}
	default:
		return nil
	}
}

func BandColor(band string, shown int, cfg *Config) string {
	switch band {
	case BandContext:
		switch {
		case shown >= cfg.Thresholds.Danger:
			return "danger"
		case shown >= cfg.Thresholds.Warning:
			return "warning"
		default:
			return "normal"
		}
	case BandRateLimit:
		if shown >= cfg.Thresholds.RateLimitWarn {
			return "warning"
		}
		return "ratelimit"
	default:
		return ""
	}
}

func SegmentDefOf(name string) (SegmentDef, bool) {
	d, ok := segmentDefByName[name]
	return d, ok
}

var segmentDefByName = func() map[string]SegmentDef {
	m := make(map[string]SegmentDef, len(SegmentDefs))
	for _, d := range SegmentDefs {
		m[d.Name] = d
	}
	return m
}()

func derivedSegmentNames() []string {
	out := make([]string, 0, len(SegmentDefs))
	for _, d := range SegmentDefs {
		out = append(out, d.Name)
	}
	return out
}

func derivedColorKeys() []ColorKey {
	out := make([]ColorKey, 0, len(ColorDefs))
	for _, c := range ColorDefs {
		out = append(out, ColorKey{Name: c.Name, Get: c.Get, Set: c.Set})
	}
	return out
}

func derivedFormatKeys() []FormatKey {
	var out []FormatKey
	for _, d := range SegmentDefs {
		for _, k := range d.FormatKeyDefs() {
			names := make([]string, 0, len(k.Fields))
			for _, f := range k.Fields {
				names = append(names, f.Name)
			}
			out = append(out, FormatKey{
				Key:          k.Path(d.Name),
				Segment:      d.Name,
				Placeholders: names,
				Get:          k.Get,
				Set:          k.Set,
			})
		}
	}
	return out
}

func derivedTimeKeys() []TimeKey {
	var out []TimeKey
	for _, d := range SegmentDefs {
		for _, k := range d.TimeKeyDefs() {
			out = append(out, TimeKey{
				Key:     k.Path(d.Name),
				Segment: d.Name,
				Get:     k.Get,
				Set:     k.Set,
			})
		}
	}
	return out
}

func derivedVariants() map[string][]Variant {
	out := make(map[string][]Variant, len(SegmentDefs))
	for _, d := range SegmentDefs {
		keys := d.PresentationKeys()
		vs := make([]Variant, 0, len(d.Presentations))
		for _, p := range d.Presentations {
			var v Variant
			for i, value := range p {
				if i >= len(keys) {
					break
				}
				v = append(v, KeyValue{keys[i].Path(d.Name), value})
			}
			vs = append(vs, v)
		}
		out[d.Name] = vs
	}
	return out
}

func applyRegistryDefaults(c *Config) {
	for _, d := range ColorDefs {
		d.Set(c, d.Default)
	}
	for _, d := range SegmentDefs {
		for _, k := range d.Keys {
			if k.Set != nil {
				k.Set(c, k.Default)
			}
		}
	}
}
