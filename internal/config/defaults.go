package config

// Defaults returns the embedded default configuration — the bottom layer of
// PRD §7.1's overlay, and the only layer that exists at M2.
//
// Every value here is transcribed from the schema in PRD §7.2. The reference
// states in §5.1 are byte-identical acceptance criteria for exactly this
// configuration, so a change to any default is a change to the acceptance
// criteria and should fail a golden test rather than pass quietly.
//
// It returns a fresh value each call. Handing out a pointer to a package-level
// struct would let any caller — including the M7 wizard's live preview —
// mutate the defaults for everyone else in the process.
func Defaults() *Config {
	return &Config{
		General: General{
			Separator:       "auto",
			Powerline:       Flexible("auto"),
			Icons:           "unicode",
			Color:           "auto",
			MaxWidth:        0,
			WidthReserve:    12,
			Padding:         0,
			RefreshInterval: 60,
			AmbiguousWidth:  Flexible("auto"),
		},
		Thresholds: Threshold{
			Warning:       70,
			Danger:        85,
			RateLimitWarn: 80,
		},
		Bar: Bar{
			Enabled:  true,
			Width:    10,
			Filled:   "auto",
			Empty:    "auto",
			Gradient: true,
		},
		Git: Git{
			Enabled:      true,
			BranchMaxLen: 24,
		},
		Context: Context{
			ShowSize: "non_default",
		},
		Colors: Colors{
			ModelMarker:   "#cba6f7",
			ModelName:     "#89dceb",
			Normal:        "#4ade80",
			Warning:       "#facc15",
			Danger:        "#ef4444",
			Cost:          "#4ade80",
			Duration:      "#6c7086",
			RateLimit:     "#6c7086",
			Branch:        "#cba6f7",
			Added:         "#4ade80",
			Removed:       "#ef4444",
			Project:       "#89b4fa",
			Separator:     "#45475a",
			DiffstatDelim: "#45475a",
			BarEmpty:      "#45475a",
			GradientStops: []string{"#4ade80", "#facc15", "#fb923c", "#ef4444"},
		},
		Lines: []Line{
			{Segments: []SegmentRef{
				{Name: "model", Drop: NeverDrop},
				{Name: "context", Drop: NeverDrop},
				{Name: "cost", Drop: 4},
				{Name: "duration", Drop: 5},
				{Name: "ratelimits", Drop: 3},
			}},
			{Segments: []SegmentRef{
				{Name: "branch", Drop: NeverDrop},
				{Name: "diffstat", Drop: 2},
				{Name: "project", Drop: 1},
			}},
		},
		Segments: Segments{
			Duration: DurationSeg{
				UnderHour: "{m}m",
				OverHour:  "{h}h{m}m",
				OverDay:   "{d}d{h}h",
				Pad:       false,
			},
			RateLimits: RateLimitsSeg{
				FiveFormat:  "5h:{n}%",
				SevenFormat: "7d:{n}%",
				Join:        " ",
			},
			Context:  FormatSeg{Format: "{bar} {n}%{warn}{size}"},
			Diffstat: FormatSeg{Format: "+{added}/-{removed}"},
			Cost:     FormatSeg{Format: "${n}"},
			Model:    FormatSeg{Format: "{marker} {name}"},
			Branch:   FormatSeg{Format: "{name}"},
			Project:  FormatSeg{Format: "{name}"},
		},
	}
}

// DefaultDrop is the drop priority for a segment that omits one (PRD §7.2).
const DefaultDrop = 50

// NeverDrop is the priority at which a segment is exempt from stage 1 of the
// fitter (PRD §5.6). It is also the schema's upper bound: a larger number can
// mean nothing more than "never", so validation clamps to it.
const NeverDrop = 99
