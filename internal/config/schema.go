package config

var SegmentDefs = []SegmentDef{
	{
		Name: "model",
		Doc:  "which model answered, from the payload's model.display_name.",
		Keys: []Key{{
			Name:   "format",
			Syntax: SyntaxPlaceholders,
			Fields: []Field{
				{Name: "name", Kind: KindText, Color: "model_name"},
				{Name: "marker", Kind: KindGlyph, Color: "model_marker"},
			},
			Default: "{marker} {name}",
			Get:     func(c *Config) string { return c.Segments.Model.Format },
			Set:     func(c *Config, v string) { c.Segments.Model.Format = v },
		}},
		Presentations: []Presentation{
			{"{marker} {name}"},
			{"Model: {name}"},
		},
	},

	{
		Name: "effort",
		Doc: "the reasoning effort this session is set to, from the payload's " +
			"effort.level. Absent on a model that has no effort setting.",
		Keys: []Key{{
			Name:    "format",
			Syntax:  SyntaxPlaceholders,
			Fields:  []Field{{Name: "level", Kind: KindText, Color: "effort"}},
			Default: "Effort: {level}",
			Get:     func(c *Config) string { return c.Segments.Effort.Format },
			Set:     func(c *Config, v string) { c.Segments.Effort.Format = v },
		}},
		Presentations: []Presentation{
			{"Effort: {level}"},
			{"{level}"},
		},
	},

	{
		Name:  "context",
		Doc:   "how full the context window is — the bar, the percentage, ⚠ past danger.",
		Tunes: []string{"[bar]", "[thresholds] warning, danger", "[context] show_size"},
		Keys: []Key{{
			Name:   "format",
			Syntax: SyntaxPlaceholders,
			Fields: []Field{
				{Name: "bar", Kind: KindGauge, Color: "bar_empty", Band: BandContext},
				{Name: "n", Kind: KindPercent, Band: BandContext},
				{Name: "warn", Kind: KindGlyph, Color: "danger"},
				{Name: "size", Kind: KindText, Band: BandContext},
			},
			Default: "{bar} {n}%{warn}{size}",
			Get:     func(c *Config) string { return c.Segments.Context.Format },
			Set:     func(c *Config, v string) { c.Segments.Context.Format = v },
		}},
		Presentations: []Presentation{
			{"{bar} {n}%{warn}{size}"},
			{"Ctx: {bar} {n}%{warn}{size}"},
		},
	},

	{
		Name: "cost",
		Doc:  "what this session has cost so far, from cost.total_cost_usd.",
		Keys: []Key{{
			Name:    "format",
			Syntax:  SyntaxPlaceholders,
			Fields:  []Field{{Name: "n", Kind: KindMoney, Color: "cost"}},
			Default: "${n}",
			Get:     func(c *Config) string { return c.Segments.Cost.Format },
			Set:     func(c *Config, v string) { c.Segments.Cost.Format = v },
		}},
		Presentations: []Presentation{
			{"${n}"},
			{"Cost: ${n}"},
		},
	},

	{
		Name: "duration",
		Doc:  "how long this session has run. Empty below a minute.",
		Keys: []Key{
			{
				Name:    "under_hour",
				Syntax:  SyntaxPlaceholders,
				Fields:  durationFields,
				Default: "{m}m",
				Get:     func(c *Config) string { return c.Segments.Duration.UnderHour },
				Set:     func(c *Config, v string) { c.Segments.Duration.UnderHour = v },
			},
			{
				Name:    "over_hour",
				Syntax:  SyntaxPlaceholders,
				Fields:  durationFields,
				Default: "{h}h{m}m",
				Get:     func(c *Config) string { return c.Segments.Duration.OverHour },
				Set:     func(c *Config, v string) { c.Segments.Duration.OverHour = v },
			},
			{
				Name:    "over_day",
				Syntax:  SyntaxPlaceholders,
				Fields:  durationFields,
				Default: "{d}d{h}h",
				Get:     func(c *Config) string { return c.Segments.Duration.OverDay },
				Set:     func(c *Config, v string) { c.Segments.Duration.OverDay = v },
			},
			{Name: "pad", Syntax: SyntaxOpaque},
		},
		Presentations: []Presentation{
			{"{m}m", "{h}h{m}m", "{d}d{h}h"},
			{"Time: {m}m", "Time: {h}h{m}m", "Time: {d}d{h}h"},
		},
	},

	{
		Name:  "ratelimits",
		Doc:   "how much of the 5-hour and 7-day windows is spent. Either shows alone.",
		Tunes: []string{"[thresholds] ratelimit_warn"},
		Keys: []Key{
			{
				Name:    "five_format",
				Syntax:  SyntaxPlaceholders,
				Fields:  []Field{{Name: "n", Kind: KindPercent, Band: BandRateLimit}},
				Default: "5h:{n}%",
				Get:     func(c *Config) string { return c.Segments.RateLimits.FiveFormat },
				Set:     func(c *Config, v string) { c.Segments.RateLimits.FiveFormat = v },
			},
			{
				Name:    "seven_format",
				Syntax:  SyntaxPlaceholders,
				Fields:  []Field{{Name: "n", Kind: KindPercent, Band: BandRateLimit}},
				Default: "7d:{n}%",
				Get:     func(c *Config) string { return c.Segments.RateLimits.SevenFormat },
				Set:     func(c *Config, v string) { c.Segments.RateLimits.SevenFormat = v },
			},
			{
				Name:    "join",
				Syntax:  SyntaxOpaque,
				Default: " ",
				Get:     func(c *Config) string { return c.Segments.RateLimits.Join },
				Set:     func(c *Config, v string) { c.Segments.RateLimits.Join = v },
			},
		},
		Presentations: []Presentation{
			{"5h:{n}%", "7d:{n}%"},
			{"Session: {n}%", "Weekly: {n}%"},
		},
	},

	{
		Name: "ratelimit_5h",
		Doc: "the 5-hour window alone, so it can take its own row and its own drop. " +
			"tab adds the clock time it comes back.",
		Tunes: []string{"[thresholds] ratelimit_warn"},
		Keys: []Key{
			{
				Name:    "format",
				Syntax:  SyntaxPlaceholders,
				Fields:  resetWindowFields,
				Default: "5h:{n}%",
				Get:     func(c *Config) string { return c.Segments.RateLimit5h.Format },
				Set:     func(c *Config, v string) { c.Segments.RateLimit5h.Format = v },
			},
			{
				Name:    "reset_format",
				Syntax:  SyntaxTimeLayout,
				Default: "15:04",
				Get:     func(c *Config) string { return c.Segments.RateLimit5h.ResetFormat },
				Set:     func(c *Config, v string) { c.Segments.RateLimit5h.ResetFormat = v },
			},
		},
		Presentations: fiveHourPresentations,
	},

	{
		Name: "ratelimit_7d",
		Doc: "the 7-day window alone, so it can take its own row and its own drop. " +
			"tab adds the date and time it comes back.",
		Tunes: []string{"[thresholds] ratelimit_warn"},
		Keys: []Key{
			{
				Name:    "format",
				Syntax:  SyntaxPlaceholders,
				Fields:  resetWindowFields,
				Default: "7d:{n}%",
				Get:     func(c *Config) string { return c.Segments.RateLimit7d.Format },
				Set:     func(c *Config, v string) { c.Segments.RateLimit7d.Format = v },
			},
			{
				Name:    "reset_format",
				Syntax:  SyntaxTimeLayout,
				Default: "2 Jan 15:04",
				Get:     func(c *Config) string { return c.Segments.RateLimit7d.ResetFormat },
				Set:     func(c *Config, v string) { c.Segments.RateLimit7d.ResetFormat = v },
			},
		},
		Presentations: sevenDayPresentations,
	},

	{
		Name:  "branch",
		Doc:   "the checked-out branch, found by walking up from the payload's cwd.",
		Tunes: []string{"[git] enabled, branch_max_len"},
		Keys: []Key{{
			Name:    "format",
			Syntax:  SyntaxPlaceholders,
			Fields:  []Field{{Name: "name", Kind: KindText, Color: "branch"}},
			Default: "{name}",
			Get:     func(c *Config) string { return c.Segments.Branch.Format },
			Set:     func(c *Config, v string) { c.Segments.Branch.Format = v },
		}},
		Presentations: []Presentation{
			{"{name}"},
		},
	},

	{
		Name: "diffstat",
		Doc:  "lines added and removed this session — the payload's count, not git's.",
		Keys: []Key{{
			Name:   "format",
			Syntax: SyntaxPlaceholders,
			Fields: []Field{
				{Name: "added", Kind: KindCount, Color: "added"},
				{Name: "removed", Kind: KindCount, Color: "removed"},
			},
			Default: "+{added}/-{removed}",
			Get:     func(c *Config) string { return c.Segments.Diffstat.Format },
			Set:     func(c *Config, v string) { c.Segments.Diffstat.Format = v },
		}},
		Presentations: []Presentation{
			{"+{added}/-{removed}"},
			{"Diff: +{added}/-{removed}"},
		},
	},

	{
		Name: "project",
		Doc:  "the basename of workspace.project_dir.",
		Keys: []Key{{
			Name:    "format",
			Syntax:  SyntaxPlaceholders,
			Fields:  []Field{{Name: "name", Kind: KindText, Color: "project"}},
			Default: "{name}",
			Get:     func(c *Config) string { return c.Segments.Project.Format },
			Set:     func(c *Config, v string) { c.Segments.Project.Format = v },
		}},
		Presentations: []Presentation{
			{"{name}"},
			{"Project: {name}"},
		},
	},
}

var durationFields = []Field{
	{Name: "d", Kind: KindCount, Color: "duration"},
	{Name: "h", Kind: KindCount, Color: "duration"},
	{Name: "m", Kind: KindCount, Color: "duration"},
}

var resetWindowFields = []Field{
	{Name: "n", Kind: KindPercent, Band: BandRateLimit},
	{Name: "icon", Kind: KindGlyph, Color: "ratelimit"},
	{Name: "reset", Kind: KindClock, Color: "ratelimit"},
}

var (
	fiveHourPresentations = []Presentation{
		{"5h:{n}%", "15:04"},
		{"5h:{n}%{icon}{reset}", "15:04"},
		{"Session: {n}%", "resets 15:04"},
		{"Session: {n}%{reset}", "resets 15:04"},
	}

	sevenDayPresentations = []Presentation{
		{"7d:{n}%", "2 Jan 15:04"},
		{"7d:{n}%{icon}{reset}", "2 Jan 15:04"},
		{"Weekly: {n}%", "resets 2 Jan 15:04"},
		{"Weekly: {n}%{reset}", "resets 2 Jan 15:04"},
	}
)

const (
	BandContext = "context"

	BandRateLimit = "ratelimit"
)

var ColorDefs = []ColorDef{
	{"model_marker", "#cba6f7", func(c *Config) string { return c.Colors.ModelMarker }, func(c *Config, v string) { c.Colors.ModelMarker = v }},
	{"model_name", "#89dceb", func(c *Config) string { return c.Colors.ModelName }, func(c *Config, v string) { c.Colors.ModelName = v }},
	{"normal", "#4ade80", func(c *Config) string { return c.Colors.Normal }, func(c *Config, v string) { c.Colors.Normal = v }},
	{"warning", "#facc15", func(c *Config) string { return c.Colors.Warning }, func(c *Config, v string) { c.Colors.Warning = v }},
	{"danger", "#ef4444", func(c *Config) string { return c.Colors.Danger }, func(c *Config, v string) { c.Colors.Danger = v }},
	{"cost", "#4ade80", func(c *Config) string { return c.Colors.Cost }, func(c *Config, v string) { c.Colors.Cost = v }},
	{"duration", "#6c7086", func(c *Config) string { return c.Colors.Duration }, func(c *Config, v string) { c.Colors.Duration = v }},
	{"ratelimit", "#6c7086", func(c *Config) string { return c.Colors.RateLimit }, func(c *Config, v string) { c.Colors.RateLimit = v }},
	{"effort", "#94e2d5", func(c *Config) string { return c.Colors.Effort }, func(c *Config, v string) { c.Colors.Effort = v }},
	{"branch", "#cba6f7", func(c *Config) string { return c.Colors.Branch }, func(c *Config, v string) { c.Colors.Branch = v }},
	{"added", "#4ade80", func(c *Config) string { return c.Colors.Added }, func(c *Config, v string) { c.Colors.Added = v }},
	{"removed", "#ef4444", func(c *Config) string { return c.Colors.Removed }, func(c *Config, v string) { c.Colors.Removed = v }},
	{"project", "#89b4fa", func(c *Config) string { return c.Colors.Project }, func(c *Config, v string) { c.Colors.Project = v }},
	{"separator", "#45475a", func(c *Config) string { return c.Colors.Separator }, func(c *Config, v string) { c.Colors.Separator = v }},
	{"diffstat_delim", "#45475a", func(c *Config) string { return c.Colors.DiffstatDelim }, func(c *Config, v string) { c.Colors.DiffstatDelim = v }},
	{"bar_empty", "#45475a", func(c *Config) string { return c.Colors.BarEmpty }, func(c *Config, v string) { c.Colors.BarEmpty = v }},
}
