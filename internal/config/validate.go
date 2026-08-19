package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Defaulted struct {
	Key    string
	Got    string
	Reason string
}

func (d Defaulted) String() string {
	return fmt.Sprintf("%s = %q: %s; using the default", d.Key, d.Got, d.Reason)
}

func Validate(cfg *Config) []Defaulted {
	def := Defaults()
	var out []Defaulted

	rec := func(key, got, reason string) {
		out = append(out, Defaulted{Key: key, Got: got, Reason: reason})
	}

	enum := func(key, got string, allowed []string, set func(string), fallback string) {
		norm := strings.ToLower(strings.TrimSpace(got))
		if contains(allowed, norm) {
			set(norm)
			return
		}
		rec(key, got, "want one of "+strings.Join(allowed, " | "))
		set(fallback)
	}

	within := func(key string, got, lo, hi int, set func(int), fallback int) {
		if got >= lo && got <= hi {
			return
		}
		rec(key, strconv.Itoa(got), fmt.Sprintf("want %d–%d", lo, hi))
		set(fallback)
	}

	g, dg := &cfg.General, def.General

	if strings.ContainsAny(g.Separator, "\r\n") {
		rec("general.separator", g.Separator, "must not contain a newline")
		g.Separator = dg.Separator
	}
	if !g.Powerline.is("auto") {
		if _, ok := g.Powerline.Bool(); !ok {
			rec("general.powerline", g.Powerline.String(), "want auto | true | false")
			g.Powerline = dg.Powerline
		}
	}
	enum("general.icons", g.Icons, []string{"ascii", "unicode", "nerdfont"},
		func(v string) { g.Icons = v }, dg.Icons)
	enum("general.color", g.Color, []string{"auto", "none", "16", "256", "truecolor"},
		func(v string) { g.Color = v }, dg.Color)
	enum("general.ambiguous_width", g.AmbiguousWidth.String(), []string{"auto", "1", "2"},
		func(v string) { g.AmbiguousWidth = Flexible(v) }, dg.AmbiguousWidth.String())

	within("general.max_width", g.MaxWidth, 0, 10000, func(v int) { g.MaxWidth = v }, dg.MaxWidth)
	within("general.width_reserve", g.WidthReserve, 0, 200, func(v int) { g.WidthReserve = v }, dg.WidthReserve)
	within("general.padding", g.Padding, 0, 100, func(v int) { g.Padding = v }, dg.Padding)
	within("general.refresh_interval", g.RefreshInterval, 0, 86400,
		func(v int) { g.RefreshInterval = v }, dg.RefreshInterval)

	t, dt := &cfg.Thresholds, def.Thresholds
	within("thresholds.warning", t.Warning, 0, 100, func(v int) { t.Warning = v }, dt.Warning)
	within("thresholds.danger", t.Danger, 0, 100, func(v int) { t.Danger = v }, dt.Danger)
	within("thresholds.ratelimit_warn", t.RateLimitWarn, 0, 100,
		func(v int) { t.RateLimitWarn = v }, dt.RateLimitWarn)

	if t.Warning > t.Danger {
		rec("thresholds", fmt.Sprintf("warning=%d danger=%d", t.Warning, t.Danger),
			"warning must not exceed danger, or the warning band is unreachable")
		t.Warning, t.Danger = dt.Warning, dt.Danger
	}

	b, db := &cfg.Bar, def.Bar
	within("bar.width", b.Width, 1, 200, func(v int) { b.Width = v }, db.Width)
	validateCell("bar.filled", &b.Filled, db.Filled, rec)
	validateCell("bar.empty", &b.Empty, db.Empty, rec)

	within("git.branch_max_len", cfg.Git.BranchMaxLen, 0, 1000,
		func(v int) { cfg.Git.BranchMaxLen = v }, def.Git.BranchMaxLen)

	enum("context.show_size", cfg.Context.ShowSize, []string{"non_default", "always", "never"},
		func(v string) { cfg.Context.ShowSize = v }, def.Context.ShowSize)

	for _, k := range ColorKeys {
		got := k.Get(cfg)
		if validHex(got) {
			k.Set(cfg, strings.ToLower(got))
			continue
		}
		rec("colors."+k.Name, got, "want a #rrggbb hex colour")
		k.Set(cfg, k.Get(def))
	}
	validateStops(cfg, def, rec)

	for _, k := range FormatKeys {
		got := k.Get(cfg)
		if bad := unknownPlaceholders(got, k.Placeholders); len(bad) > 0 {
			rec(k.Key, got, fmt.Sprintf("unknown placeholder {%s}; %s accepts {%s}",
				strings.Join(bad, "}, {"), k.Segment, strings.Join(k.Placeholders, "} {")))
			k.Set(cfg, k.Get(def))
		}
	}

	for _, k := range TimeKeys {
		if got := k.Get(cfg); !isTimeLayout(got) {
			rec(k.Key, got, "not a Go time layout; write the reference instant "+
				"itself — "+layoutHint)
			k.Set(cfg, k.Get(def))
		}
	}

	validateLines(cfg, def, rec)
	return out
}

const layoutHint = `"15:04" is hours and minutes, "2 Jan" is the day`

var layoutProbe = time.Date(2011, time.November, 18, 9, 34, 58, 0, time.UTC)

func isTimeLayout(s string) bool {
	return s != "" && layoutProbe.Format(s) != s
}

func validateCell(key string, value *string, fallback string, rec func(k, g, r string)) {
	v := *value
	if v == "" || v == "auto" {
		return
	}
	if n := len([]rune(v)); n != 1 {
		rec(key, v, fmt.Sprintf("want a single character or \"auto\"; got %d", n))
		*value = fallback
	}
}

func validateStops(cfg, def *Config, rec func(k, g, r string)) {
	stops := cfg.Colors.GradientStops
	bad := ""
	for _, s := range stops {
		if !validHex(s) {
			bad = s
			break
		}
	}
	if len(stops) == 0 || bad != "" {
		reason := "want at least one #rrggbb hex colour"
		if bad != "" {
			reason = fmt.Sprintf("%q is not a #rrggbb hex colour", bad)
		}
		rec("colors.gradient_stops", strings.Join(stops, ", "), reason)
		cfg.Colors.GradientStops = def.Colors.GradientStops
		return
	}
	for i, s := range stops {
		stops[i] = strings.ToLower(s)
	}
}

func validateLines(cfg, def *Config, rec func(k, g, r string)) {
	var lines []Line
	for i, l := range cfg.Lines {
		var refs []SegmentRef
		segments := 0
		for _, ref := range l.Segments {
			if ref.IsFlex() {
				ref.Drop = NeverDrop
				refs = append(refs, ref)
				continue
			}
			if !contains(SegmentNames, ref.Name) {
				rec(fmt.Sprintf("line[%d].segments", i), ref.Name,
					"unknown segment; known: "+strings.Join(SegmentNames, ", ")+", "+FlexName)
				continue
			}
			ref.Drop = max(0, min(ref.Drop, NeverDrop))
			refs = append(refs, ref)
			segments++
		}
		if segments > 0 {
			lines = append(lines, Line{Segments: refs})
		}
	}
	if len(lines) == 0 {
		if len(cfg.Lines) > 0 {
			rec("line", "", "no line declares a known segment")
		}
		lines = def.Lines
	}
	cfg.Lines = lines
}

func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
