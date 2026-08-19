package cmd

import (
	"fmt"
	"os"
)

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
        harness for §9.4's manual visual gate; the README has the checklist.

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

  cc-statusline config [--dry-run]
        Edit the segment layout with a live preview: enable, reorder, and
        set drop priorities while watching what survives a narrow terminal.
        Writes your config.toml as a textual patch, so every comment in it
        survives.

  cc-statusline doctor [--json]
        What this build detected, what it resolved, and what went wrong
        last time. The first thing to run when the line looks wrong.

  cc-statusline version
        Version, commit, and build date.

  cc-statusline update [--check] [--force]
        Check GitHub for a newer release. --check only reports it;
        --force downloads, verifies its checksum, and replaces this
        binary. The only command here that touches the network.

  cc-statusline spike capture [-- CMD [ARGS...]]
  cc-statusline spike report
        M0 contract verification. Temporary; removed when TODOS.md's
        C-4 and C-5 close.
`

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
	case "config":
		return Config(args[1:], env, os.Stdout, os.Stderr)
	case "uninstall":
		return Uninstall(args[1:], env, os.Stdout, os.Stderr)
	case "doctor":
		return Doctor(args[1:], env, os.Stdout, os.Stderr)
	case "version":
		return Version(os.Stdout)
	case "update":
		return Update(args[1:], os.Stdout, os.Stderr)
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
