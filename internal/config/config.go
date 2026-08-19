package config

import (
	"fmt"
	"strings"
)

var SegmentNames = derivedSegmentNames()

const FlexName = "flex"

func (s SegmentRef) IsFlex() bool { return s.Name == FlexName }

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
	Effort        string   `toml:"effort"`
	Branch        string   `toml:"branch"`
	Added         string   `toml:"added"`
	Removed       string   `toml:"removed"`
	Project       string   `toml:"project"`
	Separator     string   `toml:"separator"`
	DiffstatDelim string   `toml:"diffstat_delim"`
	BarEmpty      string   `toml:"bar_empty"`
	GradientStops []string `toml:"gradient_stops"`
}

type Line struct {
	Segments []SegmentRef `toml:"segments"`
}

type SegmentRef struct {
	Name string `toml:"name"`
	Drop int    `toml:"drop"`
}

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
	Duration    DurationSeg   `toml:"duration"`
	RateLimits  RateLimitsSeg `toml:"ratelimits"`
	RateLimit5h ResetSeg      `toml:"ratelimit_5h"`
	RateLimit7d ResetSeg      `toml:"ratelimit_7d"`
	Context     FormatSeg     `toml:"context"`
	Diffstat    FormatSeg     `toml:"diffstat"`
	Cost        FormatSeg     `toml:"cost"`
	Model       FormatSeg     `toml:"model"`
	Effort      FormatSeg     `toml:"effort"`
	Branch      FormatSeg     `toml:"branch"`
	Project     FormatSeg     `toml:"project"`
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

type ResetSeg struct {
	Format      string `toml:"format"`
	ResetFormat string `toml:"reset_format"`
}
