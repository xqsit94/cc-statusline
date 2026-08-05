package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/xqsit94/cc-statusline/internal/spike"
)

// Spike exposes the M0 contract-verification build. It is temporary and namespaced
// under one subcommand so that removing it is deleting this file, one case in
// Main, and internal/spike — nothing else.
//
// It survives M1 because PRD §14.1's C-4 (what used_percentage does compaction
// fire at) and C-5 (the 200k window, and a null percentage at session start)
// both need more real sessions, and `spike capture` is what collects them
// without disturbing whatever status line the user already has.
//
// Note the deliberate difference from `capture`: `spike capture` passes the
// payload through to an existing status line command and forwards its output,
// so nothing on screen changes while data accumulates. `capture` renders our
// own line. They are not interchangeable, which is why they are not the same
// subcommand.
func Spike(args []string, stdin io.Reader, stdout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "cc-statusline: spike needs a mode: capture or report\n")
		return 2
	}

	switch args[0] {
	case "capture":
		return spike.Capture(args[1:])
	case "report":
		return spike.Report(stdout)
	default:
		fmt.Fprintf(os.Stderr, "cc-statusline: unknown spike mode %q\n", args[0])
		return 2
	}
}
