package payload

import (
	"math"
	"strings"
	"testing"
	"time"
)

func parseT(t *testing.T, s string) *Payload {
	t.Helper()
	p, _ := Parse([]byte(s))
	if p == nil {
		t.Fatal("Parse returned nil; the render path would dereference it")
	}
	return p
}

func TestDecodeNeverReturnsNil(t *testing.T) {
	// Render dereferences the result unconditionally. Every input has to
	// produce something safe to hold.
	for _, in := range []string{"", "{}", "null", "garbage", `[1,2]`, `"str"`, "\x00"} {
		if p, _ := Parse([]byte(in)); p == nil {
			t.Errorf("Parse(%q) = nil", in)
		}
		if p, _ := Decode(strings.NewReader(in)); p == nil {
			t.Errorf("Decode(%q) = nil", in)
		}
	}
}

func TestParseReportsErrorsWithoutLosingUsability(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{`{}`, false},
		{`null`, false}, // valid JSON; decodes to nothing
		{``, true},
		{`{"model":`, true},
		{`[1,2,3]`, true},
	}
	for _, tc := range cases {
		p, err := Parse([]byte(tc.in))
		if (err != nil) != tc.wantErr {
			t.Errorf("Parse(%q) err = %v, want error: %v", tc.in, err, tc.wantErr)
		}
		if _, ok := p.ModelName(); ok && tc.wantErr {
			t.Errorf("Parse(%q) surfaced a model name from unparseable input", tc.in)
		}
	}
}

func TestAbsenceIsNotZero(t *testing.T) {
	// The reason every accessor returns (value, ok): a missing cost object and
	// a genuinely free session are different facts, and only one should render.
	absent := parseT(t, `{}`)
	zero := parseT(t, `{"cost":{"total_cost_usd":0,"total_lines_added":0,"total_lines_removed":0}}`)

	if _, ok := absent.CostUSD(); ok {
		t.Error("CostUSD() on {} reported present")
	}
	v, ok := zero.CostUSD()
	if !ok || v != 0 {
		t.Errorf("CostUSD() on an explicit 0 = (%v, %v), want (0, true)", v, ok)
	}

	if _, _, ok := absent.LinesChanged(); ok {
		t.Error("LinesChanged() on {} reported present")
	}
	a, r, ok := zero.LinesChanged()
	if !ok || a != 0 || r != 0 {
		t.Errorf("LinesChanged() on explicit zeros = (%d, %d, %v), want (0, 0, true)", a, r, ok)
	}

	// A cost object holding only nulls is present-but-empty, which is absence
	// for every field in it.
	nulls := parseT(t, `{"cost":{"total_cost_usd":null}}`)
	if _, ok := nulls.CostUSD(); ok {
		t.Error("CostUSD() on an explicit null reported present")
	}
}

func TestPercentFollowsTheSpec(t *testing.T) {
	// PRD §5.3 defines p_exact and p_shown once. These cases are that
	// definition, including the precedence that makes it worth defining.
	cases := []struct {
		name      string
		in        string
		wantOK    bool
		wantExact float64
		wantShown int
	}{
		{
			name:      "tokens and size win over the reported percentage",
			in:        `{"context_window":{"total_input_tokens":109879,"context_window_size":1000000,"used_percentage":11}}`,
			wantOK:    true,
			wantExact: 10.9879,
			wantShown: 11,
		},
		{
			name: "the reported percentage is the fallback, not the source",
			// M0 measured used_percentage is round(tokens/size*100); here the
			// operands are missing, so the rounded value is all there is.
			in:        `{"context_window":{"used_percentage":42}}`,
			wantOK:    true,
			wantExact: 42,
			wantShown: 42,
		},
		{
			name:      "a zero window never divides",
			in:        `{"context_window":{"total_input_tokens":100,"context_window_size":0,"used_percentage":7}}`,
			wantOK:    true,
			wantExact: 7,
			wantShown: 7,
		},
		{
			name:      "present but empty renders as zero",
			in:        `{"context_window":{}}`,
			wantOK:    true,
			wantExact: 0,
			wantShown: 0,
		},
		{
			name:      "a null percentage renders as zero — the startup state",
			in:        `{"context_window":{"used_percentage":null}}`,
			wantOK:    true,
			wantExact: 0,
			wantShown: 0,
		},
		{
			name:   "an absent object is the only empty case",
			in:     `{}`,
			wantOK: false,
		},
		{
			name: "the fractional case: fill and band read different values",
			// p_exact 69.6 rounds to 70, which crosses the warning threshold
			// while a 10-cell bar fills only 7. PRD §9.1's fractional-pct.json.
			in:        `{"context_window":{"total_input_tokens":139200,"context_window_size":200000}}`,
			wantOK:    true,
			wantExact: 69.6,
			wantShown: 70,
		},
		{
			name:      "over-full is not clamped in p_exact, only in p_shown",
			in:        `{"context_window":{"total_input_tokens":250000,"context_window_size":200000}}`,
			wantOK:    true,
			wantExact: 125,
			wantShown: 100,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parseT(t, tc.in)

			exact, ok := p.PercentExact()
			if ok != tc.wantOK {
				t.Fatalf("PercentExact() ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				if p.ContextPresent() {
					t.Error("ContextPresent() = true for an absent object")
				}
				return
			}
			if math.Abs(exact-tc.wantExact) > 1e-9 {
				t.Errorf("p_exact = %v, want %v", exact, tc.wantExact)
			}
			shown, _ := p.PercentShown()
			if shown != tc.wantShown {
				t.Errorf("p_shown = %d, want %d", shown, tc.wantShown)
			}
			if !p.ContextPresent() {
				t.Error("ContextPresent() = false for a present object")
			}
		})
	}
}

func TestRateLimitWindowsAreIndependent(t *testing.T) {
	// A non-subscriber has neither window; both are absent before the first
	// response; and one can arrive without the other.
	sevenOnly := parseT(t, `{"rate_limits":{"seven_day":{"used_percentage":62,"resets_at":1786435200}}}`)

	if _, ok := sevenOnly.RateLimitPercent(FiveHour); ok {
		t.Error("five_hour reported present when only seven_day exists")
	}
	v, ok := sevenOnly.RateLimitPercent(SevenDay)
	if !ok || v != 62 {
		t.Errorf("seven_day = (%v, %v), want (62, true)", v, ok)
	}

	// PRD §3.1: resets_at is unix epoch seconds as a number, not a string.
	// M0 measured this; parsing it as RFC3339 would silently yield nothing.
	at, ok := sevenOnly.RateLimitResetsAt(SevenDay)
	if !ok {
		t.Fatal("resets_at absent")
	}
	if want := time.Unix(1786435200, 0); !at.Equal(want) {
		t.Errorf("resets_at = %v, want %v", at, want)
	}

	none := parseT(t, `{}`)
	if _, ok := none.RateLimitPercent(SevenDay); ok {
		t.Error("rate limit reported present with no rate_limits object")
	}
}

func TestProjectName(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
		ok   bool
	}{
		"basename":         {`{"workspace":{"project_dir":"/home/u/my-project"}}`, "my-project", true},
		"trailing slash":   {`{"workspace":{"project_dir":"/home/u/my-project/"}}`, "my-project", true},
		"filesystem root":  {`{"workspace":{"project_dir":"/"}}`, "", false},
		"no workspace":     {`{}`, "", false},
		"null project_dir": {`{"workspace":{"project_dir":null}}`, "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := parseT(t, tc.in).ProjectName()
			if got != tc.want || ok != tc.ok {
				t.Errorf("ProjectName() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestDuration(t *testing.T) {
	p := parseT(t, `{"cost":{"total_duration_ms":7641362}}`)
	d, ok := p.Duration()
	if !ok {
		t.Fatal("Duration() absent")
	}
	if want := 7641362 * time.Millisecond; d != want {
		t.Errorf("Duration() = %v, want %v", d, want)
	}
	if _, ok := parseT(t, `{"cost":{}}`).Duration(); ok {
		t.Error("Duration() reported present with no total_duration_ms")
	}
}

func TestKeyDiffFindsDrift(t *testing.T) {
	// PRD §3.1.2: nothing on the render path can notice a schema change,
	// because the failure contract requires render to stay quiet. This is
	// where the noticing happens.
	p := parseT(t, `{
		"model":{"display_name":"Opus 5"},
		"context_window":{"current_usage":{"input_tokens":2}},
		"brand_new_field":true,
		"nested":{"also_new":"x"}
	}`)

	unknown, missing := p.KeyDiff()

	wantUnknown := map[string]bool{"brand_new_field": true, "nested.also_new": true}
	for _, k := range unknown {
		if !wantUnknown[k] {
			t.Errorf("unknown includes %q, which is documented in §3.1", k)
		}
		delete(wantUnknown, k)
	}
	for k := range wantUnknown {
		t.Errorf("unknown is missing %q — drift went undetected", k)
	}

	// current_usage.* is a documented subtree, so its members are not drift.
	for _, k := range unknown {
		if strings.HasPrefix(k, "context_window.current_usage") {
			t.Errorf("unknown includes %q, but §3.1 documents the subtree", k)
		}
	}

	// Everything absent here is conditional, so missing should be long — the
	// accessor for it exists to be presented with that caveat, not as a fault.
	if len(missing) == 0 {
		t.Error("missing is empty for a payload holding four keys")
	}
	for _, k := range missing {
		if k == "model.display_name" {
			t.Error("missing includes a key that is present in the payload")
		}
	}
}

func TestFlattenKeysShapes(t *testing.T) {
	got, err := FlattenKeys([]byte(`{"a":1,"b":"s","c":true,"d":null,"e":[{"f":2}],"g":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"a": "number", "b": "string", "c": "bool", "d": "null",
		"e": "array[1]", "e[].f": "number", "g": "object(empty)",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("FlattenKeys()[%q] = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("FlattenKeys() returned %d keys, want %d: %v", len(got), len(want), got)
	}

	if _, err := FlattenKeys([]byte("not json")); err == nil {
		t.Error("FlattenKeys() accepted invalid JSON")
	}
}

func TestDecodeIsBounded(t *testing.T) {
	// An unbounded io.ReadAll on stdin is a memory hazard on a path that runs
	// on every assistant turn. Oversized input must be truncated, not absorbed.
	huge := strings.NewReader(`{"model":{"display_name":"x"}}` + strings.Repeat(" ", 8<<20))
	p, _ := Decode(huge)
	if len(p.Raw()) > maxPayloadBytes {
		t.Errorf("Decode read %d bytes, want at most %d", len(p.Raw()), maxPayloadBytes)
	}
}
