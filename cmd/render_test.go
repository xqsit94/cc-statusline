package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/payload"
)

// M1's exit criterion (PRD §11): `{}` and malformed input render a fallback
// line and exit 0. These tests are that criterion, plus the rest of §3.3.

const realPayload = `{"model":{"id":"claude-opus-5","display_name":"Opus 5 (1M context)"},
	"workspace":{"current_dir":"/w","project_dir":"/w/proj"},
	"cost":{"total_cost_usd":0.85,"total_duration_ms":180000,
	        "total_lines_added":150,"total_lines_removed":30},
	"context_window":{"context_window_size":1000000,"used_percentage":11,
	                  "total_input_tokens":109879,"total_output_tokens":574}}`

func TestRenderHoldsTheFailureContract(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		want  string // expected exact line
	}{
		{"real payload", realPayload, "◆ Opus 5 (1M context)"},
		{"empty object", `{}`, "cc-statusline"},
		{"empty stdin", ``, "cc-statusline"},
		{"json null", `null`, "cc-statusline"},
		{"malformed json", `not json at all`, "cc-statusline"},
		{"truncated json", `{"model":{"display_`, "cc-statusline"},
		{"json array", `[1,2,3]`, "cc-statusline"},
		{"json string", `"hello"`, "cc-statusline"},
		{"nul bytes", "\x00\x00\x00", "cc-statusline"},
		{"model present, name null", `{"model":{"display_name":null}}`, "cc-statusline"},
		{"model name blank", `{"model":{"display_name":"   "}}`, "cc-statusline"},
		{"deeply nested junk", `{"a":{"b":{"c":{"d":[[[1]]]}}}}`, "cc-statusline"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := Render(nil, strings.NewReader(tc.stdin), &out)

			if code != 0 {
				t.Errorf("exit = %d, want 0 (non-zero blanks the status line)", code)
			}
			if got := out.String(); got != tc.want+"\n" {
				t.Errorf("stdout = %q, want %q", got, tc.want+"\n")
			}
		})
	}
}

func TestRenderRecoversFromAPanic(t *testing.T) {
	// Nothing in correct code panics, so the recover in Render is otherwise
	// unreachable and would rot untested. This is the seam that exercises it.
	original := renderLines
	t.Cleanup(func() { renderLines = original })

	t.Run("panic yields the model name", func(t *testing.T) {
		renderLines = func(p *payload.Payload) []string {
			panic("segment exploded")
		}
		var out bytes.Buffer
		if code := Render(nil, strings.NewReader(realPayload), &out); code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("buffer is reset before the fallback", func(t *testing.T) {
		// A segment that writes a partial line and then dies must not leave
		// that fragment on screen ahead of the fallback.
		renderLines = func(p *payload.Payload) []string {
			defer panic("died after emitting")
			return []string{"PARTIAL-LINE-THAT-MUST-NOT-SURVIVE"}
		}
		var out bytes.Buffer
		Render(nil, strings.NewReader(realPayload), &out)

		if strings.Contains(out.String(), "PARTIAL") {
			t.Errorf("stdout = %q, want the torn line discarded", out.String())
		}
		if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("panic with no payload still prints a line", func(t *testing.T) {
		renderLines = func(p *payload.Payload) []string { panic("nope") }
		var out bytes.Buffer
		Render(nil, strings.NewReader("garbage"), &out)
		if got, want := out.String(), "cc-statusline\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
}

func TestRenderNeverEmitsAnEmptyLine(t *testing.T) {
	original := renderLines
	t.Cleanup(func() { renderLines = original })

	// M2's segments can all report absent at once, which is what an empty
	// payload does. The joiner would then hand back nothing at all.
	for name, lines := range map[string][]string{
		"no lines":     {},
		"empty string": {""},
		"whitespace":   {"   ", "\t"},
		"nil":          nil,
	} {
		t.Run(name, func(t *testing.T) {
			renderLines = func(p *payload.Payload) []string { return lines }
			var out bytes.Buffer
			Render(nil, strings.NewReader(realPayload), &out)
			if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestRenderPayloadFlag(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(good, []byte(realPayload), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("reads the file instead of stdin", func(t *testing.T) {
		var out bytes.Buffer
		// stdin deliberately holds a different model, so a pass-through bug
		// would be visible rather than coincidentally correct.
		stdin := strings.NewReader(`{"model":{"display_name":"WRONG"}}`)
		if code := Render([]string{"--payload", good}, stdin, &out); code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("unreadable file exits 1 — the one exception", func(t *testing.T) {
		var out bytes.Buffer
		code := Render([]string{"--payload", filepath.Join(dir, "absent.json")},
			strings.NewReader(realPayload), &out)
		if code != 1 {
			t.Errorf("exit = %d, want 1 (PRD §4.1's only non-zero render path)", code)
		}
	})

	t.Run("a bad flag still renders", func(t *testing.T) {
		var out bytes.Buffer
		if code := Render([]string{"--nonsense"}, strings.NewReader(realPayload), &out); code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if strings.TrimSpace(out.String()) == "" {
			t.Error("a typo in settings.json must not blank the status line")
		}
	})
}

func TestCaptureRendersAndTees(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	explicit := filepath.Join(dir, "explicit.json")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("COLUMNS", "120")

	var out bytes.Buffer
	if code := Capture([]string{explicit}, strings.NewReader(realPayload), &out); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	def, err := DefaultCapturePath()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{def, explicit} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		// The environment is the half that makes a bug report actionable.
		for _, want := range []string{`"COLORTERM": "truecolor"`, `"COLUMNS": "120"`, `"Opus 5 (1M context)"`} {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s does not contain %s", filepath.Base(path), want)
			}
		}
	}
}

func TestCaptureSurvivesAnUnwritableTarget(t *testing.T) {
	// PRD §4.1: a capture write failure can never affect the render or the
	// exit code. A read-only cache directory must cost a diagnostic, not a
	// status line.
	dir := t.TempDir()

	// A regular file where a directory is expected: MkdirAll below it fails
	// with ENOTDIR, which is what a user with a stale ~/.cache entry hits.
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", blocked)

	var out bytes.Buffer
	if code := Capture([]string{filepath.Join(blocked, "explicit.json")},
		strings.NewReader(realPayload), &out); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "◆ Opus 5 (1M context)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestCaptureKeepsMalformedPayloads(t *testing.T) {
	// A malformed payload is exactly the thing worth capturing, so it must
	// survive as data rather than being dropped for not being valid JSON.
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	var out bytes.Buffer
	Capture(nil, strings.NewReader("this is not json"), &out)

	def, _ := DefaultCapturePath()
	b, err := os.ReadFile(def)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "this is not json") {
		t.Errorf("capture = %s, want the raw bytes preserved", b)
	}
	if !json.Valid(b) {
		t.Error("the capture file itself must remain valid JSON")
	}
}

func TestMainDispatch(t *testing.T) {
	if code := Main(nil); code != 0 {
		t.Errorf("bare invocation exit = %d, want 0 (PRD §4.1)", code)
	}
	if code := Main([]string{"no-such-subcommand"}); code != 2 {
		t.Errorf("unknown subcommand exit = %d, want 2", code)
	}
}

func TestVersionReports(t *testing.T) {
	var out bytes.Buffer
	if code := Version(&out); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(out.String(), "cc-statusline ") {
		t.Errorf("version = %q", out.String())
	}
}
