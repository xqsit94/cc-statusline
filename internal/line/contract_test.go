package line

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
)

func TestRegistryMatchesConfig(t *testing.T) {
	for _, name := range config.SegmentNames {
		seg, ok := New(name)
		if !ok {
			t.Errorf("config accepts %q but New does not build it", name)
			continue
		}
		if seg.Name() != name {
			t.Errorf("New(%q).Name() = %q", name, seg.Name())
		}
	}
	known := map[string]bool{}
	for _, name := range config.SegmentNames {
		known[name] = true
	}
	for name := range builders {
		if !known[name] {
			t.Errorf("%q is registered but is not in config.SegmentNames, so no "+
				"config can name it and nothing validates it", name)
		}
	}
}

func TestNoSegmentIsRegisteredTwice(t *testing.T) {
	if len(duplicates) > 0 {
		t.Errorf("registered more than once: %v", duplicates)
	}
}

func TestEveryPlaceholderIsReachable(t *testing.T) {
	for _, k := range config.FormatKeys {
		for _, ph := range k.Placeholders {
			t.Run(k.Key+"/"+ph, func(t *testing.T) {
				ctx := ctxFor(t, payloadFor(k.Key), map[string]string{"COLUMNS": "500"}, "feat/auth")
				k.Set(ctx.Config, "{"+ph+"}")

				seg, ok := New(k.Segment)
				if !ok {
					t.Fatalf("no segment named %q", k.Segment)
				}
				got := seg.Render(ctx)

				if got.Empty() {
					t.Errorf("{%s} rendered nothing from the %s segment", ph, k.Segment)
				}
				if strings.Contains(got.Plain, "{"+ph+"}") {
					t.Errorf("{%s} came back verbatim from the %s segment: %q",
						ph, k.Segment, got.Plain)
				}
			})
		}
	}
}

func shapeOf(k config.Kind) (*regexp.Regexp, string, bool) {
	switch k {
	case config.KindMoney:
		return regexp.MustCompile(`^\d+\.\d{2}$`),
			"KindMoney is two decimal places — an observed session reported " +
				"107.43094200000006, and a default float format puts that on screen", true
	case config.KindPercent:
		return regexp.MustCompile(`^\d+$`),
			"KindPercent reaches the screen as a bare integer, rounded once at " +
				"the point of display, so §5.4's threshold and the reader see " +
				"the same number", true
	case config.KindCount:
		return regexp.MustCompile(`^\d+$`), "KindCount is a whole number", true
	}
	return nil, "", false
}

func TestEveryFieldRendersItsDeclaredKind(t *testing.T) {
	for _, d := range config.SegmentDefs {
		for _, k := range d.FormatKeyDefs() {
			for _, f := range k.Fields {
				want, why, checked := shapeOf(f.Kind)
				if !checked {
					continue
				}
				path := k.Path(d.Name)
				t.Run(path+"/"+f.Name, func(t *testing.T) {
					ctx := ctxFor(t, isolatingPayloadFor(path), map[string]string{"COLUMNS": "500"}, "feat/auth")
					k.Set(ctx.Config, "{"+f.Name+"}")

					seg, ok := New(d.Name)
					if !ok {
						t.Fatalf("no segment named %q", d.Name)
					}
					got := seg.Render(ctx).Plain

					if !want.MatchString(got) {
						t.Errorf("{%s} rendered %q, which is not %v\n%s",
							f.Name, got, want, why)
					}
				})
			}
		}
	}
}

func TestUnknownPlaceholderNeverReachesRender(t *testing.T) {
	for _, k := range config.FormatKeys {
		cfg := config.Defaults()
		k.Set(cfg, "{definitely_not_a_placeholder}")
		notes := config.Validate(cfg)

		if got := k.Get(cfg); got != k.Get(config.Defaults()) {
			t.Errorf("%s = %q after validation, want the default", k.Key, got)
		}
		found := false
		for _, n := range notes {
			if n.Key == k.Key {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was repaired without a note", k.Key)
		}
	}
}

func payloadFor(key string) string {
	switch key {
	case "segments.duration.under_hour":
		return durationPayload(2700000)
	case "segments.duration.over_hour":
		return durationPayload(3900000)
	case "segments.duration.over_day":
		return durationPayload(183600000)
	}
	return durationPayload(183600000)
}

func isolatingPayloadFor(key string) string {
	switch key {
	case "segments.ratelimits.five_format":
		return oneWindowPayload("five_hour", 85)
	case "segments.ratelimits.seven_format":
		return oneWindowPayload("seven_day", 62)
	}
	return payloadFor(key)
}

func oneWindowPayload(window string, pct int) string {
	return `{"model":{"display_name":"Claude Opus 4.6"},
		"rate_limits":{"` + window + `":{"used_percentage":` + itoa(pct) + `}}}`
}

func durationPayload(ms int) string {
	return `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"current_dir":"/w/api-server","project_dir":"/w/api-server"},
		"cost":{"total_cost_usd":15.30,"total_duration_ms":` + itoa(ms) + `,
		        "total_lines_added":500,"total_lines_removed":120},
		"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
		"effort":{"level":"high"},
		"rate_limits":{"five_hour":{"used_percentage":85,"resets_at":1785951600},
		               "seven_day":{"used_percentage":62,"resets_at":1786383600}}}`
}

func TestEveryColorKeyResolves(t *testing.T) {
	ctx := ctxFor(t, `{"model":{"display_name":"M"}}`, nil, "")
	for _, k := range config.ColorKeys {
		if got := ctx.Style.Hex(k.Name); got == "" {
			t.Errorf("colors.%s does not resolve through Style.Hex", k.Name)
		}
	}
	if got := ctx.Style.Hex("no_such_key"); got != "" {
		t.Errorf("an unknown key resolved to %q", got)
	}
}

func TestEveryShippedVariantRenders(t *testing.T) {
	for _, name := range config.SegmentNames {
		for i, v := range config.Variants[name] {
			ctx := ctxFor(t, durationPayload(183600000), nil, "")
			ctx.Zone = time.UTC
			ctx.Git = gitinfo.Info{Found: true, Branch: "main", GitDir: "/synthetic/.git"}
			config.ApplyVariant(v, &ctx.Config.Segments)

			seg, ok := New(name)
			if !ok {
				t.Fatalf("no segment named %q", name)
			}
			got := seg.Render(ctx)
			if got.Empty() {
				t.Errorf("%s format %d rendered nothing against a payload with every field",
					name, i+1)
				continue
			}
			if strings.ContainsAny(got.Plain, "{}") {
				t.Errorf("%s format %d rendered %q, which still holds a placeholder",
					name, i+1, got.Plain)
			}
		}
	}
}
