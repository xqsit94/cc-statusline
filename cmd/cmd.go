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

  cc-statusline preview [--matrix] [--state NAME] [--width N]
                        [--icons SET] [--sep STYLE] [--ambiguous N]
        Render the PRD §5.1 reference states with a width rule under each
        line, against capability sets this terminal may not have. The
        harness for §9.4's manual visual gate; see docs/M4-visual-gate.md.

  cc-statusline preview --probe
        Print a column ruler instead of a status line. Install it as the
        statusLine command to measure what width_reserve should be (C-7).

  cc-statusline init [--preset NAME] [--icons SET] [--force] [--dry-run]
        Write ~/.config/cc-statusline/config.toml and point Claude Code's
        statusLine at this binary. Idempotent; backs up settings.json
        before touching it; declines rather than edit a file that has
        comments in it.

  cc-statusline uninstall [--force]
        Remove the statusLine key and nothing else. Leaves the config file.

  cc-statusline doctor [--json]
        What this build detected, what it resolved, and what went wrong
        last time. The first thing to run when the line looks wrong.

  cc-statusline version
        Version, commit, and build date.

  cc-statusline spike capture [-- CMD [ARGS...]]
  cc-statusline spike report
        M0 contract verification. Temporary; removed when PRD §14.1's
        C-4 and C-5 close.

Not yet implemented: config (M7).
`

// Main dispatches a subcommand and returns the process exit code.
//
// PRD §4.1: `render` is required, never inferred from stdin. Sniffing
// os.Stdin.Stat() misfires on /dev/null, on file redirects, and on closed
// descriptors — that is, under cron, under CI, and under the `env -i` test
// §9.3 mandates. `init` writes the subcommand explicitly, so requiring it
// costs nothing.
func Main(args []string, env map[string]string) int {
	if len(args) == 0 {
		fmt.Print(Usage)
		return 0
	}

	switch args[0] {
	case "render":
		return Render(args[1:], env, os.Stdin, os.Stdout)
	case "capture":
		return Capture(args[1:], env, os.Stdin, os.Stdout)
	case "preview":
		return Preview(args[1:], env, os.Stdin, os.Stdout)
	case "init":
		return Init(args[1:], env, os.Stdout, os.Stderr)
	case "uninstall":
		return Uninstall(args[1:], env, os.Stdout, os.Stderr)
	case "doctor":
		return Doctor(args[1:], env, os.Stdout, os.Stderr)
	case "config":
		// Named in the usage text and in §4.1, so it gets a real answer rather
		// than "unknown subcommand" — which would read as a typo in the name.
		return notImplemented(os.Stderr, "config", "M7")
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
