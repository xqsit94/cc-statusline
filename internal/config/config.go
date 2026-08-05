// Package config holds cc-statusline's configuration schema (PRD §7.2).
//
// M2 ships the schema and the embedded defaults only. TOML loading, XDG
// resolution, the CC_STATUSLINE_* overlay, and validation arrive at M3; the
// struct is here now because PRD §6.4's capability resolution takes a
// *config.Config, and writing that signature honestly today is what stops M7's
// wizard from becoming a refactor of M2.
package config

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

// General's sentinel fields are strings because several accept more than one
// TOML type — `powerline` is "auto" | true | false and `ambiguous_width` is
// "auto" | 1 | 2. M3 adds an UnmarshalTOML that normalises the bool and integer
// forms into these strings; until then nothing parses TOML at all, so the
// looser type costs nothing and keeps one representation downstream.
type General struct {
	Separator       string `toml:"separator"`
	Powerline       string `toml:"powerline"`
	Icons           string `toml:"icons"`
	Color           string `toml:"color"`
	MaxWidth        int    `toml:"max_width"`
	WidthReserve    int    `toml:"width_reserve"`
	Padding         int    `toml:"padding"`
	RefreshInterval int    `toml:"refresh_interval"`
	AmbiguousWidth  string `toml:"ambiguous_width"`
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
