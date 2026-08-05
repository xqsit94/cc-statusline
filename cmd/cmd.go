// Package cmd implements cc-statusline's subcommands. PRD §4.1.
package cmd

import (
	"fmt"
	"io"
	"os"
)

// Usage is printed for a bare invocation, for -h, and to stderr for an unknown
// subcommand. M1 ships a subset; the rest arrive with their milestones.
const Usage = `cc-statusline — a Claude Code status line

  cc-statusline render [--payload FILE]
        Read a session payload on stdin and print the status line.
        Exits 0 for any input, including none. Never writes to stderr.

  cc-statusline capture [FILE]
        Render, and tee the payload and terminal environment to FILE
        (default $XDG_CACHE_HOME/cc-statusline/last-payload.json).

  cc-statusline version
        Version, commit, and build date.

  cc-statusline spike capture [-- CMD [ARGS...]]
  cc-statusline spike report
        M0 contract verification. Temporary; removed when PRD §14.1's
        C-4 and C-5 close.

Not yet implemented: config (M7), init / uninstall / doctor (M5).
`

// Main dispatches a subcommand and returns the process exit code.
//
// PRD §4.1: `render` is required, never inferred from stdin. Sniffing
// os.Stdin.Stat() misfires on /dev/null, on file redirects, and on closed
// descriptors — that is, under cron, under CI, and under the `env -i` test
// §9.3 mandates. `init` writes the subcommand explicitly, so requiring it
// costs nothing.
func Main(args []string) int {
	if len(args) == 0 {
		fmt.Print(Usage)
		return 0
	}

	switch args[0] {
	case "render":
		return Render(args[1:], os.Stdin, os.Stdout)
	case "capture":
		return Capture(args[1:], os.Stdin, os.Stdout)
	case "version":
		return Version(os.Stdout)
	case "spike":
		return Spike(args[1:], os.Stdin, os.Stdout)
	case "-h", "--help", "help":
		fmt.Print(Usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cc-statusline: unknown subcommand %q\n\n%s", args[0], Usage)
		return 2
	}
}

// notImplemented is the placeholder for subcommands whose milestone has not
// landed. It is not reachable from Main yet; it exists so that adding a case
// is a one-line change rather than an invitation to stub out behaviour.
func notImplemented(w io.Writer, name, milestone string) int {
	fmt.Fprintf(w, "cc-statusline: %s is not implemented yet (%s)\n", name, milestone)
	return 1
}
