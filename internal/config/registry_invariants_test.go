package config

import "testing"

func TestEverySegmentDefIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range SegmentDefs {
		if d.Name == "" {
			t.Error("a segment has no name")
			continue
		}
		if seen[d.Name] {
			t.Errorf("%s: declared twice", d.Name)
		}
		seen[d.Name] = true

		if d.Name == FlexName {
			t.Errorf("%s is the layout marker, not a segment", FlexName)
		}
		if d.Doc == "" {
			t.Errorf("%s: no Doc, so the wizard's hint is empty for it", d.Name)
		}
		if len(d.Keys) == 0 {
			t.Errorf("%s: no keys, so nothing about it is configurable", d.Name)
		}

		for _, k := range d.Keys {
			switch {
			case k.Name == "":
				t.Errorf("%s: a key has no name", d.Name)
			case k.Syntax == SyntaxOpaque:
			case k.Get == nil || k.Set == nil:
				t.Errorf("%s: %s has a grammar but no accessor, so nothing "+
					"validates it and no variant can write it", d.Name, k.Name)
			}
			if k.Syntax == SyntaxPlaceholders && len(k.Fields) == 0 {
				t.Errorf("%s.%s: a placeholder format with no fields accepts "+
					"no placeholders at all", d.Name, k.Name)
			}
		}
	}
}

func TestEveryPresentationAssignsEveryKey(t *testing.T) {
	for _, d := range SegmentDefs {
		want := len(d.PresentationKeys())
		if want == 0 {
			t.Errorf("%s: no keys a presentation could assign", d.Name)
			continue
		}
		if len(d.Presentations) == 0 {
			t.Errorf("%s: no presentations, so tab has nothing to cycle", d.Name)
		}
		for i, p := range d.Presentations {
			if len(p) != want {
				t.Errorf("%s presentation %d assigns %d of %d keys: %q",
					d.Name, i, len(p), want, p)
			}
		}
	}
}

func TestEveryFieldNamesAColorThatExists(t *testing.T) {
	known := map[string]bool{}
	for _, c := range ColorDefs {
		known[c.Name] = true
	}
	for _, d := range SegmentDefs {
		for _, k := range d.Keys {
			for _, f := range k.Fields {
				if f.Color != "" && !known[f.Color] {
					t.Errorf("%s.%s {%s}: colour %q is not a ColorDef",
						d.Name, k.Name, f.Name, f.Color)
				}
				if f.Band != "" && BandColors(f.Band) == nil {
					t.Errorf("%s.%s {%s}: band %q resolves to no colours",
						d.Name, k.Name, f.Name, f.Band)
				}
				if f.Color == "" && f.Band == "" {
					t.Errorf("%s.%s {%s}: neither a colour nor a band, so it "+
						"renders in whatever the format's default is",
						d.Name, k.Name, f.Name)
				}
			}
		}
	}
	for _, band := range []string{BandContext, BandRateLimit} {
		for _, name := range BandColors(band) {
			if !known[name] {
				t.Errorf("band %q resolves to %q, which is not a ColorDef", band, name)
			}
		}
	}
}

func TestBandColorOnlyReturnsItsOwnColors(t *testing.T) {
	cfg := Defaults()
	for _, band := range []string{BandContext, BandRateLimit} {
		allowed := map[string]bool{}
		for _, name := range BandColors(band) {
			allowed[name] = true
		}
		for n := -10; n <= 110; n++ {
			if got := BandColor(band, n, cfg); !allowed[got] {
				t.Fatalf("BandColor(%q, %d) = %q, which BandColors does not name",
					band, n, got)
			}
		}
	}
	if got := BandColor("nonsense", 50, cfg); got != "" {
		t.Errorf(`BandColor("nonsense", …) = %q, want ""`, got)
	}
}

func TestBandColorReadsTheConfiguredThresholds(t *testing.T) {
	cfg := Defaults()
	cfg.Thresholds.Warning, cfg.Thresholds.Danger = 10, 20
	cfg.Thresholds.RateLimitWarn = 30

	for _, tc := range []struct {
		band string
		n    int
		want string
	}{
		{BandContext, 9, "normal"},
		{BandContext, 10, "warning"},
		{BandContext, 19, "warning"},
		{BandContext, 20, "danger"},
		{BandRateLimit, 29, "ratelimit"},
		{BandRateLimit, 30, "warning"},
	} {
		if got := BandColor(tc.band, tc.n, cfg); got != tc.want {
			t.Errorf("BandColor(%q, %d) = %q, want %q", tc.band, tc.n, got, tc.want)
		}
	}
}
