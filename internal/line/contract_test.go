package line

import (
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/config"
)

// The two tables this project refuses to write down twice, checked from the
// side that would otherwise fail silently.

// TestRegistryMatchesConfig: config.SegmentNames is the vocabulary the
// validator accepts, New is the registry that renders. A name in one and not
// the other is either a segment nobody can configure or a config key that
// renders nothing — and neither produces an error anywhere.
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
	// And the reverse. Nothing enumerates the switch in New, so this walks the
	// names the default config uses plus every one config knows about, which is
	// the closest a test can get without reflection over a switch statement.
	for _, l := range config.Defaults().Lines {
		for _, ref := range l.Segments {
			if _, ok := New(ref.Name); !ok {
				t.Errorf("the default config references %q, which New cannot build", ref.Name)
			}
		}
	}
}

// TestEveryPlaceholderIsReachable is PRD §5.7's requirement, executed: "A test
// asserts every key in the table is reachable from its segment."
//
// The failure it prevents is specific. config.FormatKeys is what the validator
// accepts; the vars map inside each segment is what actually renders. If the
// table lists a placeholder the segment does not supply, the config validates
// cleanly and the status line displays the literal text `{whatever}` — a
// failure with no error attached to it anywhere.
func TestEveryPlaceholderIsReachable(t *testing.T) {
	for _, k := range config.FormatKeys {
		for _, ph := range k.Placeholders {
			t.Run(k.Key+"/"+ph, func(t *testing.T) {
				ctx := ctxFor(t, payloadFor(k.Key), map[string]string{"COLUMNS": "500"}, "feat/auth")
				// The format becomes the single placeholder under test, so
				// anything in the output came from it and nothing else.
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

// TestUnknownPlaceholderNeverReachesRender is the same table from the other
// side: a format naming something its segment cannot supply must be replaced
// with the default before any segment sees it.
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

// payloadFor returns a payload that makes the placeholder under test render.
//
// The duration keys need three different payloads because they are selected by
// magnitude; everything else renders from a payload in the danger band, which
// is the only state where {warn} is non-empty.
func payloadFor(key string) string {
	switch key {
	case "segments.duration.under_hour":
		return durationPayload(2700000) // 45m
	case "segments.duration.over_hour":
		return durationPayload(3900000) // 1h5m
	case "segments.duration.over_day":
		return durationPayload(183600000) // 2d3h
	}
	return durationPayload(183600000)
}

// durationPayload populates every field any segment reads: the danger band for
// {warn}, a 1M window for {size}, a non-zero diffstat, and both rate limits.
func durationPayload(ms int) string {
	return `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"current_dir":"/w/api-server","project_dir":"/w/api-server"},
		"cost":{"total_cost_usd":15.30,"total_duration_ms":` + itoa(ms) + `,
		        "total_lines_added":500,"total_lines_removed":120},
		"context_window":{"context_window_size":1000000,"total_input_tokens":920000},
		"rate_limits":{"five_hour":{"used_percentage":85},
		               "seven_day":{"used_percentage":62}}}`
}

// TestEveryColorKeyResolves is the same shape for [colors]. Style.Paint returns
// text unstyled for an unknown key rather than failing, which is correct — and
// which is exactly why a missing key has to be caught here instead.
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
