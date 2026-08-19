package config

func Defaults() *Config {
	c := &Config{
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
			Duration: DurationSeg{Pad: false},
		},
	}
	applyRegistryDefaults(c)
	return c
}

const DefaultDrop = 50

const NeverDrop = 99
