package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// capturedEnv names the environment this build's rendering depends on.
//
// PRD §4.1: capture records the environment, not just the payload. Without it
// §6's capability model is unverifiable and §10.3's wizard cannot reproduce the
// real render environment — a bug report saying "the colours are wrong" is
// unactionable when the reporter's COLORTERM and LANG are unknown.
var capturedEnv = []string{
	"COLUMNS", "LINES", "TERM", "COLORTERM", "LANG", "LC_CTYPE", "NO_COLOR",
}

// capture is the on-disk record. It is a wrapper rather than the bare payload
// because the environment has to travel with it; `--payload` reads the bare
// form, so the two are deliberately different shapes.
type capture struct {
	CapturedAt string            `json:"captured_at"`
	Env        map[string]string `json:"env"`
	Payload    json.RawMessage   `json:"payload"`
}

// Capture implements `cc-statusline capture [FILE]`: render, and tee the
// payload and terminal environment aside.
//
// It always writes the default path, and additionally writes FILE when given,
// so that a one-off capture never costs the user their rolling last-payload.
// A write failure is swallowed: PRD §4.1 requires that capture can never
// affect the render or the exit code, and a status line that vanishes because
// a cache directory is read-only would be a poor trade for a diagnostic.
func Capture(args []string, env map[string]string, stdin io.Reader, stdout io.Writer) int {
	raw, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))

	rec := capture{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Env:        map[string]string{},
		Payload:    json.RawMessage(raw),
	}
	if !json.Valid(raw) {
		// Preserve it as a string rather than emitting invalid JSON: a
		// malformed payload is exactly the thing worth capturing.
		quoted, _ := json.Marshal(string(raw))
		rec.Payload = quoted
	}
	for _, k := range capturedEnv {
		if v, ok := os.LookupEnv(k); ok {
			rec.Env[k] = v
		}
	}

	if encoded, err := json.MarshalIndent(rec, "", "  "); err == nil {
		if path, err := DefaultCapturePath(); err == nil {
			writeQuietly(path, encoded)
		}
		for _, explicit := range args {
			if explicit != "" {
				writeQuietly(explicit, encoded)
			}
		}
	}

	return Render(nil, env, bytes.NewReader(raw), stdout)
}

// DefaultCapturePath is $XDG_CACHE_HOME/cc-statusline/last-payload.json.
func DefaultCapturePath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last-payload.json"), nil
}

func cacheDir() (string, error) {
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "cc-statusline"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "cc-statusline"), nil
}

func writeQuietly(path string, b []byte) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(path, b, 0o644)
}
