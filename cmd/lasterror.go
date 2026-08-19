package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
)

const lastErrorName = "last-error.txt"

func recordConfigNotes(notes []config.Defaulted) {
	if len(notes) == 0 {
		if path, err := lastErrorPath(); err == nil {
			removeQuietly(path)
		}
		return
	}
	path, err := lastErrorPath()
	if err != nil {
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s  cc-statusline %s\n", time.Now().UTC().Format(time.RFC3339), version)
	fmt.Fprintf(&b, "%d configuration %s repaired at load; the status line rendered anyway.\n\n",
		len(notes), plural(len(notes), "problem", "problems"))
	for _, n := range notes {
		fmt.Fprintf(&b, "  %s\n", n)
	}
	writeQuietly(path, []byte(b.String()))
}

func lastErrorPath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, lastErrorName), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
