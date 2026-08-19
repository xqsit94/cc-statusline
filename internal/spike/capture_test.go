package spike

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const sentinel = "CC_STATUSLINE_TEST_CAPTURE"

func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv(sentinel); ok {
		var argv []string
		if args != "" {
			argv = strings.Split(args, "\x1f")
		}
		os.Exit(Capture(argv))
	}
	os.Exit(m.Run())
}

func runCapture(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestMain")
	cmd.Env = append(os.Environ(),
		sentinel+"="+strings.Join(args, "\x1f"),
		"XDG_CACHE_HOME="+t.TempDir(),
	)
	cmd.Stdin = strings.NewReader(stdin)

	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()

	code = 0
	var exitErr *exec.ExitError
	if err != nil {
		if !asExitError(err, &exitErr) {
			t.Fatalf("running capture: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return out.String(), errBuf.String(), code
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func TestCaptureAlwaysHoldsTheFailureContract(t *testing.T) {
	const validPayload = `{"model":{"display_name":"Opus 5"},` +
		`"context_window":{"context_window_size":200000,"used_percentage":42,` +
		`"current_usage":{"input_tokens":10,"cache_read_input_tokens":83990,` +
		`"cache_creation_input_tokens":0,"output_tokens":500}}}`

	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"valid payload", validPayload, nil},
		{"empty stdin", "", nil},
		{"malformed json", "not json at all", nil},
		{"null payload", "null", nil},
		{"empty object", "{}", nil},
		{"truncated json", `{"model":{"display_`, nil},
		{"json array", `[1,2,3]`, nil},
		{"child exits non-zero", validPayload, []string{"--", "sh", "-c", "echo out; exit 3"}},
		{"child prints nothing", validPayload, []string{"--", "sh", "-c", "exit 0"}},
		{"child does not exist", validPayload, []string{"--", "/nonexistent/binary"}},
		{"child writes only stderr", validPayload, []string{"--", "sh", "-c", "echo boom >&2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runCapture(t, tc.stdin, tc.args...)

			if code != 0 {
				t.Errorf("exit code = %d, want 0 (a non-zero exit blanks the status line)", code)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty (§3.3 forbids stderr on the render path)", stderr)
			}
			if strings.TrimSpace(stdout) == "" {
				t.Error("stdout is empty; the status line would render blank")
			}
			if n := strings.Count(strings.TrimRight(stdout, "\n"), "\n"); n > 1 {
				t.Errorf("stdout has %d newlines, want at most 1 (2 lines max)", n+1)
			}
		})
	}
}

func TestCaptureForwardsChildOutputVerbatim(t *testing.T) {
	const colored = `\033[38;2;255;0;0mRED\033[0m`

	stdout, _, code := runCapture(t, `{}`, "--", "sh", "-c", "printf '"+colored+"'")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if want := "\x1b[38;2;255;0;0mRED\x1b[0m"; !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", stdout, want)
	}
}

func TestOccupancyExcludesOutputTokens(t *testing.T) {
	d, ok := parse([]byte(`{"context_window":{"current_usage":{
		"input_tokens":2,"cache_read_input_tokens":1000,
		"cache_creation_input_tokens":98,"output_tokens":500}}}`))
	if !ok {
		t.Fatal("fixture did not parse")
	}

	got, parts, ok := d.occupancy()
	if !ok {
		t.Fatal("occupancy() reported no usage fields")
	}
	if want := 1100.0; got != want {
		t.Errorf("occupancy() = %v, want %v (output_tokens must not count)", got, want)
	}
	if joined := strings.Join(parts, " "); !strings.Contains(joined, "(output_tokens=500 excluded)") {
		t.Errorf("parts = %q, want the exclusion to be visible in the report", joined)
	}
}
