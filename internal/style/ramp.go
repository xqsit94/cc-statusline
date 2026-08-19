package style

import (
	"fmt"
	"math"
)

type Ramp struct {
	stops []rgb
}

type rgb struct{ r, g, b float64 }

func NewRamp(hexes []string) Ramp {
	var r Ramp
	for _, h := range hexes {
		if c, ok := parseHex(h); ok {
			r.stops = append(r.stops, c)
		}
	}
	return r
}

func (r Ramp) Valid() bool { return len(r.stops) > 0 }

func (r Ramp) At(t float64) string {
	if len(r.stops) == 0 {
		return ""
	}
	if math.IsNaN(t) {
		t = 0
	}
	t = math.Min(math.Max(t, 0), 1)

	if len(r.stops) == 1 {
		return r.stops[0].hex()
	}

	pos := t * float64(len(r.stops)-1)
	i := int(pos)
	if i >= len(r.stops)-1 {
		return r.stops[len(r.stops)-1].hex()
	}
	return lerp(r.stops[i], r.stops[i+1], pos-float64(i)).hex()
}

func lerp(a, b rgb, f float64) rgb {
	return rgb{
		r: a.r + (b.r-a.r)*f,
		g: a.g + (b.g-a.g)*f,
		b: a.b + (b.b-a.b)*f,
	}
}

func (c rgb) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", channel(c.r), channel(c.g), channel(c.b))
}

func channel(v float64) int {
	return int(math.Min(math.Max(math.Round(v), 0), 255))
}

func parseHex(s string) (rgb, bool) {
	if len(s) != 7 || s[0] != '#' {
		return rgb{}, false
	}
	var v [3]float64
	for i := range v {
		hi, ok1 := hexDigit(s[1+i*2])
		lo, ok2 := hexDigit(s[2+i*2])
		if !ok1 || !ok2 {
			return rgb{}, false
		}
		v[i] = float64(hi*16 + lo)
	}
	return rgb{r: v[0], g: v[1], b: v[2]}, true
}

func hexDigit(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	}
	return 0, false
}
