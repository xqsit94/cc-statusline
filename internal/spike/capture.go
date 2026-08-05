// Package spike implements the M0 contract-verification build (docs/PRD.md
// §11, §3.1.1). It is deliberately self-contained and deliberately temporary:
// M1 replaces it with internal/payload, and this package is deleted.
//
// One thing here is not temporary. Capture holds the §3.3 failure contract from
// the first commit — always exit 0, always write one non-empty line, never
// write to stderr, buffer the output and write it once — because a spike that
// blanks the user's status line teaches nothing about anything else.
package spike

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// childTimeout bounds the passthrough command. Claude Code cancels an in-flight
// status line on the next render anyway; this just stops a wedged child from
// holding a process open indefinitely.
const childTimeout = 2 * time.Second

// Capture implements `cc-statusline capture`. It never returns non-zero.
func Capture(args []string) int {
	var out bytes.Buffer

	defer func() {
		if r := recover(); r != nil {
			// §3.3: reset before the fallback, so a panic mid-write cannot
			// emit a half-formed line.
			out.Reset()
			out.WriteString("cc-statusline\n")
		}
		os.Stdout.Write(out.Bytes())
	}()

	// `capture -- CMD ...` and `capture CMD ...` mean the same thing; the
	// separator is there so a command starting with a dash is unambiguous.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	payload, _ := io.ReadAll(os.Stdin)
	spooled, spoolErr := spool(payload)

	if len(args) > 0 {
		if forwarded, ok := runChild(args, payload); ok {
			out.Write(forwarded)
			if !bytes.HasSuffix(forwarded, []byte("\n")) {
				out.WriteByte('\n')
			}
			return 0
		}
		// Child failed, timed out, or printed nothing. Fall through to the
		// probe line rather than leaving the status line blank.
	}

	out.WriteString(probeLine(payload, spooled, spoolErr))
	out.WriteByte('\n')
	return 0
}

// SpoolDir returns the directory captured payloads are written to.
func SpoolDir() (string, error) {
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "cc-statusline", "spike"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "cc-statusline", "spike"), nil
}

// spool writes one payload to its own file. One file per capture rather than
// appended lines: status line renders are not serialised against each other,
// and a payload is far larger than PIPE_BUF, so concurrent appends could
// interleave and corrupt exactly the data M0 exists to collect.
func spool(payload []byte) (string, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return "", errors.New("empty payload")
	}
	dir, err := SpoolDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 10)+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// runChild feeds the payload to an existing status line command and returns its
// stdout. Its stderr is discarded: PRD §3.3 forbids stderr on this path, and a
// child that writes there must not be able to break the contract on our behalf.
func runChild(argv []string, payload []byte) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), childTimeout)
	defer cancel()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	// A child that exits non-zero is dropped even if it printed: Claude Code
	// blanks the status line on a non-zero exit, so forwarding that output
	// would show something in the spike that production would not show.
	if err := cmd.Run(); err != nil || len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil, false
	}
	return stdout.Bytes(), true
}
