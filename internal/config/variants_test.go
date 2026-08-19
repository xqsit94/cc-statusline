package config

import (
	"strings"
	"testing"
)

func TestEverySegmentHasVariants(t *testing.T) {
	for _, name := range SegmentNames {
		if len(Variants[name]) == 0 {
			t.Errorf("%s has no shipped formats", name)
		}
	}
	for name := range Variants {
		if !contains(SegmentNames, name) {
			t.Errorf("Variants has %q, which is not a segment", name)
		}
	}
}

func TestEveryVariantIsACompleteAssignment(t *testing.T) {
	for name, ring := range Variants {
		want := SegmentKeys(name)
		for i, v := range ring {
			seen := map[string]bool{}
			for _, kv := range v {
				acc, ok := accessorByKey[kv.Key]
				if !ok {
					t.Errorf("%s format %d sets %q, which is not a format key", name, i+1, kv.Key)
					continue
				}
				if acc.segment != name {
					t.Errorf("%s format %d sets %q, which belongs to %s",
						name, i+1, kv.Key, acc.segment)
				}
				seen[kv.Key] = true
			}
			for _, key := range want {
				if !seen[key] {
					t.Errorf("%s format %d does not set %q; a partial variant leaves that key "+
						"holding whatever set it last", name, i+1, key)
				}
			}
		}
	}
}

func TestTheFirstVariantIsWhatTheDefaultsSay(t *testing.T) {
	def := Defaults().Segments
	for _, name := range SegmentNames {
		if i := IndexOfVariant(Variants[name], name, def); i != 0 {
			t.Errorf("%s: the defaults are at ring position %d, want 0.\n default: %v\n first:   %v",
				name, i, VariantOf(name, def), Variants[name][0])
		}
	}
}

func TestEveryVariantSurvivesValidation(t *testing.T) {
	for name, ring := range Variants {
		for i, v := range ring {
			cfg := Defaults()
			ApplyVariant(v, &cfg.Segments)
			for _, n := range Validate(cfg) {
				if strings.HasPrefix(n.Key, "segments."+name+".") {
					t.Errorf("%s format %d was repaired on load: %s", name, i+1, n)
				}
			}
			if got := VariantOf(name, cfg.Segments); !sameAssignment(got, v) {
				t.Errorf("%s format %d loaded as %v, want %v", name, i+1, got, v)
			}
		}
	}
}

func TestEveryShippedLayoutRendersWhatItLooksLike(t *testing.T) {
	want := map[string]string{
		"15:04":              "09:34",
		"2 Jan 15:04":        "18 Nov 09:34",
		"resets 15:04":       "resets 09:34",
		"resets 2 Jan 15:04": "resets 18 Nov 09:34",
	}

	shipped := map[string]bool{}
	for name, ring := range Variants {
		for i, v := range ring {
			layout, ok := v.value("segments." + name + ".reset_format")
			if !ok {
				continue
			}
			shipped[layout] = true
			w, listed := want[layout]
			if !listed {
				t.Errorf("%s format %d uses the layout %q, which nothing here has looked at",
					name, i+1, layout)
				continue
			}
			if got := layoutProbe.Format(layout); got != w {
				t.Errorf("%s format %d: %q renders %q, want %q", name, i+1, layout, got, w)
			}
		}
	}
	for layout := range want {
		if !shipped[layout] {
			t.Errorf("%q is checked here but no variant uses it", layout)
		}
	}
}

func TestChangedNamesOnlyWhatMoved(t *testing.T) {
	base := Defaults().Segments

	if got := Changed(base, base); len(got) != 0 {
		t.Errorf("nothing moved but Changed reported %v", got)
	}

	moved := base
	ApplyVariant(Variants["cost"][1], &moved)
	got := Changed(moved, base)
	if len(got) != 1 || got[0].Key != "segments.cost.format" {
		t.Fatalf("Changed = %v, want just segments.cost.format", got)
	}
	if want, _ := Variants["cost"][1].value("segments.cost.format"); got[0].Value != want {
		t.Errorf("Changed carried %q, want %q", got[0].Value, want)
	}
}

func TestSplitKeyAddressesTheTable(t *testing.T) {
	for _, name := range SegmentNames {
		for _, key := range SegmentKeys(name) {
			table, k, ok := SplitKey(key)
			if !ok {
				t.Errorf("SplitKey(%q) failed", key)
				continue
			}
			if table != "segments."+name {
				t.Errorf("SplitKey(%q) table = %q, want %q", key, table, "segments."+name)
			}
			if strings.Contains(k, ".") {
				t.Errorf("SplitKey(%q) key = %q, which still holds a dot", key, k)
			}
		}
	}
	for _, bad := range []string{"", "format", ".format", "segments."} {
		if _, _, ok := SplitKey(bad); ok {
			t.Errorf("SplitKey(%q) claimed to address something", bad)
		}
	}
}
