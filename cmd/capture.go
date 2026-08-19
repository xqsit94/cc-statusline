package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

var capturedEnv = []string{
	"COLUMNS", "LINES", "TERM", "COLORTERM", "LANG", "LC_CTYPE", "NO_COLOR",
}

type capture struct {
	CapturedAt string            `json:"captured_at"`
	Env        map[string]string `json:"env"`
	Payload    json.RawMessage   `json:"payload"`
}

func Capture(args []string, env map[string]string, stdin io.Reader, stdout io.Writer) int {
	raw, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))

	rec := capture{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Env:        map[string]string{},
		Payload:    json.RawMessage(raw),
	}
	if !json.Valid(raw) {
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

func removeQuietly(path string) { _ = os.Remove(path) }

func writeQuietly(path string, b []byte) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(path, b, 0o644)
}
