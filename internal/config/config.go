// Package config holds cc-statusline's configuration schema (PRD §7.2), loads
// it from TOML, and repairs it.
//
// Three rules govern everything here:
//
//   - Loading never fails. PRD §7.1 requires that an invalid config behave
//     identically in every command: silently default, and record what was
//     defaulted. A config error that blanked the status line would be
//     indistinguishable from a broken binary.
//   - The environment is a map, never os.Getenv. Same reason as internal/style
//     (PRD §6.4): M7's wizard has to resolve a preview under an environment it
//     is not running in.
//   - Format placeholders are enumerated once, in validate.go. Duplicating that
//     table is the one repetition that would drift silently — validation would
//     pass while render emitted nothing.
package config

import (
	"fmt"
	"strings"
)

// SegmentNames is every segment cc-statusline can render, in the order §5.3
// lists them.
//
// It lives here rather than in internal/line because the validator has to
// reject an unknown name and internal/line already imports this package. Line's
// registry is the implementation; this is the name list, and a test in that
// package asserts the two agree in both directions.
var SegmentNames = []string{
	"model", "context", "cost", "duration", "ratelimits",
	"branch", "diffstat", "project",
}

// Config mirrors the TOML schema in PRD §7.2 one-for-one.
type Config struct {
	General    General   `toml:"general"`
	Thresholds Threshold `toml:"thresholds"`
	Bar        Bar       `toml:"bar"`
	Git        Git       `toml:"git"`
	Context    Context   `toml:"context"`
	Colors     Colors    `toml:"colors"`
	Lines      []Line    `toml:"line"`
	Segments   Segments  `toml:"segments"`
}

// General holds PRD §7.2's [general] table.
//
// Powerline and AmbiguousWidth are Flexible rather than string because the
// schema accepts more than one TOML type for each — "auto" | true | false and
// "auto" | 1 | 2. See flexible.go.
type General struct {
	Separator       string   `toml:"separator"`
	Powerline       Flexible `toml:"powerline"`
	Icons           string   `toml:"icons"`
	Color           string   `toml:"color"`
	MaxWidth        int      `toml:"max_width"`
	WidthReserve    int      `toml:"width_reserve"`
	Padding         int      `toml:"padding"`
	RefreshInterval int      `toml:"refresh_interval"`
	AmbiguousWidth  Flexible `toml:"ambiguous_width"`
}

type Threshold struct {
	Warning       int `toml:"warning"`
	Danger        int `toml:"danger"`
	RateLimitWarn int `toml:"ratelimit_warn"`
}

type Bar struct {
	Enabled  bool   `toml:"enabled"`
	Width    int    `toml:"width"`
	Filled   string `toml:"filled"`
	Empty    string `toml:"empty"`
	Gradient bool   `toml:"gradient"`
}

type Git struct {
	Enabled      bool `toml:"enabled"`
	BranchMaxLen int  `toml:"branch_max_len"`
}

type Context struct {
	ShowSize string `toml:"show_size"`
}

type Colors struct {
	ModelMarker   string   `toml:"model_marker"`
	ModelName     string   `toml:"model_name"`
	Normal        string   `toml:"normal"`
	Warning       string   `toml:"warning"`
	Danger        string   `toml:"danger"`
	Cost          string   `toml:"cost"`
	Duration      string   `toml:"duration"`
	RateLimit     string   `toml:"ratelimit"`
	Branch        string   `toml:"branch"`
	Added         string   `toml:"added"`
	Removed       string   `toml:"removed"`
	Project       string   `toml:"project"`
	Separator     string   `toml:"separator"`
	DiffstatDelim string   `toml:"diffstat_delim"`
	BarEmpty      string   `toml:"bar_empty"`
	GradientStops []string `toml:"gradient_stops"`
}

// Line is one rendered row. The number of Line entries is the line count.
type Line struct {
	Segments []SegmentRef `toml:"segments"`
}

// SegmentRef names a segment and its drop priority. Higher drops first when the
// line overflows; 99 never drops; omitted defaults to 50. Priorities are scoped
// to their own line (PRD §7.2).
type SegmentRef struct {
	Name string `toml:"name"`
	Drop int    `toml:"drop"`
}

// UnmarshalTOML decodes one `{name="…", drop=N}` inline table.
//
// It is hand-written for one reason: an omitted `drop` must become 50, and
// struct decoding cannot express that. Without it, `{name="project"}` would
// decode to drop 0 — the lowest priority in the schema, so the segment the user
// declined to prioritise would become the last one standing.
//
// It also removes a subtler hazard. The decoder unifies an array of tables
// element-by-element against whatever is already in the slice, so a file with
// fewer segments than the defaults would have merged its entries onto unrelated
// ones. Because SegmentRef decodes itself from a clean state, that cannot
// happen regardless of what the defaults hold.
func (s *SegmentRef) UnmarshalTOML(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf(`want an inline table like {name="model", drop=99}; got %T`, v)
	}
	*s = SegmentRef{Drop: DefaultDrop}
	if name, ok := m["name"].(string); ok {
		s.Name = strings.ToLower(strings.TrimSpace(name))
	}
	switch d := m["drop"].(type) {
	case int64:
		s.Drop = int(d)
	case float64:
		s.Drop = int(d)
	}
	return nil
}

type Segments struct {
	Duration   DurationSeg   `toml:"duration"`
	RateLimits RateLimitsSeg `toml:"ratelimits"`
	Context    FormatSeg     `toml:"context"`
	Diffstat   FormatSeg     `toml:"diffstat"`
	Cost       FormatSeg     `toml:"cost"`
	Model      FormatSeg     `toml:"model"`
	Branch     FormatSeg     `toml:"branch"`
	Project    FormatSeg     `toml:"project"`
}

type FormatSeg struct {
	Format string `toml:"format"`
}

type DurationSeg struct {
	UnderHour string `toml:"under_hour"`
	OverHour  string `toml:"over_hour"`
	OverDay   string `toml:"over_day"`
	Pad       bool   `toml:"pad"`
}

type RateLimitsSeg struct {
	FiveFormat  string `toml:"five_format"`
	SevenFormat string `toml:"seven_format"`
	Join        string `toml:"join"`
}
