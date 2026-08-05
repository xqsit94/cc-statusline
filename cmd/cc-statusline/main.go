// Command cc-statusline renders a Claude Code status line.
//
// This is the M0 spike build (docs/PRD.md §11). Nothing here is the real
// product: the entire implementation lives in internal/spike and is deleted at
// M1. M0 exists to answer the two questions in PRD §3.1.1 against real payloads
// before any of the design downstream of §3 is built on assumption.
package main

import (
	"fmt"
	"os"

	"github.com/xqsit94/cc-statusline/internal/spike"
)

const usage = `cc-statusline — Claude Code status line (M0 spike build)

  cc-statusline capture [-- CMD [ARGS...]]
        Read a status line payload on stdin and spool it for later analysis.
        With a trailing command, run CMD with the same payload on stdin and
        forward its output verbatim — so an existing status line keeps working
        untouched while payloads accumulate underneath it.
        Without one, print a one-line probe of the context window numbers.
        Always exits 0 and always prints one non-empty line.

  cc-statusline report
        Analyse every spooled payload. Prints the observed key set against the
        set docs/PRD.md §3.1 claims, and what used_percentage is a percentage
        of. These are M0's two exit criteria.

  cc-statusline spool-dir
        Print the spool directory path.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "capture":
		os.Exit(spike.Capture(os.Args[2:]))
	case "report":
		os.Exit(spike.Report(os.Stdout))
	case "spool-dir":
		dir, err := spike.SpoolDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cc-statusline:", err)
			os.Exit(1)
		}
		fmt.Println(dir)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "cc-statusline: unknown subcommand %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
