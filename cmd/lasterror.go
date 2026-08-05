package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
)

// lastErrorName is the file PRD §7.1 promises: "doctor reports it and render
// appends it to last-error.txt".
const lastErrorName = "last-error.txt"

// recordConfigNotes is the other half of §7.1's contract. Loading a config
// never fails; every problem becomes a note against a config that still
// renders. The status line has nowhere to display those notes, so this is where
// they go, and `doctor` is where they are read back.
//
// # One correction to §7.1
//
// §7.1 says render *appends*. It must not. `render` runs on every refresh —
// every sixty seconds, in every session, for as long as Claude Code is open —
// and a config with a typo'd key produces the same note every time. Appending
// would write about fourteen hundred identical lines a day, forever, to
// diagnose a single misspelling. The file is therefore rewritten with the
// current set: same diagnostic value, bounded size.
//
// It is best-effort in the strictest sense. A failure here cannot be allowed to
// affect §3.3's contract — exit 0, one line on stdout, nothing on stderr — so
// every error is discarded. A diagnostic that could blank the status line would
// be worse than no diagnostic.
func recordConfigNotes(notes []config.Defaulted) {
	if len(notes) == 0 {
		// Nothing is wrong. The stale file from a problem the user has since
		// fixed is removed, so that `doctor` does not report a corrected typo
		// as a live one.
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
