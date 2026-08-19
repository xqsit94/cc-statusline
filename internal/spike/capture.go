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

const childTimeout = 2 * time.Second

func Capture(args []string) int {
	var out bytes.Buffer

	defer func() {
		if r := recover(); r != nil {
			out.Reset()
			out.WriteString("cc-statusline\n")
		}
		os.Stdout.Write(out.Bytes())
	}()

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
	}

	out.WriteString(probeLine(payload, spooled, spoolErr))
	out.WriteByte('\n')
	return 0
}

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

func runChild(argv []string, payload []byte) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), childTimeout)
	defer cancel()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil || len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil, false
	}
	return stdout.Bytes(), true
}
