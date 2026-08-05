package config

// The two accessor tables every other file in the project reads.
//
// Both exist to kill a duplication that would fail silently. A colour key named
// in [colors] but missing from internal/style renders unpainted with no error;
// a format placeholder accepted by the validator but unknown to its segment
// renders as literal `{foo}` on the status line. Enumerating each once, with
// accessors attached, makes both failures compile-time impossible.

// ColorKey is one entry in PRD §7.2's [colors] table.
type ColorKey struct {
	Name string
	Get  func(*Config) string
	Set  func(*Config, string)
}

// ColorKeys is [colors], in schema order. Excludes gradient_stops, which is a
// list and is validated separately.
var ColorKeys = []ColorKey{
	{"model_marker", func(c *Config) string { return c.Colors.ModelMarker }, func(c *Config, v string) { c.Colors.ModelMarker = v }},
	{"model_name", func(c *Config) string { return c.Colors.ModelName }, func(c *Config, v string) { c.Colors.ModelName = v }},
	{"normal", func(c *Config) string { return c.Colors.Normal }, func(c *Config, v string) { c.Colors.Normal = v }},
	{"warning", func(c *Config) string { return c.Colors.Warning }, func(c *Config, v string) { c.Colors.Warning = v }},
	{"danger", func(c *Config) string { return c.Colors.Danger }, func(c *Config, v string) { c.Colors.Danger = v }},
	{"cost", func(c *Config) string { return c.Colors.Cost }, func(c *Config, v string) { c.Colors.Cost = v }},
	{"duration", func(c *Config) string { return c.Colors.Duration }, func(c *Config, v string) { c.Colors.Duration = v }},
	{"ratelimit", func(c *Config) string { return c.Colors.RateLimit }, func(c *Config, v string) { c.Colors.RateLimit = v }},
	{"branch", func(c *Config) string { return c.Colors.Branch }, func(c *Config, v string) { c.Colors.Branch = v }},
	{"added", func(c *Config) string { return c.Colors.Added }, func(c *Config, v string) { c.Colors.Added = v }},
	{"removed", func(c *Config) string { return c.Colors.Removed }, func(c *Config, v string) { c.Colors.Removed = v }},
	{"project", func(c *Config) string { return c.Colors.Project }, func(c *Config, v string) { c.Colors.Project = v }},
	{"separator", func(c *Config) string { return c.Colors.Separator }, func(c *Config, v string) { c.Colors.Separator = v }},
	{"diffstat_delim", func(c *Config) string { return c.Colors.DiffstatDelim }, func(c *Config, v string) { c.Colors.DiffstatDelim = v }},
	{"bar_empty", func(c *Config) string { return c.Colors.BarEmpty }, func(c *Config, v string) { c.Colors.BarEmpty = v }},
}

var colorByName = func() map[string]ColorKey {
	m := make(map[string]ColorKey, len(ColorKeys))
	for _, k := range ColorKeys {
		m[k.Name] = k
	}
	return m
}()

// Color resolves a [colors] key to its configured hex value. An unknown key
// yields ok=false, and internal/style renders such text unpainted: a colour is
// decoration, and a render path that can fail on decoration is a render path
// that can blank the line.
func (c *Config) Color(name string) (string, bool) {
	k, ok := colorByName[name]
	if !ok {
		return "", false
	}
	return k.Get(c), true
}

// FormatKey is one row of PRD §5.7's placeholder table.
//
// §5.7 requires this table be defined once and consumed by both the validator
// and every segment's renderer. Segments consume it indirectly: Validate
// rewrites any format naming a placeholder its segment does not supply back to
// the embedded default, so by the time a segment sees its format string, the
// format is known to be renderable. TestEveryPlaceholderIsReachable closes the
// other direction — every placeholder listed here actually produces output.
type FormatKey struct {
	Key          string // the dotted TOML path, for diagnostics
	Segment      string // the segment that consumes it
	Placeholders []string
	Get          func(*Config) string
	Set          func(*Config, string)
}

// FormatKeys is PRD §5.7's table, transcribed.
var FormatKeys = []FormatKey{
	{
		Key: "segments.duration.under_hour", Segment: "duration",
		Placeholders: []string{"d", "h", "m"},
		Get:          func(c *Config) string { return c.Segments.Duration.UnderHour },
		Set:          func(c *Config, v string) { c.Segments.Duration.UnderHour = v },
	},
	{
		Key: "segments.duration.over_hour", Segment: "duration",
		Placeholders: []string{"d", "h", "m"},
		Get:          func(c *Config) string { return c.Segments.Duration.OverHour },
		Set:          func(c *Config, v string) { c.Segments.Duration.OverHour = v },
	},
	{
		Key: "segments.duration.over_day", Segment: "duration",
		Placeholders: []string{"d", "h", "m"},
		Get:          func(c *Config) string { return c.Segments.Duration.OverDay },
		Set:          func(c *Config, v string) { c.Segments.Duration.OverDay = v },
	},
	{
		Key: "segments.ratelimits.five_format", Segment: "ratelimits",
		Placeholders: []string{"n"},
		Get:          func(c *Config) string { return c.Segments.RateLimits.FiveFormat },
		Set:          func(c *Config, v string) { c.Segments.RateLimits.FiveFormat = v },
	},
	{
		Key: "segments.ratelimits.seven_format", Segment: "ratelimits",
		Placeholders: []string{"n"},
		Get:          func(c *Config) string { return c.Segments.RateLimits.SevenFormat },
		Set:          func(c *Config, v string) { c.Segments.RateLimits.SevenFormat = v },
	},
	{
		Key: "segments.diffstat.format", Segment: "diffstat",
		Placeholders: []string{"added", "removed"},
		Get:          func(c *Config) string { return c.Segments.Diffstat.Format },
		Set:          func(c *Config, v string) { c.Segments.Diffstat.Format = v },
	},
	{
		Key: "segments.cost.format", Segment: "cost",
		Placeholders: []string{"n"},
		Get:          func(c *Config) string { return c.Segments.Cost.Format },
		Set:          func(c *Config, v string) { c.Segments.Cost.Format = v },
	},
	{
		Key: "segments.context.format", Segment: "context",
		Placeholders: []string{"bar", "n", "warn", "size"},
		Get:          func(c *Config) string { return c.Segments.Context.Format },
		Set:          func(c *Config, v string) { c.Segments.Context.Format = v },
	},
	{
		Key: "segments.model.format", Segment: "model",
		Placeholders: []string{"name", "marker"},
		Get:          func(c *Config) string { return c.Segments.Model.Format },
		Set:          func(c *Config, v string) { c.Segments.Model.Format = v },
	},
	{
		Key: "segments.branch.format", Segment: "branch",
		Placeholders: []string{"name"},
		Get:          func(c *Config) string { return c.Segments.Branch.Format },
		Set:          func(c *Config, v string) { c.Segments.Branch.Format = v },
	},
	{
		Key: "segments.project.format", Segment: "project",
		Placeholders: []string{"name"},
		Get:          func(c *Config) string { return c.Segments.Project.Format },
		Set:          func(c *Config, v string) { c.Segments.Project.Format = v },
	},
}
