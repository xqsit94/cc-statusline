// Command cc-statusline is a Claude Code status line.
//
// main lives at the module root, not under cmd/, so that
// `go install github.com/xqsit94/cc-statusline@latest` resolves — PRD §10.1
// promises that install path, and it only works for a main package at the
// module root. Arg dispatch and the outermost recover live here; everything
// else is in cmd/.
package main

import (
	"fmt"
	"os"

	"github.com/xqsit94/cc-statusline/cmd"
	"github.com/xqsit94/cc-statusline/internal/style"
)

func main() { os.Exit(run(os.Args[1:], style.Environ(os.Environ()))) }

// run is separated from main only so its deferred recover can set the exit
// code — os.Exit skips deferred functions, so main cannot both defer and exit.
//
// This recover is a backstop for the non-render subcommands. It deliberately
// does NOT print a status line: `render` and `capture` own PRD §3.3's contract
// themselves, because only they hold the output buffer that has to be reset
// before a fallback can be written. A panic that reaches here came from `init`,
// `config`, or `doctor`, where failing loudly is correct.
func run(args []string, env map[string]string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "cc-statusline: internal error: %v\n", r)
			code = 1
		}
	}()
	return cmd.Main(args, env)
}
