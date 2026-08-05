package style

import (
	"fmt"
	"math"
)

// Ramp is PRD §5.5's gradient: an ordered list of stops spaced evenly across
// [0,1], interpolated linearly in sRGB.
//
// # Why sRGB, named explicitly
//
// §5.5 specifies the colour space rather than leaving it to a library default,
// because the goldens are byte-identical. Interpolating the same stops in Lab
// or HSLuv produces different bytes for every intermediate cell — a perfectly
// defensible change that would rewrite every styled golden in the project. If
// the M4 visual gate decides sRGB's muddy midpoints are wrong, that is a
// deliberate change to a documented decision, not a silent one.
type Ramp struct {
	stops []rgb
}

type rgb struct{ r, g, b float64 }

// NewRamp builds a ramp from #rrggbb strings.
//
// Unparseable stops are skipped rather than treated as black: config.Validate
// has already replaced the whole list if any member was invalid, so reaching
// here with a bad value means a caller built a Config by hand. Painting a
// silent black cell would look like a rendering bug; skipping degrades to the
// stops that do parse, and to no gradient at all if none do.
func NewRamp(hexes []string) Ramp {
	var r Ramp
	for _, h := range hexes {
		if c, ok := parseHex(h); ok {
			r.stops = append(r.stops, c)
		}
	}
	return r
}

// Valid reports whether the ramp can produce a colour.
func (r Ramp) Valid() bool { return len(r.stops) > 0 }

// At returns the colour at position t, as #rrggbb. t is clamped to [0,1].
//
// A single stop makes At constant, which is the honest reading of a one-element
// gradient rather than an error: the user asked for one colour across the bar.
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

	// Stops are evenly spaced, so stop i sits at i/(n-1) and the position
	// scaled by n-1 is both the index and the fraction between neighbours.
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

// hex rounds half away from zero, matching §5.7's rule for the percentage. The
// alternative — Go's default truncation on a float-to-int conversion — biases
// every channel downward, which darkens the whole ramp by up to one level per
// channel for no reason anyone would be able to see or explain.
func (c rgb) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", channel(c.r), channel(c.g), channel(c.b))
}

func channel(v float64) int {
	return int(math.Min(math.Max(math.Round(v), 0), 255))
}

// parseHex accepts the #rrggbb form only, matching config.validHex. See the
// comment there for why the three-digit form is rejected.
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
